package concurrency

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"go.temporal.io/api/serviceerror"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/streambatcher"
	"google.golang.org/grpc"
)

type clientBatchKey struct {
	namespaceID string
	key         string
}

type clientBatchItem struct {
	ctx context.Context
	req *fcpb.ConcurrencyServiceBatchRequest
}

type clientBatchResult struct {
	res *fcpb.ConcurrencyServiceBatchResponse
	err error
}

// HintHandler is called with the limiter state (generation and available slots) from a Batch
// response. appliedReserveSeq is the highest reserve_seq from this client that the server has
// applied (see ConcurrencyServiceBatchRequest.reserve_seq). runID is the limiter run that
// produced the state.
type HintHandler func(namespaceID, key string, generation int64, availableSlots int32, appliedReserveSeq int64, runID string)

// BatchingClient batches calls to ConcurrencyService.Batch by limiter.
//
// Limiter state in Batch responses is passed to the HintHandler once per rpc, before any caller
// receives its result, so callers see the updated state as soon as their call returns. The
// responses returned to callers contain only their own results, plus the limiter run id.
type BatchingClient struct {
	fcpb.ConcurrencyServiceClient

	hintHandler HintHandler
	batchers    *streambatcher.KeyedBatcher[clientBatchKey, clientBatchItem, clientBatchResult]

	clientID   string
	reserveSeq sync.Map // clientBatchKey -> *atomic.Int64
}

// ClientID returns the id this client uses for reserve accounting.
func (c *BatchingClient) ClientID() string {
	return c.clientID
}

func (c *BatchingClient) nextReserveSeq(key clientBatchKey, counted int32) int64 {
	v, ok := c.reserveSeq.Load(key)
	if !ok {
		v, _ = c.reserveSeq.LoadOrStore(key, &atomic.Int64{})
	}
	return v.(*atomic.Int64).Add(int64(counted)) //nolint:revive
}

// NewBatchingClient returns a BatchingClient. hintHandler may be nil.
func NewBatchingClient(
	client fcpb.ConcurrencyServiceClient,
	opts streambatcher.BatcherOptions,
	timeSource clock.TimeSource,
	hintHandler HintHandler,
) *BatchingClient {
	c := &BatchingClient{
		ConcurrencyServiceClient: client,
		hintHandler:              hintHandler,
		clientID:                 uuid.NewString(),
	}
	c.batchers = streambatcher.NewKeyedBatcherWithPerItemResults(c.applyBatch, opts, timeSource)
	return c
}

func (c *BatchingClient) Batch(
	ctx context.Context,
	req *fcpb.ConcurrencyServiceBatchRequest,
	opts ...grpc.CallOption,
) (*fcpb.ConcurrencyServiceBatchResponse, error) {
	if len(opts) > 0 {
		res, err := c.ConcurrencyServiceClient.Batch(ctx, req, opts...)
		if err != nil {
			return nil, err
		} else if res == nil {
			return nil, serviceerror.NewInternal("nil response")
		}
		c.reportHint(req.GetNamespaceId(), req.GetKey(), res)
		return &fcpb.ConcurrencyServiceBatchResponse{
			ReserveSuccess: res.ReserveSuccess,
			CommitSuccess:  res.CommitSuccess,
			RunId:          res.RunId,
		}, nil
	}
	res, err := c.batchers.Add(ctx, clientBatchKey{
		namespaceID: req.GetNamespaceId(),
		key:         req.GetKey(),
	}, clientBatchItem{ctx: ctx, req: req})
	if err != nil {
		return nil, err
	}
	if res.res == nil && res.err == nil {
		res.err = serviceerror.NewInternal("nil response")
	}
	return res.res, res.err
}

func (c *BatchingClient) applyBatch(key clientBatchKey, items []clientBatchItem) []clientBatchResult {
	var reserveCount, cancelCount, commitCount, releaseCount int
	var counted int32
	var runID string
	var configItem *clientBatchItem
	for i := range items {
		item := &items[i]
		reserveCount += len(item.req.GetReserveSlots())
		cancelCount += len(item.req.GetCancelReservationSlots())
		commitCount += len(item.req.GetCommitSlots())
		releaseCount += len(item.req.GetReleaseSlots())
		counted += item.req.GetCountedReserves()
		if runID == "" {
			runID = item.req.GetRunId()
		}
		if item.req.GetConfigUpdate() != nil &&
			(configItem == nil || item.req.GetConfigUpdateVersion() > configItem.req.GetConfigUpdateVersion()) {
			configItem = item
		}
	}

	req := &fcpb.ConcurrencyServiceBatchRequest{
		NamespaceId: key.namespaceID,
		Key:         key.key,
		ClientId:    c.clientID,
		ReserveSeq:  c.nextReserveSeq(key, counted),
		RunId:       runID,
	}
	if reserveCount > 0 {
		req.ReserveSlots = make([]string, 0, reserveCount)
	}
	if cancelCount > 0 {
		req.CancelReservationSlots = make([]string, 0, cancelCount)
	}
	if commitCount > 0 {
		req.CommitSlots = make([]string, 0, commitCount)
	}
	if releaseCount > 0 {
		req.ReleaseSlots = make([]string, 0, releaseCount)
	}
	if configItem != nil {
		req.ConfigUpdate = configItem.req.GetConfigUpdate()
		req.ConfigUpdateVersion = configItem.req.GetConfigUpdateVersion()
	}
	for _, item := range items {
		req.ReserveSlots = append(req.ReserveSlots, item.req.GetReserveSlots()...)
		req.CancelReservationSlots = append(req.CancelReservationSlots, item.req.GetCancelReservationSlots()...)
		req.CommitSlots = append(req.CommitSlots, item.req.GetCommitSlots()...)
		req.ReleaseSlots = append(req.ReleaseSlots, item.req.GetReleaseSlots()...)
	}

	ctx := items[0].ctx
	for _, item := range items {
		if item.ctx.Err() == nil {
			ctx = item.ctx
			break
		}
	}
	res, err := c.ConcurrencyServiceClient.Batch(ctx, req)
	if err == nil && (res == nil || len(res.ReserveSuccess) != reserveCount || len(res.CommitSuccess) != commitCount) {
		err = serviceerror.NewInternal("invalid concurrency batch response")
	}
	if err != nil {
		return slices.Repeat([]clientBatchResult{{err: err}}, len(items))
	}

	// Report state once for the whole batch, before any caller sees its result.
	c.reportHint(key.namespaceID, key.key, res)

	results := make([]clientBatchResult, len(items))
	reserveOff := 0
	commitOff := 0
	for i, item := range items {
		reserveLen := len(item.req.GetReserveSlots())
		commitLen := len(item.req.GetCommitSlots())
		results[i].res = &fcpb.ConcurrencyServiceBatchResponse{
			ReserveSuccess: res.ReserveSuccess[reserveOff : reserveOff+reserveLen],
			CommitSuccess:  res.CommitSuccess[commitOff : commitOff+commitLen],
			RunId:          res.RunId,
		}
		reserveOff += reserveLen
		commitOff += commitLen
	}
	return results
}

func (c *BatchingClient) reportHint(namespaceID, key string, res *fcpb.ConcurrencyServiceBatchResponse) {
	if c.hintHandler != nil {
		c.hintHandler(namespaceID, key, res.GetGeneration(), res.GetAvailableSlotsHint(), res.GetAppliedReserveSeq(), res.GetRunId())
	}
}

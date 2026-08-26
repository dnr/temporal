package concurrency

import (
	"context"
	"sync/atomic"
	"time"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/server/chasm"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/contextutil"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/stream_batcher"
)

type batchKey struct {
	namespaceID string
	key         string
}

type batchReq struct {
	ctx context.Context
	req *fcpb.ConcurrencyBatchRequest
}

type batchRes struct {
	res *fcpb.ConcurrencyBatchResponse
	err error
}

type Handler struct {
	fcpb.UnimplementedConcurrencyServiceServer

	logger   log.Logger
	batchers *stream_batcher.KeyedBatcher[batchKey, batchReq, batchRes]
}

func NewHandler(
	logger log.Logger,
	dc *dynamicconfig.Collection,
) *Handler {
	h := &Handler{
		logger: logger,
	}
	h.batchers = stream_batcher.NewKeyedBatcherWithPerItemResults(
		h.applyBatch,
		ServerBatcherOptions.Get(dc)(),
		clock.NewRealTimeSource(),
	)
	return h
}

func initFn(_ chasm.MutableContext, items []batchReq) (*Component, error) {
	// For now (whole-queue limits), we should always have an initial config here.
	// For per-task limits, this may be missing and set via api later.
	c := &Component{
		ConcurrencyState: &fcpb.ConcurrencyState{
			Config: &taskqueuepb.ConcurrencyLimit{
				ConcurrentTasks: initialLimit,
			},
			WakeAll: true, // start will all slots available in generation 0
		},
	}
	for _, it := range items {
		c.updateConfig(it.req.GetConfigUpdate(), it.req.GetConfigUpdateVersion())
	}
	return c, nil
}

func updateFn(c *Component, cctx chasm.MutableContext, items []batchReq) ([]*fcpb.ConcurrencyBatchResponse, error) {
	now := cctx.Now(c)

	// allocate new protos for responses
	ress := make([]*fcpb.ConcurrencyBatchResponse, len(items))
	for i := range ress {
		ress[i] = &fcpb.ConcurrencyBatchResponse{}
	}

	// expire old reservations
	c.expire(now)

	// update config
	for _, it := range items {
		c.updateConfig(it.req.GetConfigUpdate(), it.req.GetConfigUpdateVersion())
	}

	// apply releases
	for _, it := range items {
		for _, slotId := range it.req.GetReleaseSlots() {
			// FIXME: add tasks to control slow release of notifications
			c.release(slotId)
		}
	}

	// apply commits
	for i, it := range items {
		for _, slotId := range it.req.GetCommitSlots() {
			success := c.commit(slotId)
			ress[i].CommitSuccess = append(ress[i].CommitSuccess, success)
		}
	}

	// apply cancels
	for _, it := range items {
		for _, slotId := range it.req.GetCancelReservationSlots() {
			c.cancelReservation(slotId)
		}
	}

	// apply reserves
	for i, it := range items {
		for _, slotId := range it.req.GetReserveSlots() {
			success := c.reserve(slotId, now)
			ress[i].ReserveSuccess = append(ress[i].ReserveSuccess, success)
		}
	}

	// capture Generation after c.reserve, which may increment it
	for i := range items {
		ress[i].Generation = c.Generation
	}

	return ress, nil
}

func (h *Handler) applyBatch(
	key batchKey,
	items []batchReq,
) []batchRes {
	needStart := false
	canSpeculate := true

	for _, it := range items {
		// only Reserve needs UpdateWithStart, other calls can assume it exists
		needStart = needStart || len(it.req.GetReserveSlots()) > 0

		// Commit and Release must be durable, Reserve and cancel may be speculative
		canSpeculate = canSpeculate && len(it.req.GetCommitSlots()) == 0 && len(it.req.GetReleaseSlots()) == 0
	}

	opts := make([]chasm.TransitionOption, 0, 2)
	if canSpeculate {
		opts = append(opts, chasm.WithSpeculative()) // note: not implemented yet
	}

	if needStart {
		opts = append(opts, chasm.WithBusinessIDPolicy(
			chasm.BusinessIDReusePolicyAllowDuplicate,
			chasm.BusinessIDConflictPolicyUseExisting,
		))
		updateRes, err := chasm.UpdateWithStartExecution(
			items[0].ctx,
			chasm.ExecutionKey{
				NamespaceID: key.namespaceID,
				BusinessID:  key.key,
			},
			initFn,
			updateFn,
			items,
			opts...,
		)
		return makeBatchResults(len(items), updateRes.UpdateOutput, err)
	}

	ress, _, err := chasm.UpdateComponent(
		items[0].ctx,
		chasm.NewComponentRef[*Component](chasm.ExecutionKey{
			NamespaceID: key.namespaceID,
			BusinessID:  key.key,
		}),
		updateFn,
		items,
		opts...,
	)
	return makeBatchResults(len(items), ress, err)
}

func makeBatchResults(
	size int,
	ress []*fcpb.ConcurrencyBatchResponse,
	err error,
) []batchRes {
	results := make([]batchRes, size)
	if err != nil {
		for i := range results {
			results[i].err = err
		}
	} else {
		for i := range results {
			if i < len(ress) {
				results[i].res = ress[i]
			}
		}
	}
	return results
}

func (h *Handler) Batch(ctx context.Context, req *fcpb.ConcurrencyBatchRequest) (retRes *fcpb.ConcurrencyBatchResponse, retErr error) {
	defer log.CapturePanic(h.logger, &retErr)

	res, err := h.batchers.Add(ctx, batchKey{
		namespaceID: req.GetNamespaceId(),
		key:         req.GetKey(),
	}, batchReq{ctx: ctx, req: req})
	if err != nil {
		return nil, err
	}
	return res.res, res.err
}

func (h *Handler) Wait(ctx context.Context, req *fcpb.ConcurrencyWaitRequest) (retRes *fcpb.ConcurrencyWaitResponse, retErr error) {
	defer log.CapturePanic(h.logger, &retErr)

	req = common.CloneProto(req)

	// TODO(fc): move to dynamic config
	ctx, cancel := contextutil.WithDeadlineBuffer(ctx, time.Minute, time.Second)
	defer cancel()

	// TODO(fc): do we actually need to return generation with timeout? if we don't, we'll
	// return zero and never get the right generation on the client
	var lastKnownGeneration atomic.Int64

	// loop to do internal retries with increased generation
	for {
		// CHASM requires that PollComponent be monotonic: if it returns true at some state,
		// then it should return true at future states. Our predicate is "is state.wake_up_to
		// >= req.start_time" (or state.wake_all means wake_upto = forever in the future). But
		// when we go from having slots to not having slots, we need to reset wake_up_to to
		// some time in the past, so that everyone is blocked again. So we need to add a
		// "generation" on top of the timestamps. Essentially, the state moves the pair
		// <generation, wake_up_to> monotonically.
		res, _, err := chasm.PollComponent(
			ctx,
			chasm.NewComponentRef[*Component](chasm.ExecutionKey{
				NamespaceID: req.NamespaceId,
				BusinessID:  req.Key,
			}),
			func(c *Component, cctx chasm.Context, req *fcpb.ConcurrencyWaitRequest) (*fcpb.ConcurrencyWaitResponse, bool, error) {
				generation, tokens, ready := c.poll(cctx.Now(c), req.Generation, req.StartTime.AsTime(), req.RequestedTokens)
				if !ready {
					lastKnownGeneration.Store(generation)
					return nil, false, nil
				}
				return &fcpb.ConcurrencyWaitResponse{Generation: generation, WakeTokens: tokens}, true, nil
			},
			req,
		)
		if err == nil && res == nil {
			// timed out without becoming ready
			return &fcpb.ConcurrencyWaitResponse{Generation: lastKnownGeneration.Load()}, nil
		} else if res.Generation != req.Generation {
			// try again on server with new generation
			req.Generation = res.Generation
			continue
		}
		return res, err
	}
}

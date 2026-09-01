package fc

import (
	"context"

	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/namespace"
	serviceerrors "go.temporal.io/server/common/serviceerror"
)

var errCommitFailure = serviceerror.NewFailedPrecondition("commit failed")

type committer interface {
	reserve() error
	commit() error
	cancelReservations()
}

type readinessCacheConcurrencyInterface interface {
	reportConcurrencyReady(namespace.ID, string, int64)
	reportConcurrencyBlocked(namespace.ID, string, int64)
}

// concurrency limits

type concurrencyCommitter struct {
	ctx    context.Context
	client fcpb.ConcurrencyServiceClient
	cache  readinessCacheConcurrencyInterface
	nsID   namespace.ID
	slotID string
	lim    Limiter
}

func newConcurrencyCommitter(
	ctx context.Context,
	client fcpb.ConcurrencyServiceClient,
	cache readinessCacheConcurrencyInterface,
	nsID namespace.ID,
	slotID string,
	lim Limiter,
) *concurrencyCommitter {
	return &concurrencyCommitter{
		ctx:    ctx,
		client: client,
		cache:  cache,
		nsID:   nsID,
		slotID: slotID,
		lim:    lim,
	}
}

func (c *concurrencyCommitter) reserve() error {
	// if config is missing or wrong type, just leave it out
	configUpdate, _ := c.lim.Config.(*taskqueuepb.ConcurrencyLimit)

	res, err := c.client.Batch(c.ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId:         c.nsID.String(),
		Key:                 c.lim.Key,
		ReserveSlots:        []string{c.slotID},
		ConfigUpdate:        configUpdate,
		ConfigUpdateVersion: c.lim.ConfigVersion,
	})
	if err != nil {
		return err // don't update cache on rpc error
	}
	if !res.ReserveSuccess[0] {
		c.cache.reportConcurrencyBlocked(c.nsID, c.lim.Key, res.Generation)
		return serviceerrors.NewFlowControlBlocked()
	}
	// TODO(fc): we could include a hint for how many slots are _remaining_, and if zero, mark
	// this limiter as blocked in the cache. but we don't want to immediately Wait on it since
	// we might not have another waiter yet.
	c.cache.reportConcurrencyReady(c.nsID, c.lim.Key, res.Generation)
	return nil
}

func (c *concurrencyCommitter) commit() error {
	res, err := c.client.Batch(c.ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.lim.Key,
		CommitSlots: []string{c.slotID},
	})
	if err != nil {
		return err
	} else if !res.CommitSuccess[0] {
		return errCommitFailure
	}
	return err
}

func (c *concurrencyCommitter) cancelReservations() {
	// call in new goroutine, don't block here, we don't care about the result
	go c.client.Batch(c.ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId:            c.nsID.String(),
		Key:                    c.lim.Key,
		CancelReservationSlots: []string{c.slotID},
	})
}

// local rate limits

type localLimiterCommitter struct {
	ll   LocalLimiter
	task any
}

func newLocalLimiterCommitter(
	lim Limiter,
) *localLimiterCommitter {
	ll, _ := lim.Config.(LocalLimiter)
	return &localLimiterCommitter{
		ll: ll,
	}
}

func (c *localLimiterCommitter) reserve() error {
	if c.ll == nil {
		return serviceerror.NewInternal("localRateLimitCommitter lim wrong type")
	}
	// by the time we get here, we already checked and allowed the right number of tasks to
	// match in the matcher, so just deduct the tokens.
	c.ll.Consume(1)
	return nil
}

func (c *localLimiterCommitter) commit() error {
	return nil
}

func (c *localLimiterCommitter) cancelReservations() {
	if c.ll == nil {
		return
	}
	c.ll.Consume(-1)
}

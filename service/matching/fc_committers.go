package matching

import (
	"context"

	enumsspb "go.temporal.io/server/api/enums/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/namespace"
)

type fcCommitter interface {
	Reserve() error
	Commit() error
	CancelReservations()
}

type fcReadinessCacheInterface interface {
	ReportReady(namespace.ID, enumsspb.LimiterType, string, int64)
	ReportBlocked(namespace.ID, enumsspb.LimiterType, string, int64)
}

type fcConcurrencyCommitter struct {
	ctx    context.Context
	client fcpb.ConcurrencyServiceClient
	cache  fcReadinessCacheInterface
	nsID   namespace.ID
	task   *internalTask
	key    string
}

func newFcConcurrencyCommitter(
	ctx context.Context,
	client fcpb.ConcurrencyServiceClient,
	cache fcReadinessCacheInterface,
	nsID namespace.ID,
	task *internalTask,
	key string,
) *fcConcurrencyCommitter {
	return &fcConcurrencyCommitter{
		ctx:    ctx,
		client: client,
		cache:  cache,
		nsID:   nsID,
		task:   task,
		key:    key,
	}
}

func (c *fcConcurrencyCommitter) Reserve() error {
	res, err := c.client.Reserve(c.ctx, &fcpb.ConcurrencyReserveRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.key,
		TaskUuid:    c.task.taskUUID(),
		// FIXME: LimitUpdate: get update in here
	})
	if err != nil {
		return err // don't update cache on rpc error
	}
	if res.SlotsReserved == 0 {
		c.cache.ReportBlocked(c.nsID, enumsspb.LIMITER_TYPE_CONCURRENCY, c.key, res.Generation)
		return errFCLimiterBlocked
	}
	c.cache.ReportReady(c.nsID, enumsspb.LIMITER_TYPE_CONCURRENCY, c.key, res.Generation)
	return nil
}

func (c *fcConcurrencyCommitter) Commit() error {
	_, err := c.client.Commit(c.ctx, &fcpb.ConcurrencyCommitRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.key,
		TaskUuid:    c.task.taskUUID(),
	})
	return err
}

func (c *fcConcurrencyCommitter) CancelReservations() {
	_, _ = c.client.CancelReservation(c.ctx, &fcpb.ConcurrencyCancelReservationRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.key,
		TaskUuid:    c.task.taskUUID(),
	})
}

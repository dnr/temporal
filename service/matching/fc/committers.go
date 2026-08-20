package fc

import (
	"context"

	enumsspb "go.temporal.io/server/api/enums/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/namespace"
)

type committer interface {
	Reserve() error
	Commit() error
	CancelReservations()
}

type readinessCacheInterface interface {
	reportReady(namespace.ID, enumsspb.LimiterType, string, int64)
	reportBlocked(namespace.ID, enumsspb.LimiterType, string, int64)
}

type concurrencyCommitter struct {
	ctx    context.Context
	client fcpb.ConcurrencyServiceClient
	cache  readinessCacheInterface
	nsID   namespace.ID
	task   fcTask
	key    string
}

func newConcurrencyCommitter(
	ctx context.Context,
	client fcpb.ConcurrencyServiceClient,
	cache readinessCacheInterface,
	nsID namespace.ID,
	task fcTask,
	key string,
) *concurrencyCommitter {
	return &concurrencyCommitter{
		ctx:    ctx,
		client: client,
		cache:  cache,
		nsID:   nsID,
		task:   task,
		key:    key,
	}
}

func (c *concurrencyCommitter) Reserve() error {
	res, err := c.client.Reserve(c.ctx, &fcpb.ConcurrencyReserveRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.key,
		TaskUuid:    c.task.TaskUUID(),
		// FIXME: LimitUpdate: get update in here
	})
	if err != nil {
		return err // don't update cache on rpc error
	}
	if res.SlotsReserved == 0 {
		c.cache.reportBlocked(c.nsID, enumsspb.LIMITER_TYPE_CONCURRENCY, c.key, res.Generation)
		return errFCLimiterBlocked
	}
	c.cache.reportReady(c.nsID, enumsspb.LIMITER_TYPE_CONCURRENCY, c.key, res.Generation)
	return nil
}

func (c *concurrencyCommitter) Commit() error {
	_, err := c.client.Commit(c.ctx, &fcpb.ConcurrencyCommitRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.key,
		TaskUuid:    c.task.TaskUUID(),
	})
	return err
}

func (c *concurrencyCommitter) CancelReservations() {
	_, _ = c.client.CancelReservation(c.ctx, &fcpb.ConcurrencyCancelReservationRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.key,
		TaskUuid:    c.task.TaskUUID(),
	})
}

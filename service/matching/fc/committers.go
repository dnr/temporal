package fc

import (
	"context"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/namespace"
)

type committer interface {
	reserve() error
	commit() error
	cancelReservations()
}

type readinessCacheInterface interface {
	reportReady(namespace.ID, enumsspb.LimiterType, string, int64)
	reportBlocked(namespace.ID, enumsspb.LimiterType, string, int64)
}

type concurrencyCommitter struct {
	ctx      context.Context
	client   fcpb.ConcurrencyServiceClient
	cache    readinessCacheInterface
	nsID     namespace.ID
	taskUUID string
	lim      limiter
	// TODO(fc): we could maybe have this component do some opportunistic batching
}

func newConcurrencyCommitter(
	ctx context.Context,
	client fcpb.ConcurrencyServiceClient,
	cache readinessCacheInterface,
	nsID namespace.ID,
	taskUUID string,
	lim limiter,
) *concurrencyCommitter {
	return &concurrencyCommitter{
		ctx:      ctx,
		client:   client,
		cache:    cache,
		nsID:     nsID,
		taskUUID: taskUUID,
		lim:      lim,
	}
}

func (c *concurrencyCommitter) reserve() error {
	// if config is missing or wrong type, just leave it out
	configUpdate, _ := c.lim.config.(*taskqueuepb.ConcurrencyLimit)

	res, err := c.client.Reserve(c.ctx, &fcpb.ConcurrencyReserveRequest{
		NamespaceId:         c.nsID.String(),
		Key:                 c.lim.key,
		TaskUuid:            c.taskUUID,
		ConfigUpdate:        configUpdate,
		ConfigUpdateVersion: c.lim.configVersion,
	})
	if err != nil {
		return err // don't update cache on rpc error
	}
	if res.SlotsReserved == 0 {
		c.cache.reportBlocked(c.nsID, c.lim.tp, c.lim.key, res.Generation)
		return errFCLimiterBlocked
	}
	c.cache.reportReady(c.nsID, c.lim.tp, c.lim.key, res.Generation)
	return nil
}

func (c *concurrencyCommitter) commit() error {
	_, err := c.client.Commit(c.ctx, &fcpb.ConcurrencyCommitRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.lim.key,
		TaskUuid:    c.taskUUID,
	})
	return err
}

func (c *concurrencyCommitter) cancelReservations() {
	_, _ = c.client.CancelReservation(c.ctx, &fcpb.ConcurrencyCancelReservationRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.lim.key,
		TaskUuid:    c.taskUUID,
	})
}

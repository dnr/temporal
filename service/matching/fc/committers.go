package fc

import (
	"context"

	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
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

type readinessCacheInterface interface {
	reportReady(namespace.ID, enumsspb.LimiterType, string, int64)
	reportBlocked(namespace.ID, enumsspb.LimiterType, string, int64)
}

type concurrencyCommitter struct {
	ctx    context.Context
	client fcpb.ConcurrencyServiceClient
	cache  readinessCacheInterface
	nsID   namespace.ID
	slotID string
	lim    limiter
}

func newConcurrencyCommitter(
	ctx context.Context,
	client fcpb.ConcurrencyServiceClient,
	cache readinessCacheInterface,
	nsID namespace.ID,
	slotID string,
	lim limiter,
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
	configUpdate, _ := c.lim.config.(*taskqueuepb.ConcurrencyLimit)

	res, err := c.client.Batch(c.ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId:         c.nsID.String(),
		Key:                 c.lim.key,
		ReserveSlots:        []string{c.slotID},
		ConfigUpdate:        configUpdate,
		ConfigUpdateVersion: c.lim.configVersion,
	})
	if err != nil {
		return err // don't update cache on rpc error
	}
	if !res.ReserveSuccess[0] {
		c.cache.reportBlocked(c.nsID, c.lim.tp, c.lim.key, res.Generation)
		return serviceerrors.NewFlowControlBlocked()
	}
	c.cache.reportReady(c.nsID, c.lim.tp, c.lim.key, res.Generation)
	return nil
}

func (c *concurrencyCommitter) commit() error {
	res, err := c.client.Batch(c.ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.lim.key,
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
	_, _ = c.client.Batch(c.ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId:            c.nsID.String(),
		Key:                    c.lim.key,
		CancelReservationSlots: []string{c.slotID},
	})
}

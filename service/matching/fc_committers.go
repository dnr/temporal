package matching

import (
	"context"

	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/namespace"
)

type fcCommitter interface {
	Reserve() error
	CancelReservations()
	Commit() error
}

type fcConcurrencyCommitter struct {
	ctx    context.Context
	client fcpb.ConcurrencyServiceClient
	nsID   namespace.ID
	task   *internalTask
	key    string
}

func newFcConcurrencyCommitter(
	ctx context.Context,
	client fcpb.ConcurrencyServiceClient,
	nsID namespace.ID,
	task *internalTask,
	key string,
) *fcConcurrencyCommitter {
	return &fcConcurrencyCommitter{
		ctx:    ctx,
		client: client,
		nsID:   nsID,
		task:   task,
		key:    key,
	}
}

func (c *fcConcurrencyCommitter) Reserve() error {
	_, err := c.client.Reserve(c.ctx, &fcpb.ConcurrencyReserveRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.key,
		TaskUuid:    c.task.taskUUID(),
		// FIXME: LimitUpdate: get update in here
	})
	// FIXME: get error into readiness cache
	return err
}

func (c *fcConcurrencyCommitter) CancelReservations() {
	_, _ = c.client.CancelReservation(c.ctx, &fcpb.ConcurrencyCancelReservationRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.key,
		TaskUuid:    c.task.taskUUID(),
	})
}

func (c *fcConcurrencyCommitter) Commit() error {
	_, err := c.client.Commit(c.ctx, &fcpb.ConcurrencyCommitRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.key,
		TaskUuid:    c.task.taskUUID(),
	})
	return err
}

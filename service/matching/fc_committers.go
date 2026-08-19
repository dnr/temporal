package matching

import (
	"context"

	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
)

type fcCommitter interface {
	Reserve() error
	CancelReservations()
	Commit() error
}

type fcConcurrencyCommitter struct {
	client fcpb.ConcurrencyServiceClient
	ctx    context.Context
	task   *internalTask
	key    string
}

func (c *fcConcurrencyCommitter) Reserve() error {
	_, err := c.client.Reserve(c.ctx, &fcpb.ConcurrencyReserveRequest{
		NamespaceId: c.task.namespace, // FIXME: ID not name!!!
		Key:         c.key,
		TaskUuid:    c.task.taskUUID(),
		// FIXME: LimitUpdate: get update in here
	})
	return err
}

func (c *fcConcurrencyCommitter) CancelReservations() {
	_, _ = c.client.CancelReservation(c.ctx, &fcpb.ConcurrencyCancelReservationRequest{
		NamespaceId: c.task.namespace, // FIXME: ID not name!!!
		Key:         c.key,
		TaskUuid:    c.task.taskUUID(),
	})
}

func (c *fcConcurrencyCommitter) Commit() error {
	_, err := c.client.Commit(c.ctx, &fcpb.ConcurrencyCommitRequest{
		NamespaceId: c.task.namespace, // FIXME: ID not name!!!
		Key:         c.key,
		TaskUuid:    c.task.taskUUID(),
	})
	return err
}

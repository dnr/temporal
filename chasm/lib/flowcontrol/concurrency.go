package flowcontrol

import (
	"go.temporal.io/server/chasm"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
)

type concurrency struct {
	chasm.UnimplementedComponent

	*fcpb.ConcurrencyState
}

func (c *concurrency) LifecycleState(_ chasm.Context) chasm.LifecycleState {
	// TODO(fc): we should be able to clean these up if they've been idle a while
	return chasm.LifecycleStateRunning
}

func (c *concurrency) Terminate(_ chasm.MutableContext, _ chasm.TerminateComponentRequest) (chasm.TerminateComponentResponse, error) {
	// TODO(fc): can we block this if there are any committed slots?
	return chasm.TerminateComponentResponse{}, nil
}

func (c *concurrency) ContextMetadata(_ chasm.Context) map[string]string {
	return nil
}

func (c *concurrency) reserve(taskUUID string) error {
}

func (c *concurrency) cancelReservation(taskUUID string) error {
}

func (c *concurrency) commit(taskUUID string) error {
}

func (c *concurrency) release(taskUUID string) error {
}

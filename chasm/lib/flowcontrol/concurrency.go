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

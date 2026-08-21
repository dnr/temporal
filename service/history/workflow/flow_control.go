package workflow

import (
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/definition"
	commontaskqueue "go.temporal.io/server/common/taskqueue"
	historyi "go.temporal.io/server/service/history/interfaces"
	"go.temporal.io/server/service/history/tasks"
)

func addReleaseLimiterTask(
	mutableState historyi.MutableState,
	limiters []*taskqueuespb.LimiterRef,
) {
	task := newReleaseLimiterTask(mutableState.GetWorkflowKey(), limiters)
	if task != nil {
		mutableState.AddTasks(task)
	}
}

func newReleaseLimiterTask(
	workflowKey definition.WorkflowKey,
	limiters []*taskqueuespb.LimiterRef,
) *tasks.ReleaseLimiterTask {
	var toRelease []*taskqueuespb.LimiterRef
	for _, limiter := range limiters {
		if commontaskqueue.NeedsRelease(limiter) {
			if toRelease == nil {
				toRelease = make([]*taskqueuespb.LimiterRef, 0, len(limiters))
			}
			toRelease = append(toRelease, limiter)
		}
	}
	if len(toRelease) == 0 {
		return nil
	}

	return &tasks.ReleaseLimiterTask{
		WorkflowKey: workflowKey,
		Limiters:    toRelease,
	}
}

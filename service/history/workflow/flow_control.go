package workflow

import (
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/definition"
	commontaskqueue "go.temporal.io/server/common/taskqueue"
	"go.temporal.io/server/service/history/tasks"
)

func (ms *MutableStateImpl) trackRemovedLimiterRefs(
	previous []*taskqueuespb.LimiterRef,
	current []*taskqueuespb.LimiterRef,
) {
	for _, previousRef := range previous {
		if !commontaskqueue.NeedsRelease(previousRef) || containsLimiterRef(current, previousRef) {
			continue
		}
		ms.releaseLimiterRefs = append(ms.releaseLimiterRefs, previousRef)
	}
}

func containsLimiterRef(
	refs []*taskqueuespb.LimiterRef,
	want *taskqueuespb.LimiterRef,
) bool {
	for _, ref := range refs {
		if ref.GetLimiterType() == want.GetLimiterType() &&
			ref.GetKey() == want.GetKey() &&
			ref.GetSlotId() == want.GetSlotId() {
			return true
		}
	}
	return false
}

// closeTransactionGenerateReleaseLimiterTask generates the task that releases the flow control
// slots this transaction stopped using.
//
// The task is generated under both the active and the passive transaction policy, following the
// same pattern as the tasks that notify a parent workflow of a child's completion: the active
// cluster performs the release, while the standby cluster keeps its own copy of the task until it
// can confirm that the release has replicated to its copy of the limiter (see
// transferQueueStandbyTaskExecutor.processReleaseLimiterTask). Without the standby copy, a
// failover between generating the task and releasing the slot would leave the slot committed
// forever, because the limiter lives on a different shard than the execution holding the slot and
// nothing else would ever notice the discrepancy.
//
// The refs come from the transitions this transaction applied (see trackRemovedLimiterRefs) rather
// than from durable state, which leaves one narrow gap on the passive side: if replication
// collapses an activity's start and close into a single mutation, the standby never sees the
// ActivityInfo that carried the refs and so generates no task for it. Slots lost that way still
// need a failover in the same window to actually leak, since the active cluster releases them
// normally.
// TODO(fc): close that gap, e.g. by tracking pending releases in mutable state.
func (ms *MutableStateImpl) closeTransactionGenerateReleaseLimiterTask() {
	limiters := ms.releaseLimiterRefs
	if ms.executionState.State == enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED &&
		ms.stateInDB != enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED {
		// The execution is closing in this transaction: anything still holding a slot is
		// abandoned now, so release those too.
		limiters = append(limiters, ms.executionInfo.WorkflowTaskLimiters...)
		for _, activityInfo := range ms.pendingActivityInfoIDs {
			limiters = append(limiters, activityInfo.Limiters...)
		}
	}
	if task := newReleaseLimiterTask(ms.GetWorkflowKey(), limiters); task != nil {
		ms.AddTasks(task)
	}
}

func newReleaseLimiterTask(
	workflowKey definition.WorkflowKey,
	limiters []*taskqueuespb.LimiterRef,
) *tasks.ReleaseLimiterTask {
	var toRelease []*taskqueuespb.LimiterRef
	for _, limiter := range limiters {
		// The same ref can be collected twice, e.g. when a transaction both closes the
		// execution and deletes a pending activity that was holding a slot. Releasing twice is
		// harmless but there's no reason to carry the duplicate around.
		if commontaskqueue.NeedsRelease(limiter) && !containsLimiterRef(toRelease, limiter) {
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

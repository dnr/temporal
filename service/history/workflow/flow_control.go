package workflow

import (
	"slices"

	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/definition"
	commontaskqueue "go.temporal.io/server/common/taskqueue"
	historyi "go.temporal.io/server/service/history/interfaces"
	"go.temporal.io/server/service/history/tasks"
)

func (ms *MutableStateImpl) trackRemovedLimiterRefs(
	previous []*taskqueuespb.LimiterRef,
	current []*taskqueuespb.LimiterRef,
) {
	for _, previousRef := range previous {
		if !commontaskqueue.NeedsRelease(previousRef) || commontaskqueue.ContainsLimiterRef(current, previousRef) {
			continue
		}
		ms.releaseLimiterRefs = append(ms.releaseLimiterRefs, previousRef)
	}
}

// MarkLimitersReleased records limiter refs that were already released outside of this
// transaction (e.g. by frontend before RespondActivityTaskCompleted), so that no release task is
// generated for them.
func (ms *MutableStateImpl) MarkLimitersReleased(refs []*taskqueuespb.LimiterRef) {
	ms.preReleasedLimiterRefs = append(ms.preReleasedLimiterRefs, refs...)
}

func (ms *MutableStateImpl) closeTransactionGenerateReleaseLimiterTask(
	transactionPolicy historyi.TransactionPolicy,
) {
	if transactionPolicy != historyi.TransactionPolicyActive {
		return
	}

	limiters := ms.releaseLimiterRefs
	if ms.executionState.State == enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED &&
		ms.stateInDB != enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED {
		limiters = append(limiters, ms.executionInfo.WorkflowTaskLimiters...)
		for _, activityInfo := range ms.pendingActivityInfoIDs {
			limiters = append(limiters, activityInfo.Limiters...)
		}
	}
	if len(ms.preReleasedLimiterRefs) > 0 {
		limiters = slices.DeleteFunc(slices.Clone(limiters), func(ref *taskqueuespb.LimiterRef) bool {
			return commontaskqueue.ContainsLimiterRef(ms.preReleasedLimiterRefs, ref)
		})
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
		if commontaskqueue.NeedsRelease(limiter) {
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

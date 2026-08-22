package workflow

import (
	"bytes"
	"slices"

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
		if !commontaskqueue.NeedsRelease(previousRef) || commontaskqueue.ContainsLimiterRef(current, previousRef) {
			continue
		}
		ms.releaseLimiterRefs = append(ms.releaseLimiterRefs, previousRef)
	}
}

func (ms *MutableStateImpl) closeTransactionGenerateReleaseLimiterTask() {
	limiters := ms.releaseLimiterRefs
	if ms.executionState.State == enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED &&
		ms.stateInDB != enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED {
		limiters = append(limiters, ms.executionInfo.WorkflowTaskLimiters...)
		for _, activityInfo := range ms.pendingActivityInfoIDs {
			limiters = append(limiters, activityInfo.Limiters...)
		}
	}
	if task := newReleaseLimiterTask(ms.GetWorkflowKey(), limiters); task != nil {
		oldSize := ms.executionInfo.Size()
		for _, release := range task.Releases {
			if commontaskqueue.FindLimiterRelease(ms.executionInfo.PendingLimiterReleases, release.GetLimiter()) == nil {
				ms.executionInfo.PendingLimiterReleases = append(ms.executionInfo.PendingLimiterReleases, release)
				ms.releaseLimiterTasks = append(ms.releaseLimiterTasks, release)
				ms.pendingLimiterReleasesUpdated = true
			}
		}
		ms.approximateSize += ms.executionInfo.Size() - oldSize
	}

	if len(ms.releaseLimiterTasks) > 0 {
		ms.AddTasks(&tasks.ReleaseLimiterTask{
			WorkflowKey: ms.GetWorkflowKey(),
			Releases:    ms.releaseLimiterTasks,
		})
	}
}

func (ms *MutableStateImpl) RecordLimiterRelease(releases []*taskqueuespb.LimiterRelease) {
	oldSize := ms.executionInfo.Size()
	for _, release := range releases {
		pending := commontaskqueue.FindLimiterRelease(
			ms.executionInfo.PendingLimiterReleases,
			release.GetLimiter(),
		)
		if pending == nil {
			ms.executionInfo.PendingLimiterReleases = append(ms.executionInfo.PendingLimiterReleases, release)
			ms.releaseLimiterTasks = append(ms.releaseLimiterTasks, release)
			ms.pendingLimiterReleasesUpdated = true
		} else if !bytes.Equal(pending.GetComponentRef(), release.GetComponentRef()) {
			pending.ComponentRef = release.GetComponentRef()
			ms.releaseLimiterTasks = append(ms.releaseLimiterTasks, release)
			ms.pendingLimiterReleasesUpdated = true
		}
	}
	ms.approximateSize += ms.executionInfo.Size() - oldSize
}

func (ms *MutableStateImpl) CompleteLimiterRelease(releases []*taskqueuespb.LimiterRelease) {
	oldSize := ms.executionInfo.Size()
	oldLen := len(ms.executionInfo.PendingLimiterReleases)
	ms.executionInfo.PendingLimiterReleases = slices.DeleteFunc(
		ms.executionInfo.PendingLimiterReleases,
		func(pending *taskqueuespb.LimiterRelease) bool {
			return commontaskqueue.FindLimiterRelease(releases, pending.GetLimiter()) != nil
		},
	)
	if len(ms.executionInfo.PendingLimiterReleases) != oldLen {
		ms.pendingLimiterReleasesUpdated = true
		ms.approximateSize += ms.executionInfo.Size() - oldSize
	}
}

func newReleaseLimiterTask(
	workflowKey definition.WorkflowKey,
	limiters []*taskqueuespb.LimiterRef,
) *tasks.ReleaseLimiterTask {
	var releases []*taskqueuespb.LimiterRelease
	for _, limiter := range limiters {
		if commontaskqueue.NeedsRelease(limiter) {
			releases = append(releases, &taskqueuespb.LimiterRelease{Limiter: limiter})
		}
	}
	if len(releases) == 0 {
		return nil
	}

	return &tasks.ReleaseLimiterTask{
		WorkflowKey: workflowKey,
		Releases:    releases,
	}
}

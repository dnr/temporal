package history

import (
	"context"

	persistencespb "go.temporal.io/server/api/persistence/v1"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
	"go.temporal.io/server/service/history/api"
	"go.temporal.io/server/service/history/consts"
	"go.temporal.io/server/service/history/queues"
	"go.temporal.io/server/service/history/tasks"
	"google.golang.org/protobuf/proto"
)

// preReleaseForActivityTimeout is the Release-first half of the activity
// timeout handler. It identifies which attempts this timer task is about
// to terminate, snapshots their ActivityInfos, drops the workflow lease,
// and then Releases each holder's semaphore slot.
//
// On nil client (placeholder, today): returns immediately without
// acquiring any lease.
//
// The function does NOT mutate workflow state. The actual timer
// processing happens later in executeActivityTimeoutTask, which acquires
// a fresh lease. If the Release succeeds but the subsequent update fails,
// the timer task is retried by the queue; the retry's pre-release reads
// the current state, finds nothing to release (slot already gone or
// activity gone), and proceeds to the update phase.
func (t *timerQueueActiveTaskExecutor) preReleaseForActivityTimeout(
	ctx context.Context,
	task *tasks.ActivityTimeoutTask,
	client semaphorepb.SemaphoreServiceClient,
) error {
	if client == nil {
		return nil
	}

	snapshots, err := t.readTimedOutActivities(ctx, task)
	if err != nil || len(snapshots) == 0 {
		return err
	}

	workflowKey := taskWorkflowKey(task)
	for _, ai := range snapshots {
		if err := api.ReleaseActivitySemaphore(
			ctx, client,
			workflowKey.NamespaceID, workflowKey.WorkflowID, workflowKey.RunID,
			ai,
		); err != nil {
			return err
		}
	}
	return nil
}

// readTimedOutActivities loads mutable state under a read lease, walks
// the activity timer firings the same way executeActivityTimeoutTask does,
// and returns deduplicated ActivityInfo snapshots for the activities
// whose current attempt this task will terminate.
func (t *timerQueueActiveTaskExecutor) readTimedOutActivities(
	ctx context.Context,
	task *tasks.ActivityTimeoutTask,
) (_ []*persistencespb.ActivityInfo, retErr error) {
	weContext, release, err := getWorkflowExecutionContextForTask(ctx, t.shardContext, t.cache, task)
	if err != nil {
		return nil, err
	}
	// Release with nil so a downstream RPC failure cannot evict the
	// mutable-state cache.
	defer func() { release(retErr) }()

	mutableState, err := loadMutableStateForTimerTask(ctx, t.shardContext, weContext, task, t.metricsHandler, t.logger)
	if err != nil {
		return nil, err
	}
	if mutableState == nil {
		return nil, consts.ErrWorkflowExecutionNotFound
	}
	if !mutableState.IsWorkflowExecutionRunning() {
		return nil, consts.ErrWorkflowCompleted
	}

	timerSequence := t.getTimerSequence(mutableState)
	referenceTime := mutableState.Now()
	seen := make(map[int64]struct{})
	var snapshots []*persistencespb.ActivityInfo

	for _, tsid := range timerSequence.LoadAndSortActivityTimers() {
		// Timer sequence IDs are sorted; once we hit one that isn't
		// expired, none after it are either.
		if !queues.IsTimeExpired(task, referenceTime, tsid.Timestamp) {
			break
		}
		ai, ok := mutableState.GetActivityInfo(tsid.EventID)
		if !ok {
			continue
		}
		// Stale timer for a previous attempt — the new attempt holds its
		// own slot (or none yet).
		if tsid.Attempt < ai.Attempt {
			continue
		}
		// One activity may have multiple timers fire on the same task;
		// only release once per ScheduledEventId.
		if _, dup := seen[ai.ScheduledEventId]; dup {
			continue
		}
		seen[ai.ScheduledEventId] = struct{}{}
		snapshots = append(snapshots, proto.Clone(ai).(*persistencespb.ActivityInfo))
	}
	return snapshots, nil
}

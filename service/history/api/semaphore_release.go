package api

import (
	"context"

	persistencespb "go.temporal.io/server/api/persistence/v1"
	tokenspb "go.temporal.io/server/api/token/v1"
	chasmsemaphore "go.temporal.io/server/chasm/lib/semaphore"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/definition"
	"go.temporal.io/server/common/locks"
	historyi "go.temporal.io/server/service/history/interfaces"
	"google.golang.org/protobuf/proto"
)

// SemaphoreIDForActivity returns the semaphore ID (if any) gating dispatch
// of this activity. The id was stamped onto the ActivityInfo at the time
// the activity was scheduled.
//
// Placeholder: always returns ("", false) so existing activity completion
// paths are unchanged. Replace with the real lookup once the `semaphore_id`
// field is added to ActivityInfo.
//
//nolint:unparam // placeholder until proto field exists
func SemaphoreIDForActivity(_ *persistencespb.ActivityInfo) (string, bool) {
	return "", false
}

// ReleaseActivitySemaphore frees the semaphore slot for one activity
// attempt. The slot is held per-attempt, so this is called whenever an
// attempt ends — completion, failure (retrying or terminal), cancellation,
// or timeout.
//
// Durability contract: the call blocks until the semaphore component has
// persisted the Release transition. When client is nil or the activity
// has no associated semaphore, the call is a no-op.
func ReleaseActivitySemaphore(
	ctx context.Context,
	client semaphorepb.SemaphoreServiceClient,
	namespaceID string,
	workflowID string,
	runID string,
	ai *persistencespb.ActivityInfo,
) error {
	if client == nil || ai == nil {
		return nil
	}
	semaphoreID, ok := SemaphoreIDForActivity(ai)
	if !ok {
		return nil
	}
	holderID := chasmsemaphore.HolderIDForTask(chasmsemaphore.TaskKey{
		NamespaceID:      namespaceID,
		TaskQueue:        ai.GetTaskQueue(),
		TaskQueueKind:    0, // TODO: include kind once integration is finalized
		WorkflowID:       workflowID,
		RunID:            runID,
		ScheduledEventID: ai.GetScheduledEventId(),
	})
	_, err := client.Release(ctx, &semaphorepb.ReleaseRequest{
		NamespaceId: namespaceID,
		SemaphoreId: semaphoreID,
		HolderId:    holderID,
	})
	return err
}

// PreReleaseActivitySemaphore performs a Release-first activity-semaphore
// release: it reads the ActivityInfo with a read-only lease, then calls
// Release before the caller's workflow update transaction runs.
//
// Why Release-first (hard semantics): if we released after the workflow
// update, then a Release failure after a successful update would return
// an error to the worker; the worker retries the API and the second call
// sees ActivityTaskNotFound (workflow already past this activity), never
// reaches the Release path, and the slot leaks forever — Committed slots
// have no TTL. Doing the Release first means a retry re-attempts both
// steps idempotently until both succeed.
//
// Placeholder optimization: when client is nil (the current state, until
// the SemaphoreServiceClient is plumbed into history), this returns
// immediately without doing any reads. Same when SemaphoreIDForActivity
// reports no semaphore for the activity.
func PreReleaseActivitySemaphore(
	ctx context.Context,
	shardContext historyi.ShardContext,
	consistencyChecker WorkflowConsistencyChecker,
	client semaphorepb.SemaphoreServiceClient,
	workflowKey definition.WorkflowKey,
	token *tokenspb.Task,
) (retErr error) {
	if client == nil {
		return nil
	}

	lease, err := consistencyChecker.GetWorkflowLease(
		ctx, nil, workflowKey, locks.PriorityHigh,
	)
	if err != nil {
		return err
	}
	defer func() { lease.GetReleaseFn()(retErr) }()

	mutableState, err := lease.GetContext().LoadMutableState(ctx, shardContext)
	if err != nil {
		return err
	}

	scheduledEventID := token.GetScheduledEventId()
	if scheduledEventID == common.EmptyEventID {
		scheduledEventID, err = GetActivityScheduledEventID(token.GetActivityId(), mutableState)
		if err != nil {
			return err
		}
	}

	ai, ok := mutableState.GetActivityInfo(scheduledEventID)
	if !ok {
		// Activity is already gone — nothing to release. The next caller
		// will see the same state and also no-op.
		return nil
	}

	// Snapshot the AI before calling Release; we want to drop the workflow
	// lease as soon as possible.
	aiSnapshot := proto.Clone(ai).(*persistencespb.ActivityInfo)
	return ReleaseActivitySemaphore(
		ctx, client,
		workflowKey.NamespaceID, workflowKey.WorkflowID, workflowKey.RunID,
		aiSnapshot,
	)
}

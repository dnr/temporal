package api

import (
	"context"

	persistencespb "go.temporal.io/server/api/persistence/v1"
	chasmsemaphore "go.temporal.io/server/chasm/lib/semaphore"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
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

// ReleaseActivitySemaphore frees the semaphore slot held by a terminal
// activity. It must be called from any path that transitions an activity
// out of a running state — RespondActivityTask{Completed,Failed,Canceled},
// activity timeouts, workflow termination — and the calling handler must
// not return success until this call has succeeded.
//
// Durability contract: the call blocks until the semaphore component has
// persisted the Release transition. The caller is responsible for
// sequencing this after the workflow's own state update has committed.
//
// When client is nil or the activity has no associated semaphore, the
// call is a no-op.
//
// TODO: plumb a SemaphoreServiceClient into the history engine and each
// Respond/timer handler so this can actually fire. For now the call sites
// pass nil and this is effectively dead code.
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

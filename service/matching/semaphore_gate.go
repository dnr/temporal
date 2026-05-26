package matching

import (
	"context"
	"errors"

	"go.temporal.io/api/serviceerror"
	chasmsemaphore "go.temporal.io/server/chasm/lib/semaphore"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

// activitySemaphoreGate wraps the Reserve/Commit/Unreserve dance around an
// activity dispatch.
//
// Lifecycle on a poll that has an attached semaphore:
//
//	g := beginActivitySemaphoreGate(ctx, engine, task)
//	if g != nil { defer g.unreserveIfNotCommitted(ctx) }
//	if !g.reserve(ctx) { /* skip task */ }
//	resp, err := recordActivityTaskStarted(...)
//	if err == nil { g.commit(ctx) }
//
// The gate is nil when the task has no associated semaphore, in which case
// callers should treat it as a no-op and follow the existing dispatch path.
type activitySemaphoreGate struct {
	client      semaphorepb.SemaphoreServiceClient
	logger      log.Logger
	namespaceID string
	semaphoreID string
	holderID    string
	committed   bool
}

// beginActivitySemaphoreGate returns a non-nil gate iff (a) the task carries
// a semaphoreID and (b) the engine has a semaphore client configured. The
// returned gate has NOT yet reserved a slot — call reserve() next.
func beginActivitySemaphoreGate(
	engine *matchingEngineImpl,
	task *internalTask,
) *activitySemaphoreGate {
	if engine.semaphoreClient == nil {
		return nil
	}
	semaphoreID, ok := task.semaphoreID()
	if !ok {
		return nil
	}
	return &activitySemaphoreGate{
		client:      engine.semaphoreClient,
		logger:      engine.logger,
		namespaceID: task.event.Data.GetNamespaceId(),
		semaphoreID: semaphoreID,
		holderID:    holderIDForTask(task),
	}
}

// reserve blocks until a slot is reserved (or already committed for this id)
// or the caller's context expires. Returns true on success.
//
// Per the current design: matching retries Reserve forever for the lifetime
// of the poll. Any non-deadline error bubbles up and the poller treats the
// task as failed.
func (g *activitySemaphoreGate) reserve(ctx context.Context) error {
	if g == nil {
		return nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		resp, err := g.client.Reserve(ctx, &semaphorepb.ReserveRequest{
			NamespaceId: g.namespaceID,
			SemaphoreId: g.semaphoreID,
			HolderId:    g.holderID,
		})
		if err != nil {
			// Retry on transient/unavailable errors; surface anything else.
			var unavailable *serviceerror.Unavailable
			if errors.As(err, &unavailable) {
				continue
			}
			return err
		}
		if resp.GetReserved() || resp.GetAlreadyCommitted() {
			if resp.GetAlreadyCommitted() {
				g.committed = true // skip the Commit step
			}
			return nil
		}
		// Reserved=false, AlreadyCommitted=false: the semaphore handler
		// timed out its long-poll. Try again immediately.
	}
}

// commit promotes the reservation. Idempotent in the AlreadyCommitted case.
func (g *activitySemaphoreGate) commit(ctx context.Context) error {
	if g == nil || g.committed {
		return nil
	}
	_, err := g.client.Commit(ctx, &semaphorepb.CommitRequest{
		NamespaceId: g.namespaceID,
		SemaphoreId: g.semaphoreID,
		HolderId:    g.holderID,
	})
	if err == nil {
		g.committed = true
		return nil
	}
	// If the reservation expired between Reserve and Commit, treat it as
	// an unfortunate-but-recoverable case: the task is effectively lost
	// from matching's POV and will be re-driven by activity timeouts.
	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		g.logger.Warn("semaphore commit found no reservation; activity will be re-driven on timeout",
			tag.NewStringTag("semaphore-id", g.semaphoreID),
			tag.NewStringTag("holder-id", g.holderID),
		)
		return err
	}
	return err
}

// unreserveIfNotCommitted is a defer-friendly cleanup: if we Reserved but
// never Committed, send an Unreserve so the slot doesn't sit for the full
// TTL. Best effort — context may already be canceled.
func (g *activitySemaphoreGate) unreserveIfNotCommitted(ctx context.Context) {
	if g == nil || g.committed {
		return
	}
	// Use a fresh context detached from the (possibly canceled) caller's,
	// so cleanup still runs after a deadline. Borrow the engine's
	// background, capped.
	cleanupCtx := context.WithoutCancel(ctx)
	_, err := g.client.Unreserve(cleanupCtx, &semaphorepb.UnreserveRequest{
		NamespaceId: g.namespaceID,
		SemaphoreId: g.semaphoreID,
		HolderId:    g.holderID,
	})
	if err != nil {
		g.logger.Warn("best-effort semaphore Unreserve failed; slot will expire via TTL",
			tag.NewStringTag("semaphore-id", g.semaphoreID),
			tag.NewStringTag("holder-id", g.holderID),
			tag.Error(err),
		)
	}
}

// holderIDForTask derives the deterministic semaphore holder id for an
// internalTask. Kept here rather than on internalTask itself so the task
// type stays free of semaphore-package imports.
func holderIDForTask(task *internalTask) string {
	d := task.event.Data
	tqName, tqKind := taskQueueIdentity(task)
	return chasmsemaphore.HolderIDForTask(chasmsemaphore.TaskKey{
		NamespaceID:      d.GetNamespaceId(),
		TaskQueue:        tqName,
		TaskQueueKind:    tqKind,
		WorkflowID:       d.GetWorkflowId(),
		RunID:            d.GetRunId(),
		ScheduledEventID: d.GetScheduledEventId(),
	})
}

// taskQueueIdentity is a placeholder that returns ("", 0) until the real
// task queue identity is plumbed through. The hash is still deterministic
// per workflow/run/activity so retries land on the same holder id.
func taskQueueIdentity(task *internalTask) (string, int32) {
	// TODO: derive the dispatch task queue from the partition or task data
	// once the integration design is finalized.
	return "", 0
}

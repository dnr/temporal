package semaphore

import (
	"crypto/sha256"
	"encoding/hex"
)

// TaskKey identifies the task that's being gated by a semaphore. It mirrors
// the fields that matching uses to dispatch an activity task. The semaphore
// HolderID is derived deterministically from these so that retries produce
// the same id.
//
// ScheduledEventID rather than ActivityID is used because matching's
// TaskInfo doesn't carry the user-facing ActivityID; the scheduled-event id
// uniquely identifies the activity instance within a workflow and is
// available on both matching and history sides.
type TaskKey struct {
	NamespaceID      string
	TaskQueue        string
	TaskQueueKind    int32
	WorkflowID       string
	RunID            string
	ScheduledEventID int64
}

// SemaphoreBinding describes which semaphore (if any) gates a task, and the
// holder id matching/history should use when interacting with it.
type SemaphoreBinding struct {
	// SemaphoreID is the BusinessID of the Semaphore execution to use. Empty
	// means this task is not gated by any semaphore.
	SemaphoreID string

	// HolderID is the deterministic id used for Reserve/Commit/Release on
	// this task.
	HolderID string
}

// BindingForTask returns the semaphore binding for a task. This is the
// placeholder integration point between activity tasks and the semaphore
// component. For now it always returns "no semaphore" so callers can
// short-circuit; later this will be driven by namespace/task-queue config.
//
// TODO: replace with real lookup against namespace config or task-queue
// metadata once the integration design is settled. The returned HolderID is
// the deterministic value we'd use even when SemaphoreID is empty, so callers
// that want to test the path can hard-code a SemaphoreID and reuse HolderID
// as-is.
func BindingForTask(t TaskKey) SemaphoreBinding {
	return SemaphoreBinding{
		SemaphoreID: "", // no semaphore until real config lookup is wired in
		HolderID:    HolderIDForTask(t),
	}
}

// HolderIDForTask computes the deterministic holder id for a task.
//
// It is a sha256 of the fields in TaskKey so that any retried path through
// matching (or history) produces the same id, which is what makes
// Reserve/Commit/Release idempotent across retries.
func HolderIDForTask(t TaskKey) string {
	h := sha256.New()
	// Length-prefix each field so encodings don't collide across fields.
	for _, f := range []string{
		t.NamespaceID,
		t.TaskQueue,
		int32Hex(t.TaskQueueKind),
		t.WorkflowID,
		t.RunID,
		int64Hex(t.ScheduledEventID),
	} {
		_, _ = h.Write([]byte{byte(len(f) >> 8), byte(len(f))})
		_, _ = h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func int32Hex(v int32) string {
	var b [4]byte
	for i := 0; i < 4; i++ {
		b[3-i] = byte(v >> (8 * i))
	}
	return hex.EncodeToString(b[:])
}

func int64Hex(v int64) string {
	var b [8]byte
	for i := 0; i < 8; i++ {
		b[7-i] = byte(v >> (8 * i))
	}
	return hex.EncodeToString(b[:])
}

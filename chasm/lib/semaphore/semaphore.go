package semaphore

import (
	"slices"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/chasm"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
)

var (
	ErrSemaphoreNotFound = serviceerror.NewNotFound("semaphore not found")
	ErrInvalidLimit      = serviceerror.NewInvalidArgument("limit must be >= 0")
	ErrEmptyHolderID     = serviceerror.NewInvalidArgument("holder id must not be empty")
)

// Semaphore is a CHASM root component representing a counting semaphore. It
// keeps a configurable limit, the current set of holders, and a FIFO queue of
// waiters that get promoted into holders as slots free up.
type Semaphore struct {
	chasm.UnimplementedComponent

	*semaphorepb.SemaphoreState
}

// newSemaphore is used internally by Create / UpdateWithStart flows.
func newSemaphore(limit int32) *Semaphore {
	return &Semaphore{
		SemaphoreState: &semaphorepb.SemaphoreState{
			Limit: limit,
		},
	}
}

// LifecycleState implements chasm.Component. Semaphores never reach a terminal
// state on their own; the caller deletes them explicitly.
func (s *Semaphore) LifecycleState(_ chasm.Context) chasm.LifecycleState {
	return chasm.LifecycleStateRunning
}

// ContextMetadata implements chasm.RootComponent.
func (s *Semaphore) ContextMetadata(_ chasm.Context) map[string]string {
	return nil
}

// Terminate implements chasm.RootComponent.
func (s *Semaphore) Terminate(
	_ chasm.MutableContext,
	_ chasm.TerminateComponentRequest,
) (chasm.TerminateComponentResponse, error) {
	return chasm.TerminateComponentResponse{}, nil
}

// CreateSemaphore is the StartExecution factory used by SetLimit when the
// semaphore does not yet exist.
func CreateSemaphore(
	_ chasm.MutableContext,
	req *semaphorepb.SetLimitRequest,
) (*Semaphore, error) {
	if req.Limit < 0 {
		return nil, ErrInvalidLimit
	}
	return newSemaphore(req.Limit), nil
}

// SetLimit updates the limit on an existing semaphore. Increasing the limit
// promotes queued waiters into holders, in FIFO order, until the limit is
// reached or the queue is empty.
func (s *Semaphore) SetLimit(
	_ chasm.MutableContext,
	req *semaphorepb.SetLimitRequest,
) (*semaphorepb.SetLimitResponse, error) {
	if req.Limit < 0 {
		return nil, ErrInvalidLimit
	}
	s.Limit = req.Limit
	s.promoteWaiters()
	return &semaphorepb.SetLimitResponse{Limit: s.Limit}, nil
}

// GetLimit returns the current limit.
func (s *Semaphore) GetLimit(
	_ chasm.Context,
	_ *semaphorepb.GetLimitRequest,
) (*semaphorepb.GetLimitResponse, error) {
	return &semaphorepb.GetLimitResponse{Limit: s.Limit}, nil
}

// GetHolders returns the current holders and waiters.
func (s *Semaphore) GetHolders(
	_ chasm.Context,
	_ *semaphorepb.GetHoldersRequest,
) (*semaphorepb.GetHoldersResponse, error) {
	return &semaphorepb.GetHoldersResponse{
		Holders: append([]string(nil), s.Holders...),
		Waiters: append([]string(nil), s.Waiters...),
	}, nil
}

// enrollResult signals to the caller whether the long-poll phase is needed.
type enrollResult struct {
	acquired bool
}

// Enroll is the mutation half of Acquire: add holder_id to the holders if
// possible, otherwise enqueue it as a waiter. Idempotent for ids already
// present in either list.
func (s *Semaphore) Enroll(
	_ chasm.MutableContext,
	req *semaphorepb.AcquireRequest,
) (enrollResult, error) {
	if req.HolderId == "" {
		return enrollResult{}, ErrEmptyHolderID
	}
	if slices.Contains(s.Holders, req.HolderId) {
		return enrollResult{acquired: true}, nil
	}
	if int32(len(s.Holders)) < s.Limit {
		s.Holders = append(s.Holders, req.HolderId)
		return enrollResult{acquired: true}, nil
	}
	if !slices.Contains(s.Waiters, req.HolderId) {
		s.Waiters = append(s.Waiters, req.HolderId)
	}
	return enrollResult{acquired: false}, nil
}

// Release removes holder_id from the holders, then promotes the next waiter
// into a holder if one is queued. If holder_id is only queued as a waiter, it
// is removed from the wait queue instead. No-op otherwise.
func (s *Semaphore) Release(
	_ chasm.MutableContext,
	req *semaphorepb.ReleaseRequest,
) (*semaphorepb.ReleaseResponse, error) {
	if req.HolderId == "" {
		return nil, ErrEmptyHolderID
	}
	if i := slices.Index(s.Holders, req.HolderId); i >= 0 {
		s.Holders = slices.Delete(s.Holders, i, i+1)
		s.promoteWaiters()
		return &semaphorepb.ReleaseResponse{}, nil
	}
	if i := slices.Index(s.Waiters, req.HolderId); i >= 0 {
		s.Waiters = slices.Delete(s.Waiters, i, i+1)
	}
	return &semaphorepb.ReleaseResponse{}, nil
}

// promoteWaiters moves waiters into holders, in FIFO order, while there is
// capacity. Called whenever the available capacity may have increased (a
// release happened, or the limit was raised).
func (s *Semaphore) promoteWaiters() {
	for int32(len(s.Holders)) < s.Limit && len(s.Waiters) > 0 {
		s.Holders = append(s.Holders, s.Waiters[0])
		s.Waiters = s.Waiters[1:]
	}
}

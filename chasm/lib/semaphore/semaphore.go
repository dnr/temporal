package semaphore

import (
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/chasm"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// reservationTTL is how long a Reserved slot lives before being auto-released
// if Commit is not called. Matching is expected to do Reserve ->
// RecordActivityTaskStarted -> Commit within this window.
//
// TODO: make this dynamic config.
const reservationTTL = 1 * time.Second

var (
	ErrSlotNotFound     = serviceerror.NewNotFound("semaphore slot not found")
	ErrInvalidLimit     = serviceerror.NewInvalidArgument("limit must be >= 0")
	ErrEmptyHolderID    = serviceerror.NewInvalidArgument("holder id must not be empty")
	ErrNoSlotsAvailable = serviceerror.NewResourceExhausted(0, "no slot available")
)

// Semaphore is a CHASM root component representing a counting semaphore with
// two-phase (Reserve + Commit) acquisition.
//
// The persisted state is just a limit and a map of currently-active slots
// (each either Reserved or Committed). There is intentionally no durable
// waiter queue: blocked Reserve callers are just open RPCs.
type Semaphore struct {
	chasm.UnimplementedComponent

	*semaphorepb.SemaphoreState
}

func newSemaphore(limit int32) *Semaphore {
	return &Semaphore{
		SemaphoreState: &semaphorepb.SemaphoreState{
			Limit: limit,
			Slots: map[string]*semaphorepb.SlotInfo{},
		},
	}
}

func (s *Semaphore) LifecycleState(_ chasm.Context) chasm.LifecycleState {
	return chasm.LifecycleStateRunning
}

func (s *Semaphore) ContextMetadata(_ chasm.Context) map[string]string {
	return nil
}

func (s *Semaphore) Terminate(
	_ chasm.MutableContext,
	_ chasm.TerminateComponentRequest,
) (chasm.TerminateComponentResponse, error) {
	return chasm.TerminateComponentResponse{}, nil
}

// CreateSemaphore is the StartExecution factory used by SetLimit.
func CreateSemaphore(
	_ chasm.MutableContext,
	req *semaphorepb.SetLimitRequest,
) (*Semaphore, error) {
	if req.Limit < 0 {
		return nil, ErrInvalidLimit
	}
	return newSemaphore(req.Limit), nil
}

// SetLimit updates the limit on an existing semaphore. The framework does
// not evict existing holders when the limit is reduced; new Reserves will
// just block until the active count falls below the new limit.
func (s *Semaphore) SetLimit(
	_ chasm.MutableContext,
	req *semaphorepb.SetLimitRequest,
) (*semaphorepb.SetLimitResponse, error) {
	if req.Limit < 0 {
		return nil, ErrInvalidLimit
	}
	s.Limit = req.Limit
	return &semaphorepb.SetLimitResponse{Limit: s.Limit}, nil
}

func (s *Semaphore) GetLimit(
	_ chasm.Context,
	_ *semaphorepb.GetLimitRequest,
) (*semaphorepb.GetLimitResponse, error) {
	return &semaphorepb.GetLimitResponse{Limit: s.Limit}, nil
}

func (s *Semaphore) GetHolders(
	_ chasm.Context,
	_ *semaphorepb.GetHoldersRequest,
) (*semaphorepb.GetHoldersResponse, error) {
	resp := &semaphorepb.GetHoldersResponse{}
	for id, slot := range s.Slots {
		if slot.GetReservationExpiresAt() == nil {
			resp.Committed = append(resp.Committed, id)
		} else {
			resp.Reserved = append(resp.Reserved, id)
		}
	}
	return resp, nil
}

// reserveOutcome is the result of a single Reserve attempt. The handler uses
// it to decide whether to return immediately or long-poll for state changes.
type reserveOutcome int

const (
	reserveOutcomeReserved reserveOutcome = iota
	reserveOutcomeAlreadyCommitted
	reserveOutcomeNoRoom
)

type reserveResult struct {
	outcome   reserveOutcome
	expiresAt time.Time
}

// Reserve attempts to add holder_id as a Reserved slot. If the holder is
// already present, the call is idempotent: a Reserved slot has its
// expiration refreshed, a Committed slot returns alreadyCommitted.
func (s *Semaphore) Reserve(
	ctx chasm.MutableContext,
	req *semaphorepb.ReserveRequest,
) (reserveResult, error) {
	if req.HolderId == "" {
		return reserveResult{}, ErrEmptyHolderID
	}

	if existing, ok := s.Slots[req.HolderId]; ok {
		if existing.ReservationExpiresAt == nil {
			return reserveResult{outcome: reserveOutcomeAlreadyCommitted}, nil
		}
		// Refresh existing reservation.
		expiresAt := ctx.Now(s).Add(reservationTTL)
		existing.ReservationExpiresAt = timestamppb.New(expiresAt)
		s.scheduleExpiryTask(ctx, req.HolderId, expiresAt)
		return reserveResult{outcome: reserveOutcomeReserved, expiresAt: expiresAt}, nil
	}

	if int32(len(s.Slots)) >= s.Limit {
		return reserveResult{outcome: reserveOutcomeNoRoom}, nil
	}

	expiresAt := ctx.Now(s).Add(reservationTTL)
	s.Slots[req.HolderId] = &semaphorepb.SlotInfo{
		ReservationExpiresAt: timestamppb.New(expiresAt),
	}
	s.scheduleExpiryTask(ctx, req.HolderId, expiresAt)
	return reserveResult{outcome: reserveOutcomeReserved, expiresAt: expiresAt}, nil
}

// Commit promotes a Reserved slot to Committed. Idempotent if already
// Committed. Returns NotFound if the slot is gone (expired).
func (s *Semaphore) Commit(
	_ chasm.MutableContext,
	req *semaphorepb.CommitRequest,
) (*semaphorepb.CommitResponse, error) {
	if req.HolderId == "" {
		return nil, ErrEmptyHolderID
	}
	slot, ok := s.Slots[req.HolderId]
	if !ok {
		return nil, ErrSlotNotFound
	}
	slot.ReservationExpiresAt = nil
	return &semaphorepb.CommitResponse{}, nil
}

// Unreserve explicitly removes a Reserved slot before its TTL. No-op for
// Committed or absent slots.
func (s *Semaphore) Unreserve(
	_ chasm.MutableContext,
	req *semaphorepb.UnreserveRequest,
) (*semaphorepb.UnreserveResponse, error) {
	if req.HolderId == "" {
		return nil, ErrEmptyHolderID
	}
	if slot, ok := s.Slots[req.HolderId]; ok && slot.ReservationExpiresAt != nil {
		delete(s.Slots, req.HolderId)
	}
	return &semaphorepb.UnreserveResponse{}, nil
}

// Release removes a slot regardless of state. No-op if absent.
func (s *Semaphore) Release(
	_ chasm.MutableContext,
	req *semaphorepb.ReleaseRequest,
) (*semaphorepb.ReleaseResponse, error) {
	if req.HolderId == "" {
		return nil, ErrEmptyHolderID
	}
	delete(s.Slots, req.HolderId)
	// TODO: push req.HolderId onto a bounded "recently released" cache so
	// ABA-style stale Reserves can be detected and rejected.
	return &semaphorepb.ReleaseResponse{}, nil
}

// hasRoom is the predicate used by Reserve's long-poll: returns true if a
// new Reserve could plausibly succeed. The check is best-effort — another
// caller may have grabbed the slot by the time the handler retries.
func (s *Semaphore) hasRoom() bool {
	return int32(len(s.Slots)) < s.Limit
}

func (s *Semaphore) scheduleExpiryTask(
	ctx chasm.MutableContext,
	holderID string,
	expiresAt time.Time,
) {
	ctx.AddTask(s,
		chasm.TaskAttributes{ScheduledTime: expiresAt},
		&semaphorepb.ReservationExpiryTask{
			HolderId:          holderID,
			ExpectedExpiresAt: timestamppb.New(expiresAt),
		},
	)
}

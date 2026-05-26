package semaphore

import (
	"go.temporal.io/server/chasm"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
)

// reservationExpiryTaskHandler runs reservationTTL after a Reserve. It
// removes the slot iff it's still Reserved with the same expiration — i.e.,
// the caller never called Commit and never refreshed the reservation.
type reservationExpiryTaskHandler struct {
	chasm.PureTaskHandlerBase
}

func newReservationExpiryTaskHandler() *reservationExpiryTaskHandler {
	return &reservationExpiryTaskHandler{}
}

// Validate is called before Execute; it drops the task if the slot has been
// Committed, removed, or superseded by a newer reservation.
func (h *reservationExpiryTaskHandler) Validate(
	_ chasm.Context,
	s *Semaphore,
	_ chasm.TaskAttributes,
	task *semaphorepb.ReservationExpiryTask,
) (bool, error) {
	slot, ok := s.Slots[task.HolderId]
	if !ok {
		// Already released.
		return false, nil
	}
	if slot.ReservationExpiresAt == nil {
		// Already committed.
		return false, nil
	}
	if !slot.ReservationExpiresAt.AsTime().Equal(task.ExpectedExpiresAt.AsTime()) {
		// Reservation was refreshed; a newer task supersedes this one.
		return false, nil
	}
	return true, nil
}

func (h *reservationExpiryTaskHandler) Execute(
	_ chasm.MutableContext,
	s *Semaphore,
	_ chasm.TaskAttributes,
	task *semaphorepb.ReservationExpiryTask,
) error {
	// Validate already confirmed the slot is the expected reservation.
	delete(s.Slots, task.HolderId)
	return nil
}

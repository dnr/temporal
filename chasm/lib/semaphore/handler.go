package semaphore

import (
	"context"
	"errors"
	"time"

	"go.temporal.io/server/chasm"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// wakeupSkew is added to the per-RPC wake-up timer so the retry happens
// strictly after the soonest reservation expiry — otherwise we can race
// the wall clock and the sweep finds nothing to evict.
const wakeupSkew = 5 * time.Millisecond

type handler struct {
	semaphorepb.UnimplementedSemaphoreServiceServer

	logger log.Logger
}

func newHandler(logger log.Logger) *handler {
	return &handler{logger: logger}
}

func (h *handler) SetLimit(
	ctx context.Context,
	req *semaphorepb.SetLimitRequest,
) (resp *semaphorepb.SetLimitResponse, err error) {
	defer log.CapturePanic(h.logger, &err)

	result, err := chasm.UpdateWithStartExecution(
		ctx,
		chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.SemaphoreId,
		},
		CreateSemaphore,
		(*Semaphore).SetLimit,
		req,
	)
	if err != nil {
		return nil, err
	}
	return result.UpdateOutput, nil
}

func (h *handler) GetLimit(
	ctx context.Context,
	req *semaphorepb.GetLimitRequest,
) (resp *semaphorepb.GetLimitResponse, err error) {
	defer log.CapturePanic(h.logger, &err)

	return chasm.ReadComponent(
		ctx,
		chasm.NewComponentRef[*Semaphore](chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.SemaphoreId,
		}),
		(*Semaphore).GetLimit,
		req,
	)
}

func (h *handler) GetHolders(
	ctx context.Context,
	req *semaphorepb.GetHoldersRequest,
) (resp *semaphorepb.GetHoldersResponse, err error) {
	defer log.CapturePanic(h.logger, &err)

	return chasm.ReadComponent(
		ctx,
		chasm.NewComponentRef[*Semaphore](chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.SemaphoreId,
		}),
		(*Semaphore).GetHolders,
		req,
	)
}

// Reserve loops UpdateComponent + PollComponent until a slot is acquired or
// the caller's deadline expires. The Reserve mutation does the on-demand
// expiry sweep, so each iteration sees fresh state.
//
// Wake-up sources for the long-poll:
//   - Any other Reserve/Commit/Unreserve/Release transition (Notify wakes
//     all blocked Polls on the execution).
//   - A per-RPC timer set to the soonest reservation_expires_at + skew, so
//     this caller wakes up as soon as it could plausibly sweep a slot.
//
// FIFO across competing Reserve callers is NOT guaranteed.
func (h *handler) Reserve(
	ctx context.Context,
	req *semaphorepb.ReserveRequest,
) (resp *semaphorepb.ReserveResponse, err error) {
	defer log.CapturePanic(h.logger, &err)

	ref := chasm.NewComponentRef[*Semaphore](chasm.ExecutionKey{
		NamespaceID: req.NamespaceId,
		BusinessID:  req.SemaphoreId,
	})

	for {
		if err := ctx.Err(); err != nil {
			return &semaphorepb.ReserveResponse{Reserved: false}, nil
		}

		result, _, err := chasm.UpdateComponent(ctx, ref, (*Semaphore).Reserve, req)
		if err != nil {
			return nil, err
		}

		switch result.outcome {
		case reserveOutcomeReserved:
			return &semaphorepb.ReserveResponse{
				Reserved:             true,
				ReservationExpiresAt: timestamppb.New(result.expiresAt),
			}, nil
		case reserveOutcomeAlreadyCommitted:
			return &semaphorepb.ReserveResponse{AlreadyCommitted: true}, nil
		case reserveOutcomeNoRoom:
			// Wait for either (a) a transition to wake the poll, or (b) the
			// per-RPC timer to fire when the soonest reservation expires.
			waitCtx, cancel := pollWaitContext(ctx, result.soonestExpiry)
			_, _, err := chasm.PollComponent(
				waitCtx,
				ref,
				func(s *Semaphore, _ chasm.Context, _ *semaphorepb.ReserveRequest) (chasm.NoValue, bool, error) {
					// Any state change wakes us; we re-attempt via the outer
					// UpdateComponent. A trivial predicate is fine.
					return nil, false, nil
				},
				req,
			)
			cancel()
			// We treat both "predicate satisfied" (won't happen with this
			// predicate) and "waitCtx done" (timer or change) the same: loop.
			// Only bubble up if it's the caller's outer ctx that expired.
			if err != nil && ctx.Err() != nil {
				return &semaphorepb.ReserveResponse{Reserved: false}, nil
			}
		}
	}
}

// pollWaitContext derives a child context that fires at the earlier of the
// caller's deadline and the soonest reservation expiry. When soonestExpiry
// is zero (no Reserved slots), the child just inherits the caller's
// deadline.
func pollWaitContext(parent context.Context, soonestExpiry time.Time) (context.Context, context.CancelFunc) {
	if soonestExpiry.IsZero() {
		return context.WithCancel(parent)
	}
	return context.WithDeadline(parent, soonestExpiry.Add(wakeupSkew))
}

func (h *handler) Commit(
	ctx context.Context,
	req *semaphorepb.CommitRequest,
) (resp *semaphorepb.CommitResponse, err error) {
	defer log.CapturePanic(h.logger, &err)

	resp, _, err = chasm.UpdateComponent(
		ctx,
		chasm.NewComponentRef[*Semaphore](chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.SemaphoreId,
		}),
		(*Semaphore).Commit,
		req,
	)
	if errors.Is(err, ErrSlotNotFound) {
		return nil, ErrSlotNotFound
	}
	return resp, err
}

func (h *handler) Unreserve(
	ctx context.Context,
	req *semaphorepb.UnreserveRequest,
) (resp *semaphorepb.UnreserveResponse, err error) {
	defer log.CapturePanic(h.logger, &err)

	resp, _, err = chasm.UpdateComponent(
		ctx,
		chasm.NewComponentRef[*Semaphore](chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.SemaphoreId,
		}),
		(*Semaphore).Unreserve,
		req,
	)
	return resp, err
}

// Release is durable: UpdateComponent does not return until the transition
// is persisted. Callers (history's RespondActivityTask*/timer handlers) can
// rely on a successful Release meaning the slot is released.
func (h *handler) Release(
	ctx context.Context,
	req *semaphorepb.ReleaseRequest,
) (resp *semaphorepb.ReleaseResponse, err error) {
	defer log.CapturePanic(h.logger, &err)

	resp, _, err = chasm.UpdateComponent(
		ctx,
		chasm.NewComponentRef[*Semaphore](chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.SemaphoreId,
		}),
		(*Semaphore).Release,
		req,
	)
	return resp, err
}

package semaphore

import (
	"context"
	"errors"

	"go.temporal.io/server/chasm"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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

// Reserve is the long-poll RPC: it tries to attach holder_id to a Reserved
// slot, blocking via PollComponent until a slot opens or the caller's
// deadline expires.
//
// Loop sketch (no durable waiter queue):
//
//  1. UpdateComponent attempts Reserve. On success (Reserved or
//     AlreadyCommitted) we return immediately.
//  2. On NoRoom, PollComponent waits for any state change. The predicate is
//     intentionally lax ("at least one slot is free now") because the
//     real arbitration happens at the UpdateComponent retry in step 1.
//  3. Loop until deadline.
//
// FIFO across competing Reserve callers is NOT guaranteed in this design —
// "fifo for now" is a future optimization.
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
			// Wait for some state change, then retry the update.
			_, _, err := chasm.PollComponent(
				ctx,
				ref,
				func(s *Semaphore, _ chasm.Context, _ *semaphorepb.ReserveRequest) (chasm.NoValue, bool, error) {
					return nil, s.hasRoom(), nil
				},
				req,
			)
			if err != nil {
				if ctx.Err() != nil {
					return &semaphorepb.ReserveResponse{Reserved: false}, nil
				}
				return nil, err
			}
		}
	}
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

// Release is durable: UpdateComponent does not return until the transition is
// persisted. Callers (history's RespondActivityTask*/timer handlers) can
// rely on a successful Release to mean the slot is released.
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

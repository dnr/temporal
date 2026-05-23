package semaphore

import (
	"context"
	"slices"

	"go.temporal.io/server/chasm"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
	"go.temporal.io/server/common/log"
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

// Acquire is the long-poll RPC. It runs in two phases: first an Enroll update
// adds the holder_id to the holders (if a slot is free) or to the waiters; if
// not immediately acquired, a PollComponent waits until the holder_id appears
// in the holders list, or the caller's deadline expires.
//
// NOTE: this skeleton does not currently clean up the waiter entry on deadline
// expiry — a subsequent Acquire with the same id resumes polling, but a caller
// that gives up entirely will leak a waiter slot. Add explicit cancellation
// later.
func (h *handler) Acquire(
	ctx context.Context,
	req *semaphorepb.AcquireRequest,
) (resp *semaphorepb.AcquireResponse, err error) {
	defer log.CapturePanic(h.logger, &err)

	ref := chasm.NewComponentRef[*Semaphore](chasm.ExecutionKey{
		NamespaceID: req.NamespaceId,
		BusinessID:  req.SemaphoreId,
	})

	enroll, _, err := chasm.UpdateComponent(
		ctx,
		ref,
		(*Semaphore).Enroll,
		req,
	)
	if err != nil {
		return nil, err
	}
	if enroll.acquired {
		return &semaphorepb.AcquireResponse{Acquired: true}, nil
	}

	// Long-poll until the id appears in holders. The predicate is monotonic
	// up to the next Release for this id — for the purposes of returning
	// from the poll, that's sufficient: once observed as a holder, we're
	// done.
	_, _, err = chasm.PollComponent(
		ctx,
		ref,
		func(s *Semaphore, _ chasm.Context, req *semaphorepb.AcquireRequest) (chasm.NoValue, bool, error) {
			return nil, slices.Contains(s.Holders, req.HolderId), nil
		},
		req,
	)
	if err != nil {
		if ctx.Err() != nil {
			// Deadline reached without acquiring.
			return &semaphorepb.AcquireResponse{Acquired: false}, nil
		}
		return nil, err
	}
	return &semaphorepb.AcquireResponse{Acquired: true}, nil
}

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

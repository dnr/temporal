package flowcontrol

import (
	"context"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/server/chasm"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/log"
)

type concurrencyHandler struct {
	fcpb.UnimplementedConcurrencyServiceServer

	logger log.Logger
}

func newConcurrencyHandler(
	logger log.Logger,
) *concurrencyHandler {
	return &concurrencyHandler{
		logger: logger,
	}
}

func (h *concurrencyHandler) Reserve(ctx context.Context, req *fcpb.ConcurrencyReserveRequest) (retRes *fcpb.ConcurrencyReserveResponse, retErr error) {
	defer log.CapturePanic(h.logger, &retErr)

	res, err := chasm.UpdateWithStartExecution(
		ctx,
		chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.Key,
		},
		func(_ chasm.MutableContext, req *fcpb.ConcurrencyReserveRequest) (*concurrency, error) {
			// For now (whole-queue limits), we should always have an initial config here. For
			// per-task limits, this may be missing and set via api later.
			config := req.GetConfigUpdate()
			if config == nil {
				config = &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: initialConcurrencyLimit}
			}
			return &concurrency{
				ConcurrencyState: &fcpb.ConcurrencyState{
					Config:        config,
					ConfigVersion: req.GetConfigUpdateVersion(),
				},
			}, nil
		},
		func(c *concurrency, cctx chasm.MutableContext, req *fcpb.ConcurrencyReserveRequest) (*fcpb.ConcurrencyReserveResponse, error) {
			c.updateConfig(req.GetConfigUpdate(), req.GetConfigUpdateVersion())
			slotsReserved := c.reserve(req.TaskUuid, cctx.Now(c))
			return &fcpb.ConcurrencyReserveResponse{
				Generation:    c.Generation,
				SlotsReserved: slotsReserved,
			}, nil
		},
		req,
		chasm.WithBusinessIDPolicy(
			chasm.BusinessIDReusePolicyAllowDuplicate,
			chasm.BusinessIDConflictPolicyUseExisting,
		),
		chasm.WithSpeculative(), // note: not implemented yet
	)
	return res.UpdateOutput, err
}

func (h *concurrencyHandler) CancelReservation(ctx context.Context, req *fcpb.ConcurrencyCancelReservationRequest) (retRes *fcpb.ConcurrencyCancelReservationResponse, retErr error) {
	defer log.CapturePanic(h.logger, &retErr)

	res, _, err := chasm.UpdateComponent(
		ctx,
		chasm.NewComponentRef[*concurrency](chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.Key,
		}),
		func(c *concurrency, cctx chasm.MutableContext, req *fcpb.ConcurrencyCancelReservationRequest) (*fcpb.ConcurrencyCancelReservationResponse, error) {
			c.cancelReservation(req.TaskUuid, cctx.Now(c))
			return &fcpb.ConcurrencyCancelReservationResponse{}, nil
		},
		req,
		chasm.WithSpeculative(), // note: not implemented yet
	)
	return res, err
}

func (h *concurrencyHandler) Commit(ctx context.Context, req *fcpb.ConcurrencyCommitRequest) (retRes *fcpb.ConcurrencyCommitResponse, retErr error) {
	defer log.CapturePanic(h.logger, &retErr)

	res, _, err := chasm.UpdateComponent(
		ctx,
		chasm.NewComponentRef[*concurrency](chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.Key,
		}),
		func(c *concurrency, cctx chasm.MutableContext, req *fcpb.ConcurrencyCommitRequest) (*fcpb.ConcurrencyCommitResponse, error) {
			err := c.commit(req.TaskUuid, cctx.Now(c))
			if err != nil {
				return nil, err
			}
			return &fcpb.ConcurrencyCommitResponse{}, nil
		},
		req,
	)
	return res, err
}

// FIXME: have to get history to call Release
func (h *concurrencyHandler) Release(ctx context.Context, req *fcpb.ConcurrencyReleaseRequest) (retRes *fcpb.ConcurrencyReleaseResponse, retErr error) {
	defer log.CapturePanic(h.logger, &retErr)

	res, _, err := chasm.UpdateComponent(
		ctx,
		chasm.NewComponentRef[*concurrency](chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.Key,
		}),
		func(c *concurrency, cctx chasm.MutableContext, req *fcpb.ConcurrencyReleaseRequest) (*fcpb.ConcurrencyReleaseResponse, error) {
			c.release(req.TaskUuid, cctx.Now(c))
			// FIXME: add tasks to control slow release of notifications
			return &fcpb.ConcurrencyReleaseResponse{}, nil
		},
		req,
	)
	return res, err
}

func (h *concurrencyHandler) Wait(ctx context.Context, req *fcpb.ConcurrencyWaitRequest) (retRes *fcpb.ConcurrencyWaitResponse, retErr error) {
	defer log.CapturePanic(h.logger, &retErr)

	// CHASM requires that PollComponent be monotonic: if it returns true at some state, then
	// it should return true at future states. Our predicate is essentially "is there any free
	// slot?" This can switch from false to true on a Release or CancelReservation or
	// reservation expiry, which is monotonic. The problem is the other way, on Reserve, it
	// could switch from true to false, which isn't allowed. To fix this, we add a
	// "generation": whenever there was a free slot but isn't anymore, we increment a
	// generation counter. We return it on every Reserve and Wait, so callers should have the
	// current value. If their value is out of date then we return immediately (true), if it's
	// too new than we wait for it to catch up (false).
	res, _, err := chasm.PollComponent(
		ctx,
		chasm.NewComponentRef[*concurrency](chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.Key,
		}),
		func(c *concurrency, cctx chasm.Context, req *fcpb.ConcurrencyWaitRequest) (*fcpb.ConcurrencyWaitResponse, bool, error) {
			notify := c.notifyWaiters(cctx.Now(c))

			if req.Generation < c.Generation {
				// TODO(fc): don't always notify all free slots on stale generation?
				return &fcpb.ConcurrencyWaitResponse{Generation: c.Generation, WakeCount: notify}, true, nil
			} else if req.Generation > c.Generation {
				// Generation is too new. We have to return false until we get there.
				return nil, false, nil
			}
			if notify == 0 {
				// No free slots, wait for another transition.
				return nil, false, nil
			}
			// Notify caller with some slots.
			return &fcpb.ConcurrencyWaitResponse{Generation: c.Generation, WakeCount: notify}, true, nil
		},
		req,
	)
	if err == nil && res == nil {
		return &fcpb.ConcurrencyWaitResponse{}, nil
	}
	// TODO(fc): we can try again if we still have time and we got a stale generation?
	return res, err
}

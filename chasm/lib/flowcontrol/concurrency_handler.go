package flowcontrol

import (
	"context"

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
			return &concurrency{
				ConcurrencyState: &fcpb.ConcurrencyState{
					Limit: req.GetLimitUpdate().GetLimit(),
				},
			}, nil
		},
		func(c *concurrency, cctx chasm.MutableContext, req *fcpb.ConcurrencyReserveRequest) (*fcpb.ConcurrencyReserveResponse, error) {
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
		chasm.WithSpeculative(), // TODO: does this work yet?
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
		chasm.WithSpeculative(), // TODO: does this work yet?
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

	// FIXME: confirm if we can mutate c within PollComponent even if we don't want it persisted
	res, _, err := chasm.PollComponent(
		ctx,
		chasm.NewComponentRef[*concurrency](chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.Key,
		}),
		func(c *concurrency, cctx chasm.Context, req *fcpb.ConcurrencyWaitRequest) (*fcpb.ConcurrencyWaitResponse, bool, error) {
			slots := c.slotsFree(cctx.Now(c))
			if req.Generation < c.Generation {
				return &fcpb.ConcurrencyWaitResponse{Generation: c.Generation, WakeCount: slots}, true, nil
			}
			if slots == 0 {
				return nil, false, nil
			}
			return &fcpb.ConcurrencyWaitResponse{WakeCount: slots}, true, nil
		},
		req,
	)
	if err == nil && res == nil {
		return &fcpb.ConcurrencyWaitResponse{}, nil
	}
	return res, err
}

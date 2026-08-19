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
		func(c *concurrency, _ chasm.MutableContext, req *fcpb.ConcurrencyReserveRequest) (*fcpb.ConcurrencyReserveResponse, error) {
			err := c.reserve(req.TaskUuid)
			if err != nil {
				return nil, err
			}
			return &fcpb.ConcurrencyReserveResponse{}, nil
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
		func(c *concurrency, _ chasm.MutableContext, req *fcpb.ConcurrencyCancelReservationRequest) (*fcpb.ConcurrencyCancelReservationResponse, error) {
			err := c.cancelReservation(req.TaskUuid)
			if err != nil {
				return nil, err
			}
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
		func(c *concurrency, _ chasm.MutableContext, req *fcpb.ConcurrencyCommitRequest) (*fcpb.ConcurrencyCommitResponse, error) {
			err := c.commit(req.TaskUuid)
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
		func(c *concurrency, _ chasm.MutableContext, req *fcpb.ConcurrencyReleaseRequest) (*fcpb.ConcurrencyReleaseResponse, error) {
			err := c.release(req.TaskUuid)
			if err != nil {
				return nil, err
			}
			return &fcpb.ConcurrencyReleaseResponse{}, nil
		},
		req,
	)
	return res, err
}

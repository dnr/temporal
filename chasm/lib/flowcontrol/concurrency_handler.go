package flowcontrol

import (
	"context"

	"go.temporal.io/server/chasm"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
					Limit: 10, // FIXME: initial limit???
				},
			}, nil
		},
		func(c *concurrency, _ chasm.MutableContext, req *fcpb.ConcurrencyReserveRequest) (*fcpb.ConcurrencyReserveResponse, error) {
			err := c.reserve(req.TaskUuid())
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
		// TODO(fc): chasm.WithRequestID(???),
		chasm.WithSpeculative(), // TODO: does this work yet?
	)
	return res.UpdateOutput, err
}

func (h *concurrencyHandler) CancelReservation(ctx context.Context, req *fcpb.ConcurrencyCancelReservationRequest) (*fcpb.ConcurrencyCancelReservationResponse, error) {
	// defer log.CapturePanic(h.logger, &err)
	return nil, status.Errorf(codes.Unimplemented, "method CancelReservation not implemented")
}

func (h *concurrencyHandler) Commit(ctx context.Context, req *fcpb.ConcurrencyCommitRequest) (*fcpb.ConcurrencyCommitResponse, error) {
	// defer log.CapturePanic(h.logger, &err)
	return nil, status.Errorf(codes.Unimplemented, "method Commit not implemented")
}

func (h *concurrencyHandler) Release(ctx context.Context, req *fcpb.ConcurrencyReleaseRequest) (*fcpb.ConcurrencyReleaseResponse, error) {
	// defer log.CapturePanic(h.logger, &err)
	return nil, status.Errorf(codes.Unimplemented, "method Release not implemented")
}

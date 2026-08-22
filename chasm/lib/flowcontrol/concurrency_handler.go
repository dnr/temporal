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

func concurrencyInit(_ chasm.MutableContext, req *fcpb.ConcurrencyBatchRequest) (*concurrency, error) {
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
}

func concurrencyUpdate(c *concurrency, cctx chasm.MutableContext, req *fcpb.ConcurrencyBatchRequest) (*fcpb.ConcurrencyBatchResponse, error) {
	now := cctx.Now(c)

	// expire old reservations
	c.expire(now)

	// update config
	c.updateConfig(req.GetConfigUpdate(), req.GetConfigUpdateVersion())

	var res fcpb.ConcurrencyBatchResponse

	// apply releases (shouldn't be combined with anything but releases)
	for _, slotId := range req.GetReleaseSlots() {
		// FIXME: add tasks to control slow release of notifications
		c.release(slotId)
	}

	// apply commits
	for _, slotId := range req.GetCommitSlots() {
		success := c.commit(slotId)
		res.CommitSuccess = append(res.CommitSuccess, success)
	}

	// apply cancels
	for _, slotId := range req.GetCancelReservationSlots() {
		c.cancelReservation(slotId)
	}

	// apply reserves
	for _, slotId := range req.GetReserveSlots() {
		success := c.reserve(slotId, now)
		res.ReserveSuccess = append(res.ReserveSuccess, success)
	}

	// capture Generation after c.reserve, which may increment it
	res.Generation = c.Generation

	return &res, nil
}

func (h *concurrencyHandler) Batch(ctx context.Context, req *fcpb.ConcurrencyBatchRequest) (retRes *fcpb.ConcurrencyBatchResponse, retErr error) {
	defer log.CapturePanic(h.logger, &retErr)

	// FIXME: add server-side batching

	// only Reserve should be called on a brand new object, other calls can assume it exists
	needStart := len(req.GetReserveSlots()) > 0

	// Commit and Release must be durable, Reserve and cancel may be speculative
	canSpeculate := len(req.GetCommitSlots()) == 0 && len(req.GetReleaseSlots()) == 0

	opts := make([]chasm.TransitionOption, 0, 2)
	if canSpeculate {
		opts = append(opts, chasm.WithSpeculative()) // note: not implemented yet
	}

	if needStart {
		opts = append(opts, chasm.WithBusinessIDPolicy(
			chasm.BusinessIDReusePolicyAllowDuplicate,
			chasm.BusinessIDConflictPolicyUseExisting,
		))
		res, err := chasm.UpdateWithStartExecution(
			ctx,
			chasm.ExecutionKey{
				NamespaceID: req.NamespaceId,
				BusinessID:  req.Key,
			},
			concurrencyInit,
			concurrencyUpdate,
			req,
			opts...,
		)
		return res.UpdateOutput, err
	}

	res, _, err := chasm.UpdateComponent(
		ctx,
		chasm.NewComponentRef[*concurrency](chasm.ExecutionKey{
			NamespaceID: req.NamespaceId,
			BusinessID:  req.Key,
		}),
		concurrencyUpdate,
		req,
		opts...,
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

package fc

import (
	"context"
	"time"

	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	serviceerrors "go.temporal.io/server/common/serviceerror"
)

var ErrConcurrencyBlocked = serviceerror.NewFailedPrecondition("blocked by concurrency limit")

type concurrencyTx struct {
	cs *concurrencyState
	// nsID   namespace.ID
	slotID string
	lim    Limiter
	pri    int32
	age    time.Time
}

func newConcurrencyTx(
	cs *concurrencyState,
	// nsID namespace.ID,
	slotID string,
	lim Limiter,
	pri int32,
	age time.Time,
) *concurrencyTx {
	return &concurrencyTx{
		// r:  r,
		cs: cs,
		// nsID:   nsID,
		slotID: slotID,
		lim:    lim,
		pri:    pri,
		age:    age,
	}
}

func (ct *concurrencyTx) check(cb ReadinessCallback) error {
	return ct.cs.check(cb)
}

func (ct *concurrencyTx) cancelCheck(cb ReadinessCallback) {
	ct.cs.cancelCheck(cb)
}

func (ct *concurrencyTx) reserve(ctx context.Context) error {
	// if config is missing or wrong type, just leave it out
	configUpdate, _ := ct.lim.Config.(*taskqueuepb.ConcurrencyLimit)

	res, err := ct.cs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId:         ct.nsID.String(),
		Key:                 ct.lim.Key,
		ReserveSlots:        []string{ct.slotID},
		ConfigUpdate:        configUpdate,
		ConfigUpdateVersion: ct.lim.ConfigVersion,
	})
	if err != nil {
		return err // don't update cache on rpc error
	}
	if !res.ReserveSuccess[0] {
		ct.cs.reportReserveFailed(res.Generation)
		return serviceerrors.NewFlowControlBlocked()
	}
	// TODO(fc): we could include a hint for how many slots are _remaining_, and if zero, mark
	// this limiter as blocked in the cache. but we don't want to immediately Wait on it since
	// we might not have another waiter yet.
	ct.r.reportReserveHint(ct.nsID, ct.lim.Key, res.Generation, 0) // FIXME: 0?
	return nil
}

func (ct *concurrencyTx) commit(ctx context.Context) error {
	res, err := ct.cs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId: ct.nsID.String(),
		Key:         ct.lim.Key,
		CommitSlots: []string{ct.slotID},
	})
	if err != nil {
		return err
	} else if !res.CommitSuccess[0] {
		return errCommitFailure
	}
	return err
}

func (ct *concurrencyTx) cancelReserve(ctx context.Context) {
	// call in new goroutine, don't block here, we don't care about the result
	go ct.cs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId:            ct.nsID.String(),
		Key:                    ct.lim.Key,
		CancelReservationSlots: []string{ct.slotID},
	})
}

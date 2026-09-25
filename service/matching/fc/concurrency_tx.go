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
	return ct.cs.check(cb, ct.pri, ct.age)
}

func (ct *concurrencyTx) cancelCheck() {
	ct.cs.cancelCheck()
}

func (ct *concurrencyTx) reserve(ctx context.Context) error {
	// if config is missing or wrong type, just leave it out
	configUpdate, _ := ct.lim.Config.(*taskqueuepb.ConcurrencyLimit)

	res, err := ct.cs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId:         ct.cs.nsID.String(),
		Key:                 ct.cs.key,
		ReserveSlots:        []string{ct.slotID},
		ConfigUpdate:        configUpdate,
		ConfigUpdateVersion: ct.lim.ConfigVersion,
	})
	if err != nil {
		return err // don't update cache on rpc error
	}
	ct.cs.reportSlotsHint(res.Generation, res.AvailableSlotsHint)
	if !res.ReserveSuccess[0] {
		return serviceerrors.NewFlowControlBlocked()
	}
	return nil
}

func (ct *concurrencyTx) commit(ctx context.Context) error {
	res, err := ct.cs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId: ct.cs.nsID.String(),
		Key:         ct.cs.key,
		CommitSlots: []string{ct.slotID},
	})
	if err != nil {
		return err
	}
	ct.cs.reportSlotsHint(res.Generation, res.AvailableSlotsHint)
	if !res.CommitSuccess[0] {
		return errCommitFailure
	}
	return nil
}

func (ct *concurrencyTx) cancelReserve(ctx context.Context) {
	// call in new goroutine, don't block here
	go func() {
		res, err := ct.cs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
			NamespaceId:            ct.cs.nsID.String(),
			Key:                    ct.cs.key,
			CancelReservationSlots: []string{ct.slotID},
		})
		if err == nil {
			ct.cs.reportSlotsHint(res.Generation, res.AvailableSlotsHint)
		}
	}()
}

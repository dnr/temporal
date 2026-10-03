package fc

import (
	"context"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
)

type concurrencyTx struct {
	cs                  *concurrencyState
	slotID              string
	configUpdate        *taskqueuepb.ConcurrencyLimit
	configUpdateVersion int64
	pri                 wakePriority
	checkEpoch          int64
}

func newConcurrencyTx(
	cs *concurrencyState,
	slotID string,
	configUpdate *taskqueuepb.ConcurrencyLimit,
	configUpdateVersion int64,
	pri wakePriority,
) *concurrencyTx {
	return &concurrencyTx{
		cs:                  cs,
		slotID:              slotID,
		configUpdate:        configUpdate,
		configUpdateVersion: configUpdateVersion,
		pri:                 pri,
	}
}

func (ct *concurrencyTx) check(cb ReadinessCallback) error {
	var err error
	ct.checkEpoch, err = ct.cs.check(cb, ct.pri)
	return err
}

func (ct *concurrencyTx) cancelCheck() {
	ct.cs.cancelCheck(ct.checkEpoch)
}

func (ct *concurrencyTx) reserve(ctx context.Context) error {
	return ct.cs.reserve(ctx, ct.slotID, ct.configUpdate, ct.configUpdateVersion)
}

func (ct *concurrencyTx) commit(ctx context.Context) error {
	return ct.cs.commit(ctx, ct.slotID)
}

func (ct *concurrencyTx) cancelReserve(ctx context.Context) {
	ct.cs.cancelReserve(ctx, ct.slotID)
}

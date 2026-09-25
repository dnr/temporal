package fc

import (
	"context"
	"time"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
)

type concurrencyTx struct {
	cs                  *concurrencyState
	slotID              string
	configUpdate        *taskqueuepb.ConcurrencyLimit
	configUpdateVersion int64
	pri                 int32
	age                 time.Time
}

func newConcurrencyTx(
	cs *concurrencyState,
	slotID string,
	configUpdate *taskqueuepb.ConcurrencyLimit,
	configUpdateVersion int64,
	pri int32,
	age time.Time,
) *concurrencyTx {
	return &concurrencyTx{
		cs:                  cs,
		slotID:              slotID,
		configUpdate:        configUpdate,
		configUpdateVersion: configUpdateVersion,
		pri:                 pri,
		age:                 age,
	}
}

func (ct *concurrencyTx) check(cb ReadinessCallback) error {
	return ct.cs.check(cb, ct.pri, ct.age)
}

func (ct *concurrencyTx) cancelCheck() {
	ct.cs.cancelCheck()
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

package fc

import (
	"context"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
)

type concurrencyTx struct {
	cs                  *concurrencyState
	slotID              string
	configUpdate        *taskqueuepb.ConcurrencyLimit
	configUpdateVersion int64
	pri                 wakePriority
	checkSeq            int64
	// latest known limiter state, from reserve, commit, or applyHint
	lastHint *taskqueuespb.LimiterHint
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
	ct.checkSeq, err = ct.cs.check(cb, ct.pri)
	return err
}

func (ct *concurrencyTx) cancelCheck() {
	ct.cs.cancelCheck(ct.checkSeq)
}

func (ct *concurrencyTx) reserve(ctx context.Context) error {
	hint, err := ct.cs.reserve(ctx, ct.slotID, ct.configUpdate, ct.configUpdateVersion)
	ct.recordHint(hint)
	return err
}

func (ct *concurrencyTx) commit(ctx context.Context) error {
	hint, err := ct.cs.commit(ctx, ct.slotID)
	ct.recordHint(hint)
	return err
}

func (ct *concurrencyTx) cancelReserve(ctx context.Context) {
	ct.cs.cancelReserve(ctx, ct.slotID)
}

func (ct *concurrencyTx) close() {
	ct.cs.unpin()
}

func (ct *concurrencyTx) hint() *taskqueuespb.LimiterHint {
	return ct.lastHint
}

func (ct *concurrencyTx) applyHint(hint *taskqueuespb.LimiterHint) {
	if hint.GetLimiterType() != enumsspb.LIMITER_TYPE_CONCURRENCY || hint.GetKey() != ct.cs.limiterKey() {
		return
	}
	ct.cs.reportSlotsHint(hint.GetGeneration(), hint.GetAvailableSlotsHint(), notifyAsync)
	ct.recordHint(hint)
}

func (ct *concurrencyTx) recordHint(hint *taskqueuespb.LimiterHint) {
	if hint != nil {
		ct.lastHint = hint
	}
}

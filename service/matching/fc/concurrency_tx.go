package fc

import (
	"context"

	"github.com/google/uuid"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
)

type concurrencyTx struct {
	cs                  *concurrencyState
	slotID              string // generated lazily, use getSlotID
	configUpdate        *taskqueuepb.ConcurrencyLimit
	configUpdateVersion int64
	pri                 wakePriority
	checkSeq            int64
	outstanding         bool // took a check token that's counted in cs.outstanding
}

func newConcurrencyTx(
	cs *concurrencyState,
	configUpdate *taskqueuepb.ConcurrencyLimit,
	configUpdateVersion int64,
	pri wakePriority,
) *concurrencyTx {
	return &concurrencyTx{
		cs:                  cs,
		configUpdate:        configUpdate,
		configUpdateVersion: configUpdateVersion,
		pri:                 pri,
	}
}

// getSlotID returns the slot id for this tx. It's generated lazily so we don't create it until
// we get to Reserve.
func (ct *concurrencyTx) getSlotID() string {
	if ct.slotID == "" {
		// TODO(fc): consider deriving slot id from task to fix some nongraceful failover situations
		ct.slotID = uuid.NewString()
	}
	return ct.slotID
}

func (ct *concurrencyTx) check(cb ReadinessCallback) error {
	var err error
	ct.checkSeq, err = ct.cs.check(cb, ct.pri)
	ct.outstanding = err == nil
	return err
}

func (ct *concurrencyTx) cancelCheck() {
	ct.cs.cancelCheck(ct.checkSeq, ct.outstanding)
	ct.outstanding = false
}

func (ct *concurrencyTx) reserve(ctx context.Context) error {
	counted := int32(0)
	if ct.outstanding {
		counted = 1
	}
	err := ct.cs.reserve(ctx, ct.getSlotID(), ct.configUpdate, ct.configUpdateVersion, counted)
	if ct.outstanding {
		ct.outstanding = false
		ct.cs.reserveDone()
	}
	return err
}

func (ct *concurrencyTx) commit(ctx context.Context) error {
	return ct.cs.commit(ctx, ct.getSlotID())
}

func (ct *concurrencyTx) cancelReserve(ctx context.Context) {
	ct.cs.cancelReserve(ctx, ct.getSlotID())
}

func (ct *concurrencyTx) close() {
	if ct.outstanding {
		// checked but never reserved or canceled (e.g. dispatch paths that only Check, or
		// tasks forwarded to another partition that reserves them)
		ct.outstanding = false
		ct.cs.dropOutstanding()
	}
	ct.cs.unpin()
}

func (ct *concurrencyTx) hint() *taskqueuespb.LimiterHint {
	return ct.cs.getSlotsHint()
}

func (ct *concurrencyTx) applyHint(hint *taskqueuespb.LimiterHint) {
	if hint.GetLimiterType() != enumsspb.LIMITER_TYPE_CONCURRENCY || hint.GetKey() != ct.cs.limiterKey() {
		return
	} else if hint.GetAvailableSlotsHint() < 0 {
		// invalid: negative tokens would never block in check
		return
	}
	ct.cs.reportSlotsHint(hint.GetGeneration(), hint.GetAvailableSlotsHint(), notifyAsync)
}

func (ct *concurrencyTx) ref() *taskqueuespb.LimiterRef {
	return &taskqueuespb.LimiterRef{
		LimiterType: enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:         ct.cs.limiterKey(),
		SlotId:      ct.getSlotID(),
	}
}

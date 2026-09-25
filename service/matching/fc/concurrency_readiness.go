package fc

import (
	"context"
	"sync"
	"time"

	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/namespace"
	serviceerrors "go.temporal.io/server/common/serviceerror"
	"go.temporal.io/server/common/util"
)

var ErrConcurrencyBlocked = serviceerror.NewFailedPrecondition("blocked by concurrency limit")

type concurrencyState struct {
	r    *Readiness
	nsID namespace.ID // TODO(fc): can we consolidate these?
	key  string

	lock       sync.Mutex
	generation int64
	tokens     int32
	// invariant: {len(waiters) > 0} == {Wait goroutine is running} == {goroCancel != nil}
	// (for now, until we add eviction)
	waiters    waiterEntries
	goroCancel context.CancelFunc
}

func (r *Readiness) getConcurrencyLimiter(nsID namespace.ID, key string) *concurrencyState {
	if cs, ok := r.concurrencyLimiters.Load(nsID.String() + key); ok {
		return cs.(*concurrencyState) // nolint:revive
	}
	cs, _ := r.concurrencyLimiters.LoadOrStore(nsID.String()+key, &concurrencyState{
		r:       r,
		nsID:    nsID,
		key:     key,
		waiters: *newWaiterEntries(),
	})
	return cs.(*concurrencyState) // nolint:revive
}

func (cs *concurrencyState) stop() {
	cs.update(func() error {
		cs.waiters.clear()
		// FIXME: cs.r.unregisterWaiter on each one
		return nil
	})
}

func (cs *concurrencyState) cancelWaiter(cb ReadinessCallback) {
	cs.update(func() error {
		cs.waiters.remove(cb)
		return nil
	})
}

func (cs *concurrencyState) check(cb ReadinessCallback, pri int32, age time.Time) error {
	return cs.update(func() error {
		if cs.tokens == 0 {
			cs.waiters.add(cb, pri, age)
			cs.r.registerWaiter(cs, cb)
			return ErrConcurrencyBlocked
		}

		// remove in case it was present before
		cs.waiters.remove(cb)
		cs.r.unregisterWaiter(cs, cb)
		// take one check token
		cs.tokens--
		return nil
	})
}

func (cs *concurrencyState) cancelCheck() {
	cs.update(func() error {
		// this could theoretically go over the limit but it doesn't matter here
		cs.tokens++
		return nil
	})
}

func (cs *concurrencyState) reserve(
	ctx context.Context,
	slotID string,
	configUpdate *taskqueuepb.ConcurrencyLimit,
	configUpdateVersion int64,
) error {
	res, err := cs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId:         cs.nsID.String(),
		Key:                 cs.key,
		ReserveSlots:        []string{slotID},
		ConfigUpdate:        configUpdate,
		ConfigUpdateVersion: configUpdateVersion,
	})
	if err != nil {
		return err // don't update cache on rpc error
	}
	cs.reportSlotsHint(res.Generation, res.AvailableSlotsHint)
	if !res.ReserveSuccess[0] {
		return serviceerrors.NewFlowControlBlocked()
	}
	return nil
}

func (cs *concurrencyState) commit(ctx context.Context, slotID string) error {
	res, err := cs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId: cs.nsID.String(),
		Key:         cs.key,
		CommitSlots: []string{slotID},
	})
	if err != nil {
		return err
	}
	cs.reportSlotsHint(res.Generation, res.AvailableSlotsHint)
	if !res.CommitSuccess[0] {
		return errCommitFailure
	}
	return nil
}

func (cs *concurrencyState) cancelReserve(ctx context.Context, slotID string) {
	// call in new goroutine, don't block here
	go func() {
		res, err := cs.r.concurrencyServiceClient.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
			NamespaceId:            cs.nsID.String(),
			Key:                    cs.key,
			CancelReservationSlots: []string{slotID},
		})
		if err == nil {
			cs.reportSlotsHint(res.Generation, res.AvailableSlotsHint)
		}
	}()
}

func (cs *concurrencyState) reportSlotsHint(gen int64, slots int32) {
	cs.update(func() error {
		if gen < cs.generation {
			return nil
		}
		cs.generation = gen

		// the first batched response will get the actual slots hint, the rest will be given -1 so
		// that we don't resupply tokens after a waiter that we wake takes them
		if slots >= 0 {
			cs.tokens = slots
		}
		return nil
	})
}

func (cs *concurrencyState) update(f func() error) error {
	var waiters []ReadinessCallback
	defer func() { notifyWaiters(waiters) }()

	cs.lock.Lock()
	defer cs.lock.Unlock()

	defer func() { waiters = cs.syncGoroLocked() }()

	return f()
}

func (cs *concurrencyState) syncGoroLocked() (out []ReadinessCallback) {
	// wake as many as we can. note that we do not take the tokens here, the waiter will do
	// that after it wakes up.
	out = cs.waiters.take(cs.tokens)

	haveWaiters := cs.waiters.len() > 0
	if (cs.goroCancel != nil) == haveWaiters {
		return
	}

	if !haveWaiters {
		cs.goroCancel()
		cs.goroCancel = nil
		return
	}

	// TODO(fc): put some limit on these, maybe two-stage lru
	ctx := headers.SetCallerInfo(context.Background(), headers.NewCallerInfo(
		cs.nsID.String(), // TODO(fc): use namespace name instead of id
		headers.CallerTypeBackgroundHigh,
		"",
	))
	ctx, cs.goroCancel = context.WithCancel(ctx)
	// Wait result will be reported back through ReportReady/Blocked
	go cs.callWait(ctx)

	return
}

func (cs *concurrencyState) makeWaitRequest() *fcpb.ConcurrencyWaitRequest {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	return &fcpb.ConcurrencyWaitRequest{
		NamespaceId:         cs.nsID.String(),
		Key:                 cs.key,
		Generation:          cs.generation,
		WakePriority:        int64(cs.waiters.minPriority()),
		RequestedWakeTokens: int32(cs.waiters.len()),
	}
}

func (cs *concurrencyState) callWait(ctx context.Context) {
	policy := backoff.NewExponentialRetryPolicy(time.Second).
		WithExpirationInterval(backoff.NoInterval).
		WithMaximumInterval(time.Minute)
	retrier := backoff.NewRetrier(policy, clock.NewRealTimeSource())

	for ctx.Err() == nil {
		// TODO(fc): if minPriority decreases during this call, interrupt and restart it.
		// increasing minPriority should not interrupt
		res, err := cs.r.concurrencyServiceClient.Wait(ctx, cs.makeWaitRequest())
		if err != nil {
			util.InterruptibleSleep(ctx, retrier.NextBackOff(err))
			continue
		}
		retrier.Reset()

		cs.reportSlotsHint(res.Generation, res.WakeTokens)
		// note: If we have satisfied all our waiters, then ctx
		// will be canceled before we continue this loop.
	}
}

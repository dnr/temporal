package fc

import (
	"context"
	"sync"
	"time"

	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/util"
)

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
	if cs, ok := r.concurrencyLimiters.Load(nsID + key); ok {
		return cs.(*localLimiterState) // nolint:revive
	}
	cs, _ = r.concurrencyLimiters.LoadOrStore(nsID+key, &concurrencyState{
		r:       r,
		nsID:    nsID,
		key:     key,
		waiters: *newWaiterEntries(),
	})
	return cs.(*concurrencyState) // nolint:revive
}

func (cs *concurrencyState) stop() {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	cs.waiters.clear()
	cs.syncGoroLocked(n, rkey)
}

// func (n *nsReadiness) cancelAllConcurrencyCallbacksLocked(cb ReadinessCallback) {
// 	// TODO(fc): this is unfortunate, maybe we should optimize this
// 	for key, cs := range n.concurrencyLimiters {
// 		cs.waiters.remove(cb)
// 		cs.syncGoroLocked(n, key)
// 	}
// }

// func (r *Readiness) reportConcurrencyReady(nsID namespace.ID, key string, gen int64, tokens int32) {
// 	r.getNS(nsID).reportConcurrencyReady(key, gen, tokens)
// }

func (cs *concurrencyState) check() error {
	ct.cs.lock.Lock()
	defer ct.cs.lock.Unlock()
	defer ct.cs.syncGoroLocked()

	if ct.cs.tokens == 0 {
		cs.waiters.add(cb, ct.pri, ct.age)
		return ErrConcurrencyBlocked
	}

	// remove in case it was present before
	cs.waiters.remove(cb)
	// take one check token
	ct.cs.tokens--
	return nil
}

func (cs *concurrencyState) cancelCheck() {
	var waiters []ReadinessCallback
	defer func() { notifyWaiters(waiters) }()

	ct.cs.lock.Lock()
	defer ct.cs.lock.Unock()
	defer ct.cs.syncGoroLocked()

	// this could theoretically go over the limit but it doesn't matter here
	ct.cs.tokens++
	waiters = ct.cs.waiters.take(cs.tokens)
}

func (cs *concurrencyState) reportWaitResponse(gen int64, tokens int32) {
	var waiters []ReadinessCallback
	defer func() { notifyWaiters(waiters) }()

	cs.lock.Lock()
	defer cs.lock.Unlock()

	if gen < cs.generation {
		return
	}

	// FIXME: should not set tokens multiple times from the same batch call
	cs.generation = gen
	cs.tokens = tokens

	waiters = cs.waiters.take(cs.tokens)
	cs.syncGoroLocked(n, key)
}

func (cs *concurrencyState) reportReserveFailed(gen int64) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	if gen < cs.generation {
		return
	}
	cs.generation = gen
	cs.tokens = 0
}

func (cs *concurrencyState) syncGoroLocked() {
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

		cs.reportWaitResponse(res.Generation, res.WakeTokens)
		// note: If we have satisfied all our waiters, then ctx
		// will be canceled before we continue this loop.
	}
}

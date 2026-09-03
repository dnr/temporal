package fc

import (
	"context"
	"time"

	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/util"
)

type concurrencyState struct {
	state      ReadinessState
	generation int64
	// invariant: {len(waiters) > 0} == {Wait goroutine is running} == {goroCancel != nil}
	// (for now, until we add eviction)
	waiters    map[ReadinessCallback]wakePriority
	goroCancel context.CancelFunc
	// TODO(fc): cache some limiter-specific state, e.g. slots free so that we can change to
	// not ready after taking last slot
}

func (n *nsReadiness) getConcurrencyLimiterLocked(key string) *concurrencyState {
	if cs, ok := n.concurrencyLimiters[key]; ok {
		return cs
	}
	cs := &concurrencyState{
		waiters: make(map[ReadinessCallback]wakePriority),
	}
	n.concurrencyLimiters[key] = cs
	return cs
}

func (n *nsReadiness) stopConcurrencyLocked() {
	for rkey, cs := range n.concurrencyLimiters {
		cs.waiters = nil
		cs.syncGoroLocked(n, rkey)
	}
}

func (n *nsReadiness) concurrencyReadinessState(
	key string,
	pri int32,
	age time.Time,
	cb ReadinessCallback,
) ReadinessState {
	n.lock.Lock()
	defer n.lock.Unlock()

	cs, ok := n.concurrencyLimiters[key]
	if !ok {
		// if missing from cache, return unknown. matching will probably try to Reserve and
		// based on result, call either reportConcurrencyReady or reportConcurrencyBlocked.
		return ReadinessUnknown
	}

	if cb != nil {
		// add callback if blocked, remove if unblocked
		if cs.state.Likely() {
			delete(cs.waiters, cb)
		} else {
			cs.waiters[cb] = makeWakePriority(pri, age)
		}
		cs.syncGoroLocked(n, key)
	}

	return cs.state
}

func (n *nsReadiness) cancelConcurrencyCallback(key string, cb ReadinessCallback) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if cs, ok := n.concurrencyLimiters[key]; ok {
		delete(cs.waiters, cb)
		cs.syncGoroLocked(n, key)
	}
}

func (n *nsReadiness) cancelAllConcurrencyCallbacksLocked(cb ReadinessCallback) {
	// TODO(fc): this is unfortunate, maybe we should optimize this
	for key, cs := range n.concurrencyLimiters {
		if _, ok := cs.waiters[cb]; ok {
			delete(cs.waiters, cb)
			cs.syncGoroLocked(n, key)
		}
	}
}

func (r *Readiness) reportConcurrencyReady(nsID namespace.ID, key string, gen int64) {
	r.getNS(nsID).reportConcurrencyReady(key, gen)
}

// reportConcurrencyReady is called when a Reserve or Wait call succeeds.
func (n *nsReadiness) reportConcurrencyReady(key string, gen int64) {
	n.lock.Lock()

	cs := n.getConcurrencyLimiterLocked(key)
	if gen < cs.generation {
		n.lock.Unlock()
		return
	}
	cs.generation = gen
	cs.state = ReadinessReady

	// TODO(fc): do staged wakeup similar to the distributed case
	waiters := cs.waiters
	cs.waiters = make(map[ReadinessCallback]wakePriority)
	cs.syncGoroLocked(n, key)

	n.lock.Unlock()

	for w := range waiters {
		w.OnReady()
	}
}

// reportConcurrencyBlocked is called when a Reserve or Wait call fails.
func (r *Readiness) reportConcurrencyBlocked(nsID namespace.ID, key string, gen int64) {
	r.getNS(nsID).reportConcurrencyBlocked(key, gen)
}

func (n *nsReadiness) reportConcurrencyBlocked(key string, gen int64) {
	n.lock.Lock()
	defer n.lock.Unlock()

	cs := n.getConcurrencyLimiterLocked(key)
	if gen < cs.generation {
		return
	}
	cs.generation = gen
	cs.state = ReadinessBlocked
}

func (cs *concurrencyState) syncGoroLocked(rn *nsReadiness, key string) {
	haveWaiters := len(cs.waiters) > 0
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
		rn.nsID.String(), // TODO(fc): use namespace name instead of id
		headers.CallerTypeBackgroundHigh,
		"",
	))
	ctx, cs.goroCancel = context.WithCancel(ctx)
	// Wait result will be reported back through ReportReady/Blocked
	go cs.callWait(ctx, rn, key)
}

func (cs *concurrencyState) minPriorityLocked() (minPriority wakePriority) {
	for _, p := range cs.waiters {
		minPriority = min(minPriority, p)
	}
	return
}

func (cs *concurrencyState) callWait(ctx context.Context, rn *nsReadiness, key string) {
	policy := backoff.NewExponentialRetryPolicy(time.Second).
		WithExpirationInterval(backoff.NoInterval).
		WithMaximumInterval(time.Minute)
	retrier := backoff.NewRetrier(policy, clock.NewRealTimeSource())

	for ctx.Err() == nil {
		rn.lock.Lock()
		req := &fcpb.ConcurrencyWaitRequest{
			NamespaceId:         rn.nsID.String(),
			Key:                 key,
			Generation:          cs.generation,
			WakePriority:        int64(cs.minPriorityLocked()),
			RequestedWakeTokens: int32(len(cs.waiters)),
		}
		rn.lock.Unlock()

		// TODO(fc): if minPriority decreases during this call, interrupt and restart it.
		// increasing minPriority should not interrupt
		res, err := rn.r.concurrencyServiceClient.Wait(ctx, req)
		if err != nil {
			util.InterruptibleSleep(ctx, retrier.NextBackOff(err))
			continue
		}
		retrier.Reset()

		if res.WakeTokens > 0 {
			rn.reportConcurrencyReady(key, res.Generation)
			// note: If we have satisfied all our waiters, then ctx
			// will be canceled before we continue this loop.
		} else {
			rn.reportConcurrencyBlocked(key, res.Generation)
		}
	}
}

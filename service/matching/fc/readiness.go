package fc

import (
	"context"
	"sync"
	"time"

	enumsspb "go.temporal.io/server/api/enums/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/util"
)

type ReadinessState int32

const (
	ReadinessUnknown ReadinessState = iota
	ReadinessBlocked
	ReadinessReady
)

func (s ReadinessState) Likely() bool {
	return s == ReadinessUnknown || s == ReadinessReady
}

type Readiness struct {
	timeSource               clock.TimeSource
	concurrencyServiceClient fcpb.ConcurrencyServiceClient

	caches sync.Map // namespaceID -> *readinessNS
}

func NewReadiness(
	timeSource clock.TimeSource,
	concurrencyServiceClient fcpb.ConcurrencyServiceClient,
) *Readiness {
	return &Readiness{
		timeSource:               timeSource,
		concurrencyServiceClient: concurrencyServiceClient,
	}
}

func (r *Readiness) Stop() {
	r.caches.Range(func(k, v any) bool {
		v.(*nsReadiness).stop() // nolint:revive
		return true
	})
	r.caches.Clear()
}

// ReadinessState gets the readiness state of a limiter. If it's blocked and cb is not nil,
// cb.OnReady will be called once when the state of the limiter transitions to ready. If it is
// ready, the callback will be removed from the limiter
//
// If we have too many subscriptions, we may drop some. In that case, we'll set a timer to call
// cb.OnReady with some backoff.
func (r *Readiness) ReadinessState(
	nsID namespace.ID,
	lim Limiter,
	pri int32,
	age time.Time,
	cb ReadinessCallback,
) ReadinessState {
	switch lim.Type {
	case enumsspb.LIMITER_TYPE_CONCURRENCY:
		return r.getNS(nsID).concurrencyReadinessState(lim.Key, pri, age, cb)
	case enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT:
		return r.getNS(nsID).localLimiterReadiness(lim.Key, lim.Config, cb)
	default:
		return ReadinessUnknown
	}
}

// CancelCallback cancels any future calls to cb.OnReady.
func (r *Readiness) CancelCallback(nsID namespace.ID, lim Limiter, cb ReadinessCallback) {
	switch lim.Type {
	case enumsspb.LIMITER_TYPE_CONCURRENCY:
		r.getNS(nsID).cancelConcurrencyCallback(lim.Key, cb)
	case enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT:
		r.getNS(nsID).cancelLocalLimiterCallback(lim.Key, cb)
	default:
	}
}

// per-ns state

type nsReadiness struct {
	r    *Readiness
	nsID namespace.ID

	lock sync.Mutex

	// LIMITER_TYPE_CONCURRENCY: key is concurrency limiter key (within ns)
	concurrencyLimiters map[string]*concurrencyState

	// LIMITER_TYPE_LOCAL_RATE_LIMIT: key is task queue name + type + partition
	localLimiters map[string]*localLimiterState

	// TODO(fc): clean up cache if entries are unused
	// TODO(fc): gauges for size of cache
}

func (r *Readiness) getNS(nsID namespace.ID) *nsReadiness {
	v, ok := r.caches.Load(nsID)
	if ok {
		return v.(*nsReadiness) // nolint:revive
	}
	newFcrn := &nsReadiness{
		r:                   r,
		nsID:                nsID,
		concurrencyLimiters: make(map[string]*concurrencyState),
		localLimiters:       make(map[string]*localLimiterState),
	}
	fcrn, _ := r.caches.LoadOrStore(nsID, newFcrn)
	return fcrn.(*nsReadiness) // nolint:revive
}

func (n *nsReadiness) stop() {
	n.lock.Lock()
	defer n.lock.Unlock()

	for rkey, v := range n.concurrencyLimiters {
		v.waiters = nil
		v.syncGoroLocked(n, rkey)
	}
}

// concurrency limiter readiness

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
	if v, ok := n.concurrencyLimiters[key]; ok {
		return v
	}
	v := &concurrencyState{
		waiters: make(map[ReadinessCallback]wakePriority),
	}
	n.concurrencyLimiters[key] = v
	return v
}

func (n *nsReadiness) concurrencyReadinessState(
	key string,
	pri int32,
	age time.Time,
	cb ReadinessCallback,
) ReadinessState {
	n.lock.Lock()
	defer n.lock.Unlock()

	v, ok := n.concurrencyLimiters[key]
	if !ok {
		// if missing from cache, return unknown. matching will probably try to Reserve and
		// based on result, call either reportConcurrencyReady or reportConcurrencyBlocked.
		return ReadinessUnknown
	}

	if cb != nil {
		// add callback if blocked, remove if unblocked
		if v.state.Likely() {
			delete(v.waiters, cb)
		} else {
			v.waiters[cb] = makeWakePriority(pri, age)
		}
		v.syncGoroLocked(n, key)
	}

	return v.state
}

func (n *nsReadiness) cancelConcurrencyCallback(key string, cb ReadinessCallback) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if v, ok := n.concurrencyLimiters[key]; ok {
		delete(v.waiters, cb)
		v.syncGoroLocked(n, key)
	}
}

func (r *Readiness) reportConcurrencyReady(nsID namespace.ID, key string, gen int64) {
	r.getNS(nsID).reportConcurrencyReady(key, gen)
}

// reportConcurrencyReady is called when a Reserve or Wait call succeeds.
func (n *nsReadiness) reportConcurrencyReady(key string, gen int64) {
	n.lock.Lock()

	v := n.getConcurrencyLimiterLocked(key)
	if gen < v.generation {
		n.lock.Unlock()
		return
	}
	v.generation = gen
	v.state = ReadinessReady

	// TODO(fc): do staged wakeup similar to the distributed case
	waiters := v.waiters
	v.waiters = make(map[ReadinessCallback]wakePriority)
	v.syncGoroLocked(n, key)

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

	v := n.getConcurrencyLimiterLocked(key)
	if gen < v.generation {
		return
	}
	v.generation = gen
	v.state = ReadinessBlocked
}

func (v *concurrencyState) syncGoroLocked(rn *nsReadiness, key string) {
	haveWaiters := len(v.waiters) > 0
	if (v.goroCancel != nil) == haveWaiters {
		return
	}

	if !haveWaiters {
		v.goroCancel()
		v.goroCancel = nil
		return
	}

	// TODO(fc): put some limit on these, maybe two-stage lru
	ctx := headers.SetCallerInfo(context.Background(), headers.NewCallerInfo(
		rn.nsID.String(), // TODO(fc): use namespace name instead of id
		headers.CallerTypeBackgroundHigh,
		"",
	))
	ctx, v.goroCancel = context.WithCancel(ctx)
	// Wait result will be reported back through ReportReady/Blocked
	go v.callWait(ctx, rn, key)
}

func (v *concurrencyState) minPriorityLocked() (minPriority wakePriority) {
	for _, p := range v.waiters {
		minPriority = min(minPriority, p)
	}
	return
}

func (v *concurrencyState) callWait(ctx context.Context, rn *nsReadiness, key string) {
	policy := backoff.NewExponentialRetryPolicy(time.Second).
		WithExpirationInterval(backoff.NoInterval).
		WithMaximumInterval(time.Minute)
	retrier := backoff.NewRetrier(policy, clock.NewRealTimeSource())

	for ctx.Err() == nil {
		rn.lock.Lock()
		req := &fcpb.ConcurrencyWaitRequest{
			NamespaceId:         rn.nsID.String(),
			Key:                 key,
			Generation:          v.generation,
			WakePriority:        int64(v.minPriorityLocked()),
			RequestedWakeTokens: int32(len(v.waiters)),
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

// local limiter readiness

type localLimiterState struct {
	timers map[ReadinessCallback]clock.Timer
}

func (n *nsReadiness) getLocalLimiterLocked(key string) *localLimiterState {
	if v, ok := n.localLimiters[key]; ok {
		return v
	}
	v := &localLimiterState{
		timers: make(map[ReadinessCallback]clock.Timer),
	}
	n.localLimiters[key] = v
	return v
}

func (n *nsReadiness) localLimiterReadiness(key string, config any, cb ReadinessCallback) ReadinessState {
	ll, ok := config.(LocalLimiter)
	if !ok {
		return ReadinessUnknown
	}
	delay := ll.Delay()

	n.lock.Lock()
	defer n.lock.Unlock()

	state := n.getLocalLimiterLocked(key)

	if delay > 0 {
		// set timer
		tmr, ok := state.timers[cb]
		if !ok {
			state.timers[cb] = n.r.timeSource.AfterFunc(delay, func() {
				n.lock.Lock()
				delete(state.timers, cb)
				n.lock.Unlock()
				cb.OnReady()
			})
		} else {
			// FIXME: should we do min? max? all? (per-key limit skips over priority)
			tmr.Reset(delay)
		}

		return ReadinessBlocked
	}

	// clear timer
	if tmr, ok := state.timers[cb]; ok {
		tmr.Stop()
		delete(state.timers, cb)
	}

	return ReadinessReady
}

func (n *nsReadiness) cancelLocalLimiterCallback(key string, cb ReadinessCallback) {
	n.lock.Lock()
	defer n.lock.Unlock()

	state, ok := n.localLimiters[key]
	if !ok {
		return
	}

	if tmr, ok := state.timers[cb]; ok {
		tmr.Stop()
		delete(state.timers, cb)
	}
}

// reportLocalLimiterReady is called after recycling tokens: the local limiter might be ready
// now so wake waiters.
func (r *Readiness) reportLocalLimiterReady(nsID namespace.ID, key string) {
	r.getNS(nsID).reportLocalLimiterReady(key)
}

// reportLocalLimiterReady is called after recycling tokens: the local limiter might be ready
// now so wake waiters.
func (n *nsReadiness) reportLocalLimiterReady(key string) {
	n.lock.Lock()

	state, ok := n.localLimiters[key]
	if !ok || len(state.timers) == 0 {
		n.lock.Unlock()
		return
	}

	timers := state.timers
	state.timers = make(map[ReadinessCallback]clock.Timer)

	for _, tmr := range timers {
		tmr.Stop()
	}

	n.lock.Unlock()

	for cb, _ := range timers {
		cb.OnReady()
	}
}

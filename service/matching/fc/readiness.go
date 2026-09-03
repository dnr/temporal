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

	// lock protects all state under concurrencyLimiters and localLimiters
	lock sync.Mutex

	// LIMITER_TYPE_CONCURRENCY: key is concurrency limiter key (within ns)
	concurrencyLimiters map[string]*concurrencyState

	// LIMITER_TYPE_LOCAL_RATE_LIMIT: key is task queue name + type + partition
	localLimiters map[string]*localLimiterState

	// TODO(fc): clean up cache if entries are unused
	// TODO(fc): gauges for size of cache
}

func (r *Readiness) getNS(nsID namespace.ID) *nsReadiness {
	n, ok := r.caches.Load(nsID)
	if ok {
		return n.(*nsReadiness) // nolint:revive
	}
	newN := &nsReadiness{
		r:                   r,
		nsID:                nsID,
		concurrencyLimiters: make(map[string]*concurrencyState),
		localLimiters:       make(map[string]*localLimiterState),
	}
	n, _ = r.caches.LoadOrStore(nsID, newN)
	return n.(*nsReadiness) // nolint:revive
}

func (n *nsReadiness) stop() {
	n.lock.Lock()
	defer n.lock.Unlock()

	for rkey, cs := range n.concurrencyLimiters {
		cs.waiters = nil
		cs.syncGoroLocked(n, rkey)
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
	if cs, ok := n.concurrencyLimiters[key]; ok {
		return cs
	}
	cs := &concurrencyState{
		waiters: make(map[ReadinessCallback]wakePriority),
	}
	n.concurrencyLimiters[key] = cs
	return cs
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

// local limiter readiness

type localLimiterState struct {
	timers map[ReadinessCallback]clock.Timer
}

func (n *nsReadiness) getLocalLimiterLocked(key string) *localLimiterState {
	if lls, ok := n.localLimiters[key]; ok {
		return lls
	}
	lls := &localLimiterState{
		timers: make(map[ReadinessCallback]clock.Timer),
	}
	n.localLimiters[key] = lls
	return lls
}

func (n *nsReadiness) localLimiterReadiness(key string, config any, cb ReadinessCallback) ReadinessState {
	ll, ok := config.(LocalLimiter)
	if !ok {
		return ReadinessUnknown
	}
	delay := ll.Delay()

	n.lock.Lock()
	defer n.lock.Unlock()

	lls := n.getLocalLimiterLocked(key)

	if delay > 0 {
		// set timer
		tmr, ok := lls.timers[cb]
		if !ok {
			lls.timers[cb] = n.r.timeSource.AfterFunc(delay, func() {
				n.lock.Lock()
				delete(lls.timers, cb)
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
	if tmr, ok := lls.timers[cb]; ok {
		tmr.Stop()
		delete(lls.timers, cb)
	}

	return ReadinessReady
}

func (n *nsReadiness) cancelLocalLimiterCallback(key string, cb ReadinessCallback) {
	n.lock.Lock()
	defer n.lock.Unlock()

	lls, ok := n.localLimiters[key]
	if !ok {
		return
	}

	if tmr, ok := lls.timers[cb]; ok {
		tmr.Stop()
		delete(lls.timers, cb)
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

	lls, ok := n.localLimiters[key]
	if !ok || len(lls.timers) == 0 {
		n.lock.Unlock()
		return
	}

	timers := lls.timers
	lls.timers = make(map[ReadinessCallback]clock.Timer)

	for _, tmr := range timers {
		tmr.Stop()
	}

	n.lock.Unlock()

	for cb, _ := range timers {
		cb.OnReady()
	}
}

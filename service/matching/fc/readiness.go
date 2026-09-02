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

type concurrencyLimiter struct {
	generation int64
	state      ReadinessState
	// invariant: {len(waiters) > 0} == {Wait goroutine is running} == {goroCancel != nil}
	// (for now, until we add eviction)
	waiters    map[ReadinessCallback]wakePriority
	goroCancel context.CancelFunc
	// TODO(fc): cache some limiter-specific state, e.g. slots free so that we can change to
	// not ready after taking last slot
}

type rlTimer struct {
	// FIXME: consolidate with matching.resettableTimer
	t *time.Timer
}

type readinessNS struct {
	r    *Readiness
	nsID namespace.ID

	lock sync.Mutex

	// LIMITER_TYPE_CONCURRENCY: key is concurrency limiter key (within ns)
	concurrencyLimiters map[string]*concurrencyLimiter

	// LIMITER_TYPE_LOCAL_RATE_LIMIT
	rltimers map[ReadinessCallback]*rlTimer

	// TODO(fc): clean up cache if entries are unused
	// TODO(fc): gauges for size of cache
}

type Readiness struct {
	concurrencyServiceClient fcpb.ConcurrencyServiceClient

	caches sync.Map // namespaceID -> *readinessNS
}

func NewReadiness(
	concurrencyServiceClient fcpb.ConcurrencyServiceClient,
) *Readiness {
	return &Readiness{
		concurrencyServiceClient: concurrencyServiceClient,
	}
}

func (r *Readiness) Stop() {
	r.caches.Range(func(k, v any) bool {
		v.(*readinessNS).Stop() // nolint:revive
		return true
	})
}

func (rn *readinessNS) Stop() {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	for rkey, v := range rn.concurrencyLimiters {
		v.waiters = nil
		v.syncGoroLocked(rn, rkey)
	}
}

func (r *Readiness) getNS(nsID namespace.ID) *readinessNS {
	v, ok := r.caches.Load(nsID)
	if ok {
		return v.(*readinessNS) // nolint:revive
	}
	newFcrn := &readinessNS{
		r:                   r,
		nsID:                nsID,
		concurrencyLimiters: make(map[string]*concurrencyLimiter),
		rltimers:            make(map[ReadinessCallback]*rlTimer),
	}
	fcrn, _ := r.caches.LoadOrStore(nsID, newFcrn)
	return fcrn.(*readinessNS) // nolint:revive
}

func (rn *readinessNS) getConcurrencyLimiterLocked(key string) *concurrencyLimiter {
	if v, ok := rn.concurrencyLimiters[key]; ok {
		return v
	}
	v := &concurrencyLimiter{
		waiters: make(map[ReadinessCallback]wakePriority),
	}
	rn.concurrencyLimiters[key] = v
	return v
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
		return r.getNS(nsID).localLimiterReadiness(lim.Config, cb)
	default:
		return ReadinessUnknown
	}
}

func (rn *readinessNS) concurrencyReadinessState(
	key string,
	pri int32,
	age time.Time,
	cb ReadinessCallback,
) ReadinessState {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	v, ok := rn.concurrencyLimiters[key]
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
		v.syncGoroLocked(rn, key)
	}

	return v.state
}

// CancelCallback cancels any future calls to cb.OnReady.
func (r *Readiness) CancelCallback(nsID namespace.ID, lim Limiter, cb ReadinessCallback) {
	switch lim.Type {
	case enumsspb.LIMITER_TYPE_CONCURRENCY:
		r.getNS(nsID).cancelConcurrencyCallback(lim.Key, cb)
	case enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT:
		r.getNS(nsID).cancelLocalLimiterCallback(cb)
	default:
	}
}

func (rn *readinessNS) cancelConcurrencyCallback(key string, cb ReadinessCallback) {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	if v, ok := rn.concurrencyLimiters[key]; ok {
		delete(v.waiters, cb)
		v.syncGoroLocked(rn, key)
	}
}

func (r *Readiness) reportConcurrencyReady(nsID namespace.ID, key string, gen int64) {
	r.getNS(nsID).reportConcurrencyReady(key, gen)
}

// reportConcurrencyReady is called when a Reserve or Wait call succeeds.
func (rn *readinessNS) reportConcurrencyReady(key string, gen int64) {
	rn.lock.Lock()

	v := rn.getConcurrencyLimiterLocked(key)
	if gen < v.generation {
		rn.lock.Unlock()
		return
	}
	v.generation = gen
	v.state = ReadinessReady

	// TODO(fc): do staged wakeup similar to the distributed case
	waiters := v.waiters
	v.waiters = make(map[ReadinessCallback]wakePriority)
	v.syncGoroLocked(rn, key)

	rn.lock.Unlock()

	for w := range waiters {
		w.OnReady()
	}
}

// reportConcurrencyBlocked is called when a Reserve or Wait call fails.
func (r *Readiness) reportConcurrencyBlocked(nsID namespace.ID, key string, gen int64) {
	r.getNS(nsID).reportConcurrencyBlocked(key, gen)
}

func (rn *readinessNS) reportConcurrencyBlocked(key string, gen int64) {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	v := rn.getConcurrencyLimiterLocked(key)
	if gen < v.generation {
		return
	}
	v.generation = gen
	v.state = ReadinessBlocked
}

func (v *concurrencyLimiter) syncGoroLocked(rn *readinessNS, key string) {
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

func (v *concurrencyLimiter) minPriorityLocked() (minPriority wakePriority) {
	for _, p := range v.waiters {
		minPriority = min(minPriority, p)
	}
	return
}

func (v *concurrencyLimiter) callWait(ctx context.Context, rn *readinessNS, key string) {
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

// rate limit

func (rn *readinessNS) localLimiterReadiness(config any, cb ReadinessCallback) ReadinessState {
	ll, ok := config.(LocalLimiter)
	if !ok {
		return ReadinessUnknown
	}
	delay := ll.Delay()

	rn.lock.Lock()
	defer rn.lock.Unlock()

	if delay > 0 {
		// set timer
		tmr, ok := rn.rltimers[cb]
		if !ok {
			tmr = &rlTimer{}
		}
		if tmr.t == nil {
			tmr.t = time.AfterFunc(delay, func() {
				rn.lock.Lock()
				delete(rn.rltimers, cb)
				rn.lock.Unlock()
				cb.OnReady()
			})
		} else {
			// FIXME: should we do min? max? hmm...
			tmr.t.Reset(delay)
		}

		return ReadinessBlocked
	}

	// clear timer
	if tmr, ok := rn.rltimers[cb]; ok {
		if tmr.t != nil {
			tmr.t.Stop()
		}
		delete(rn.rltimers, cb)
	}

	return ReadinessReady
}

func (rn *readinessNS) cancelLocalLimiterCallback(cb ReadinessCallback) {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	if tmr, ok := rn.rltimers[cb]; ok {
		if tmr.t != nil {
			tmr.t.Stop()
		}
		delete(rn.rltimers, cb)
	}
}

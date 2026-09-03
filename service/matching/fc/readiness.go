package fc

import (
	"sync"
	"time"

	enumsspb "go.temporal.io/server/api/enums/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/namespace"
)

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

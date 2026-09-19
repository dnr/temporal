package fc

import (
	"sync"

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

func (r *Readiness) CancelAllCallbacks(nsID namespace.ID, cb ReadinessCallback) {
	r.getNS(nsID).cancelAllCallbacks(cb)
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

	n.stopConcurrencyLocked()
	n.stopLocalLimitersLocked()
}

func (n *nsReadiness) cancelAllCallbacks(cb ReadinessCallback) {
	n.lock.Lock()
	defer n.lock.Unlock()

	n.cancelAllConcurrencyCallbacksLocked(cb)
	n.cancelAllLocalLimiterCallbacksLocked(cb)
}

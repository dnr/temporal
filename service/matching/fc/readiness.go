package fc

import (
	"slices"
	"sync"

	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/clock"
)

type Readiness struct {
	timeSource               clock.TimeSource
	concurrencyServiceClient fcpb.ConcurrencyServiceClient

	// TODO(fc): should be sharded maps?
	// for LIMITER_TYPE_CONCURRENCY
	concurrencyLimiters sync.Map // nsID+key -> *concurrencyState
	// for LIMITER_TYPE_LOCAL_RATE_LIMIT
	localLimiters sync.Map // nsID+key -> *localLimiterState

	// FIXME: ugh, these are not atomically changed with the forward maps
	// reverse map from waiters to state
	allWaitersLock sync.Mutex
	allWaiters     map[ReadinessCallback][]limiterState

	// TODO(fc): clean up caches of idle/stale state
	// TODO(fc): gauges for size of cache
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
	r.concurrencyLimiters.Range(func(_, v any) bool {
		v.(*concurrencyState).stop() // nolint:revive
		return true
	})
	r.concurrencyLimiters.Clear()
	r.localLimiters.Range(func(_, v any) bool {
		v.(*localLimiterState).stop() // nolint:revive
		return true
	})
	r.localLimiters.Clear()
}

func (r *Readiness) CancelAllCallbacks(cb ReadinessCallback) {
	r.allWaitersLock.Lock()
	defer r.allWaitersLock.Unlock()

	// FIXME: this can deadlock
	if limiters, ok := r.allWaiters[cb]; ok {
		for _, l := range limiters {
			l.cancelWaiter(cb)
		}
		delete(r.allWaiters, cb)
	}
}

// interface for use by reverse waiters map
type limiterState interface {
	cancelWaiter(ReadinessCallback)
}

func (r *Readiness) registerWaiter(state limiterState, cb ReadinessCallback) {
	r.allWaitersLock.Lock()
	defer r.allWaitersLock.Unlock()

	limiters := r.allWaiters[cb]
	limiters = append(limiters, state)
	r.allWaiters[cb] = limiters
}

func (r *Readiness) unregisterWaiter(state limiterState, cb ReadinessCallback) {
	r.allWaitersLock.Lock()
	defer r.allWaitersLock.Unlock()

	if limiters, ok := r.allWaiters[cb]; ok {
		limiters = slices.DeleteFunc(limiters, func(l limiterState) bool {
			return l == state
		})
		r.allWaiters[cb] = limiters
	}
}

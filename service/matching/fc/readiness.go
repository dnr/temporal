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

	// TODO(fc): should be sharded maps?
	// for LIMITER_TYPE_CONCURRENCY
	concurrencyLimiters sync.Map // nsID+key -> *concurrencyState
	// for LIMITER_TYPE_LOCAL_RATE_LIMIT
	localLimiters sync.Map // nsID+key -> *localLimiterState

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

func (r *Readiness) CancelAllCallbacks(nsID namespace.ID, cb ReadinessCallback) {
	// FIXME: reverse mapping??
	// r.getNS(nsID).cancelAllCallbacks(cb)
}

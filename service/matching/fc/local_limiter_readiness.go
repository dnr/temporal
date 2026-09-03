package fc

import (
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/namespace"
)

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

func (n *nsReadiness) stopLocalLimitersLocked() {
	for _, lls := range n.localLimiters {
		for _, tmr := range lls.timers {
			tmr.Stop()
		}
	}
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

func (n *nsReadiness) cancelAllLocalLimiterCallbacksLocked(cb ReadinessCallback) {
	// TODO(fc): this is unfortunate, maybe we should optimize this
	for _, lls := range n.localLimiters {
		if tmr, ok := lls.timers[cb]; ok {
			delete(lls.timers, cb)
			tmr.Stop()
		}
	}
}

// reportLocalLimiterReady is called after recycling tokens: the local limiter might be ready
// now so wake waiters.
func (r *Readiness) reportLocalLimiterReady(nsID namespace.ID, key string) {
	r.getNS(nsID).reportLocalLimiterReady(key)
}

// reportLocalLimiterReady is called after recycling tokens: the local limiter might be ready
// now so wake waiters.
// TODO(fc): optimization: we could wake only one here instead of all
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

	for cb := range timers {
		cb.OnReady()
	}
}

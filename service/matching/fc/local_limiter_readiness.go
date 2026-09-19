package fc

import (
	"go.temporal.io/server/common/clock"
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

func (n *nsReadiness) cancelAllLocalLimiterCallbacksLocked(cb ReadinessCallback) {
	// TODO(fc): this is unfortunate, maybe we should optimize this
	for _, lls := range n.localLimiters {
		if tmr, ok := lls.timers[cb]; ok {
			delete(lls.timers, cb)
			tmr.Stop()
		}
	}
}

// stopAndWakeLocalLimiter cancels one local limiter callback and calls one or more of the
// other ones registered for that limiter, if any. This is called after recycling tokens.
func (n *nsReadiness) stopAndWakeLocalLimiter(key string, cancelCb ReadinessCallback) {
	n.lock.Lock()

	lls, ok := n.localLimiters[key]
	if !ok || len(lls.timers) == 0 {
		n.lock.Unlock()
		return
	}

	// remove the one we don't want anymore
	lls.stopLocked(cancelCb)

	if len(lls.timers) == 0 {
		n.lock.Unlock()
		return
	}

	// if there are any others, wake them all
	// TODO(fc): optimization: we could wake only one here instead of all
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

func (lls *localLimiterState) stopLocked(cb ReadinessCallback) {
	if tmr, ok := lls.timers[cb]; ok {
		tmr.Stop()
		delete(lls.timers, cb)
	}
}

package fc

import (
	"go.temporal.io/api/serviceerror"
	enumsspb "go.temporal.io/server/api/enums/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/service/matching/simplelimiter"
)

// GrantLocalTokens consumes up to requested tokens from all local rate limiters atomically.
// Eager dispatch uses this to share rate-limit capacity with regular dispatch.
func (r *Readiness) GrantLocalTokens(nsID namespace.ID, limiters *Limiters, requested int32) int32 {
	if requested <= 0 {
		return 0
	}
	rs := r.getShard(nsID)
	var states []*localLimiterState
	var configs []simplelimiter.Params
	defer func() {
		for _, lls := range states {
			lls.unpin()
		}
	}()
	if limiters != nil {
		for _, lim := range limiters.Limiters {
			if !lim.Valid() || lim.Type != enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT {
				continue
			}
			lls := rs.getLocalLimiter(nsID, lim.Key)
			states = append(states, lls)
			config, ok := lim.Config.(simplelimiter.Params)
			if !ok {
				config = simplelimiter.NoLimit()
			}
			configs = append(configs, config)
		}
	}
	var toNotify deferedNotify
	defer toNotify.notify(notifyAsync)
	rs.lock.Lock()
	defer rs.lock.Unlock()
	if rs.stopped {
		return 0
	}
	now := r.timeSource.Now().UnixNano()
	granted := requested
	for i, lls := range states {
		lls.params = configs[i]
		lls.lim = lls.lim.Clip(lls.params, now, maxLocalLimiterTokens)
		granted = min(granted, lls.lim.Available(lls.params, now))
	}
	for _, lls := range states {
		if granted > 0 {
			lls.lim = lls.lim.Consume(lls.params, now, int64(granted))
		}
		rs.syncLimiter(lls, &toNotify)
	}
	return granted
}

// maxLocalLimiterTokens is the maximum number of tokens we might consume at a time for
// simplelimiter.Limiter. This is used to update ready times after a rate is changed from very
// low (or zero) to higher: we may have set a ready time far in the future and need to clip it
// to something reasonable so we can dispatch again.
//
// Currently we only use 1 token at a time.
const maxLocalLimiterTokens = 1

var ErrLocalLimiterBlocked = serviceerror.NewFailedPrecondition("blocked by local limiter")

type localLimiterState struct {
	limiterStateBase

	params simplelimiter.Params
	lim    simplelimiter.Ready
	// invariant: lim.Delay() < 0 -> len(waiters) == 0
	// invariant: {len(waiters) > 0} == {tmr != nil}
	tmr clock.Timer
}

func (r *Readiness) getLocalLimiter(nsID namespace.ID, key string) *localLimiterState {
	return r.getShard(nsID).getLocalLimiter(nsID, key)
}

func (rs *readinessShard) getLocalLimiter(nsID namespace.ID, key string) *localLimiterState {
	rs.lock.Lock()
	defer rs.lock.Unlock()

	mapkey := nsID.String() + key
	if lls, ok := rs.localLimiters[mapkey]; ok {
		rs.pinLocked(lls)
		return lls
	}
	lls := &localLimiterState{
		limiterStateBase: limiterStateBase{rs: rs, pins: 1, mapKey: mapkey, namespaceLen: uint16(len(nsID))},
		params:           simplelimiter.NoLimit(),
	}
	rs.localLimiters[mapkey] = lls
	rs.created++
	return lls
}

func (lls *localLimiterState) now() int64 {
	return lls.rs.r.timeSource.Now().UnixNano()
}

func (lls *localLimiterState) check(config simplelimiter.Params, cb ReadinessCallback, pri wakePriority) error {
	return lls.rs.update(lls, notifyAsync, func() error {
		now := lls.now()

		// install new params
		lls.params = config
		// Clip to handle the case where we have increased from a zero or very low limit and had
		// ready times in the far future.
		lls.lim = lls.lim.Clip(lls.params, now, maxLocalLimiterTokens)

		if delay := lls.lim.Delay(now); delay > 0 {
			lls.rs.addEdgeLocked(lls, cb, pri)
			return ErrLocalLimiterBlocked
		}

		// remove in case it was present before
		lls.rs.removeEdgeLocked(lls, cb)
		// now we can take the token
		lls.lim = lls.lim.Consume(lls.params, now, 1)
		return nil
	})
}

func (lls *localLimiterState) cancelCheck() {
	lls.rs.update(lls, notifyAsync, func() error {
		now := lls.now()
		// return the token
		lls.lim = lls.lim.Consume(lls.params, now, -1)
		return nil
	})
}

func (lls *localLimiterState) syncLocked(rsu *readinessSyncUpdate) {
	now := lls.now()

	// use a local copy to figure out how many we can wake. note that we do not take from the
	// actual limiter here, since the waiter will do that after it wakes up and calls check.
	lim := lls.lim
	for lim.Delay(now) <= 0 && len(rsu.waiters) > 0 {
		rsu.wake(1)
		lim = lim.Consume(lls.params, now, 1)
	}

	if len(rsu.waiters) == 0 {
		if lls.tmr != nil {
			if lls.tmr.Stop() {
				lls.pins--
			}
			lls.tmr = nil
		}
		return
	}

	delay := lim.Delay(now)
	if lls.tmr != nil {
		if !lls.tmr.Reset(delay) {
			lls.pins++
		}
	} else {
		lls.pins++
		lls.tmr = lls.rs.r.timeSource.AfterFunc(delay, lls.onTimer)
	}
	return
}

func (lls *localLimiterState) onTimer() {
	defer lls.unpin()
	lls.rs.update(lls, notifySync, func() error { return nil })
}

func (lls *localLimiterState) droppableLocked(params gcParameters) bool {
	// A localLimiterState is droppable if it would be initialized in the same effective state
	// that it's in now. That's trivially true if it's completely blocked (Never) or has no
	// limit (!Limited). For a positive non-infinite limit, it would be initialized with a full
	// burst allowed, so return true if a full burst would be allowed right now.
	return lls.params.Never() || !lls.params.Limited() || lls.lim.Delay(params.now.UnixNano()) <= -lls.params.Burst
}

func (lls *localLimiterState) unpin() {
	lls.rs.unpin(lls)
}

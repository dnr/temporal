package fc

import (
	"sync"
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/service/matching/simplelimiter"
)

var ErrLocalLimiterBlocked = serviceerror.NewFailedPrecondition("blocked by local limiter")

type localLimiterState struct {
	r *Readiness

	lock   sync.Mutex
	params simplelimiter.Params
	lim    simplelimiter.Limiter
	// invariant: lim.Delay() < 0 -> len(waiters) == 0
	// invariant: {len(waiters) > 0} == {tmr != nil}
	waiters waiterEntries
	tmr     clock.Timer
}

func (r *Readiness) getLocalLimiter(nsID namespace.ID, key string) *localLimiterState {
	if lls, ok := r.localLimiters.Load(nsID.String() + key); ok {
		return lls.(*localLimiterState) // nolint:revive
	}
	lls, _ := r.localLimiters.LoadOrStore(nsID.String()+key, &localLimiterState{
		r:       r,
		params:  simplelimiter.NoLimitParams(),
		waiters: *newWaiterEntries(),
	})
	return lls.(*localLimiterState) // nolint:revive
}

func (lls *localLimiterState) stop() {
	lls.update(func(now int64) error {
		lls.waiters.clear()
		// FIXME: lls.r.unregisterWaiter on each one
		return nil
	})
}

func (lls *localLimiterState) cancelWaiter(cb ReadinessCallback) {
	lls.update(func(now int64) error {
		lls.waiters.remove(cb)
		return nil
	})
}

func (lls *localLimiterState) check(config simplelimiter.Params, cb ReadinessCallback, pri int32, age time.Time) error {
	return lls.update(func(now int64) error {

		// install new params
		lls.params = config

		if delay := lls.lim.Delay(now); delay > 0 {
			lls.waiters.add(cb, pri, age)
			lls.r.registerWaiter(lls, cb)
			return ErrLocalLimiterBlocked
		}

		// remove in case it was present before
		lls.waiters.remove(cb)
		lls.r.unregisterWaiter(lls, cb)
		// now we can take the token
		lls.lim = lls.lim.Consume(lls.params, now, 1)
		return nil
	})
}

func (lls *localLimiterState) cancelCheck() {
	lls.update(func(now int64) error {
		// return the token
		lls.lim = lls.lim.Consume(lls.params, now, -1)
		return nil
	})
}

func (lls *localLimiterState) update(f func(now int64) error) error {
	var waiters []ReadinessCallback
	defer func() { notifyWaiters(waiters) }()

	lls.lock.Lock()
	defer lls.lock.Unlock()

	now := lls.r.timeSource.Now().UnixNano()
	defer func() { waiters = lls.syncTimerLocked(now) }()

	return f(now)
}

func (lls *localLimiterState) syncTimerLocked(now int64) (out []ReadinessCallback) {
	// use a local copy to figure out how many we can wake. note that we do not consume from
	// the actual limiter here, since the waiter will do that after it wakes up.
	lim := lls.lim
	for lim.Delay(now) <= 0 && lls.waiters.len() > 0 {
		if cb, ok := lls.waiters.takeOne(); ok {
			out = append(out, cb)
			lim = lim.Consume(lls.params, now, 1)
		}
	}

	if lls.waiters.len() == 0 {
		if lls.tmr != nil {
			lls.tmr.Stop()
			lls.tmr = nil
		}
		return
	}

	delay := lim.Delay(now)
	if lls.tmr != nil {
		lls.tmr.Reset(delay)
	} else {
		lls.tmr = lls.r.timeSource.AfterFunc(delay, lls.onTimer)
	}
	return
}

func (lls *localLimiterState) onTimer() {
	lls.update(func(int64) error { return nil })
}

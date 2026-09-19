package fc

import (
	"context"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/common/namespace"
)

var ErrLocalLimiterBlocked = serviceerror.NewFailedPrecondition("blocked by local limiter")

type localLimiterTx struct {
	r    *Readiness
	nsID namespace.ID
	key  string
	ll   LocalLimiter
}

func newLocalLimiterTx(
	r *Readiness, // FIXME: whole thing?
	nsID namespace.ID,
	lim Limiter,
) *localLimiterTx {
	ll, _ := lim.Config.(LocalLimiter) // if this fails raise an error in check/reserve, not here
	return &localLimiterTx{
		r:    r,
		nsID: nsID,
		key:  lim.Key,
		ll:   ll,
	}
}

func (c *localLimiterTx) check(cb ReadinessCallback) error {
	if c.ll == nil {
		return serviceerror.NewInternal("localLimiterCommitter got wrong type")
	}

	n := c.r.getNS(c.nsID)
	n.lock.Lock()
	defer n.lock.Unlock()

	lls := n.getLocalLimiterLocked(c.key)

	if delay := c.ll.Delay(); delay > 0 {
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
			// FIXME: split whole queue and per-key limits to fix this
			tmr.Reset(delay)
		}

		return ErrLocalLimiterBlocked
	}

	// clear timer if there was one before
	lls.stopLocked(cb)

	// now we can take the token
	c.ll.Consume(1)

	return nil
}

func (c *localLimiterTx) cancelCheck(cb ReadinessCallback) {
	if c.ll == nil {
		return
	}

	// return the token
	c.ll.Consume(-1)

	// since we returned tokens, a waiter might be ready to go now
	n := c.r.getNS(c.nsID)
	n.stopAndWakeLocalLimiter(c.key, cb)
}

func (c *localLimiterTx) reserve(context.Context) error {
	return nil // check/cancelCheck is all we need here
}

func (c *localLimiterTx) commit(context.Context) error {
	return nil // check/cancelCheck is all we need here
}

func (c *localLimiterTx) cancelReserve(context.Context) {
}

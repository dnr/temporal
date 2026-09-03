package fc

import (
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/common/namespace"
)

// subset of Readiness
type readinessCacheLocalLimiterInterface interface {
	reportLocalLimiterReady(namespace.ID, string)
}

type localLimiterCommitter struct {
	cache readinessCacheLocalLimiterInterface
	nsID  namespace.ID
	key   string
	ll    LocalLimiter
}

func newLocalLimiterCommitter(
	cache readinessCacheLocalLimiterInterface,
	nsID namespace.ID,
	lim Limiter,
) *localLimiterCommitter {
	ll, _ := lim.Config.(LocalLimiter) // if this fails raise an error in reserve, not here
	return &localLimiterCommitter{
		cache: cache,
		nsID:  nsID,
		key:   lim.Key,
		ll:    ll,
	}
}

func (c *localLimiterCommitter) reserve() error {
	if c.ll == nil {
		return serviceerror.NewInternal("localLimiterCommitter got wrong type")
	}
	// by the time we get here, we already checked and allowed the right number of tasks to
	// match in the matcher, so just deduct the tokens.
	c.ll.Consume(1)
	return nil
}

func (c *localLimiterCommitter) commit() error {
	return nil // reserve is all we need here
}

func (c *localLimiterCommitter) cancelReservations() {
	if c.ll == nil {
		return
	}
	c.ll.Consume(-1)
	// since we returned tokens, a waiter might be ready to go now
	c.cache.reportLocalLimiterReady(c.nsID, c.key)
}

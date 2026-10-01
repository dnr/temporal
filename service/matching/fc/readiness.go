package fc

import (
	"hash/maphash"
	"slices"
	"sync"

	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/namespace"
)

const (
	// Shards should be a power of 2
	readinessStateShards = 32
)

type (
	Readiness struct {
		timeSource               clock.TimeSource
		concurrencyServiceClient fcpb.ConcurrencyServiceClient

		// Readiness state sharding and locking:
		//
		// The sharding and locking scheme tries to achieve high concurrency, given that we have to
		// maintain maps in two directions: limiters to callbacks, and callbacks to limiters, in a
		// many-to-many relationship. We also want the ability to do some processing over all
		// limiters.
		//
		// Overall, we shard the maps based on a hash of the namespace id: limiter<->callback
		// relations can't cross namespaces, so this is safe. Each shard uses a mutex to
		// synchronize access to its maps.
		shards [readinessStateShards]readinessShard

		seed maphash.Seed
	}

	readinessShard struct {
		r *Readiness

		// lock protects the maps below and everything in concurrencyState and
		// localLimiterState.
		lock sync.Mutex

		// for LIMITER_TYPE_CONCURRENCY
		concurrencyLimiters map[string]*concurrencyState // key is nsID+key
		// for LIMITER_TYPE_LOCAL_RATE_LIMIT
		localLimiters map[string]*localLimiterState // key is nsID+key

		// forward map, sorted by <pri, pointer(cb)>
		fwd map[limiterState][]fwdMapEntry
		// reverse map, sorted by <pointer(limiter), pri>
		rev map[ReadinessCallback][]revMapEntry

		// TODO(fc): clean up caches of idle/stale state
		// TODO(fc): gauges for size of cache
		// TODO(fc): limit active waiters, add suspended state, etc.

		_ [64 - 8 - 8 - 8 - 8 - 8]byte // force to different cache lines to avoid false sharing
	}

	fwdMapEntry struct {
		pri wakePriority
		cb  ReadinessCallback
	}
	revMapEntry struct {
		limiter limiterState
		pri     wakePriority
	}
	limiterState interface {
		syncLocked(*readinessSyncUpdate)
	}

	readinessSyncUpdate struct {
		waiters []fwdMapEntry
	}
	deferedNotify []ReadinessCallback
)

func NewReadiness(
	timeSource clock.TimeSource,
	concurrencyServiceClient fcpb.ConcurrencyServiceClient,
) *Readiness {
	r := &Readiness{
		timeSource:               timeSource,
		concurrencyServiceClient: concurrencyServiceClient,
		seed:                     maphash.MakeSeed(),
	}
	for i := range r.shards {
		rs := &r.shards[i]
		rs.r = r
		rs.concurrencyLimiters = make(map[string]*concurrencyState)
		rs.localLimiters = make(map[string]*localLimiterState)
		rs.fwd = make(map[limiterState][]fwdMapEntry)
		rs.rev = make(map[ReadinessCallback][]revMapEntry)
	}
	return r
}

func (r *Readiness) getShardIdx(nsID namespace.ID) int {
	return int(maphash.String(r.seed, nsID.String()) % readinessStateShards)
}

func (r *Readiness) getShard(nsID namespace.ID) *readinessShard {
	return &r.shards[r.getShardIdx(nsID)]
}

func (r *Readiness) Stop() {
	for i := range r.shards {
		r.shards[i].stop()
	}
}

func (rs *readinessShard) stop() {
	rs.lock.Lock()
	defer rs.lock.Lock()

	clear(rs.fwd)
	clear(rs.rev)
	for _, v := range rs.concurrencyLimiters {
		v.syncLocked(nil)
	}
	clear(rs.concurrencyLimiters)
	for _, v := range rs.localLimiters {
		v.syncLocked(nil)
	}
	clear(rs.localLimiters)
}

func (r *Readiness) CancelAllCallbacks(nsID namespace.ID, cb ReadinessCallback) {
	r.getShard(nsID).cancelAllCallbacks(cb)
}

func (rs *readinessShard) cancelAllCallbacks(cb ReadinessCallback) {
	// It would be quite unlikely, but it's possible for a limiter to decide to notify _other_
	// callbacks when we sync it. e.g. a rate limit due to the wall clock time advancing. The
	// timer would usually get there first but we may need to call some here.
	var toNotify deferedNotify
	defer toNotify.notify() // outside lock

	rs.lock.Lock()
	defer rs.lock.Lock()

	rEnts := rs.rev[cb]
	for _, rEnt := range rEnts {
		rs.removeEdgeLocked(rEnt.limiter, cb)
		rs.syncLimiter(rEnt.limiter, &toNotify)
	}
}

func (rs *readinessShard) syncLimiter(limiter limiterState, toNotify *deferedNotify) {
	// call syncLocked with current waiter set
	fEnts := rs.fwd[limiter]
	rsu := readinessSyncUpdate{waiters: fEnts}
	// syncLocked may call rsu.wake(), which will remove elements from the front of rsu.waiters
	limiter.syncLocked(&rsu)

	// record and remove the ones that we're going to notify
	woke := len(fEnts) - len(rsu.waiters)
	for _, fEnt := range fEnts[:woke] {
		toNotify.add(fEnt.cb)
		rs.removeEdgeLocked(limiter, fEnt.cb)
	}
}

func (rs *readinessShard) getWaiters(limiter limiterState) []fwdMapEntry {
	rs.lock.Lock()
	defer rs.lock.Unlock()

	return rs.fwd[limiter]
}

func (rs *readinessShard) addEdgeLocked(limiter limiterState, cb ReadinessCallback, pri wakePriority) {
	// FIXME: insert at right index to keep sorted
	rs.fwd[limiter] = append(rs.fwd[limiter], fwdMapEntry{pri: pri, cb: cb})
	// FIXME: insert at right index to sort
	rs.rev[cb] = append(rs.rev[cb], revMapEntry{pri: pri, limiter: limiter})
}

func (rs *readinessShard) removeEdgeLocked(limiter limiterState, cb ReadinessCallback) {
	rEnts := rs.rev[cb]
	// FIXME: binary search
	rEnts = slices.DeleteFunc(rEnts, func(rEnt revMapEntry) bool {
		return rEnt.limiter == limiter
	})
	if len(rEnts) == 0 {
		delete(rs.rev, cb)
	} else {
		rs.rev[cb] = rEnts
	}

	fEnts := rs.fwd[limiter]
	// FIXME: binary search
	fEnts = slices.DeleteFunc(fEnts, func(fEnt fwdMapEntry) bool {
		return fEnt.cb == cb
	})
	if len(fEnts) == 0 {
		delete(rs.fwd, limiter)
	} else {
		rs.fwd[limiter] = fEnts
	}
}

func (rs *readinessShard) update(limiter limiterState, f func() error) error {
	var toNotify deferedNotify
	defer toNotify.notify() // outside lock

	rs.lock.Lock()
	defer rs.lock.Unlock()

	defer rs.syncLimiter(limiter, &toNotify)

	return f()
}

// syncLocked may call this. wake should remove up to n items from the front of rsu.waiters and
// update rsu.waiters.
func (rsu *readinessSyncUpdate) wake(n int) {
	take := min(n, len(rsu.waiters))
	rsu.waiters = rsu.waiters[take:]
}

func (dn *deferedNotify) notify() {
	for _, cb := range *dn {
		cb.OnReady()
	}
}

func (dn *deferedNotify) add(cb ...ReadinessCallback) {
	*dn = append(*dn, cb...)
}

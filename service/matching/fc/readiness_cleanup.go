package fc

import (
	"context"
	"time"

	"go.temporal.io/server/common/util"
)

const (
	cleanupBatchSize        = 256
	cleanupScansPerCreation = 2
	cleanupBusyInterval     = 5 * time.Millisecond
	cleanupIdleInterval     = 5 * time.Second
	cleanupSweepInterval    = time.Minute
)

type cleanupState struct {
	created   uint64
	credits   int
	nextShard int
	nextSweep time.Time
}

func (r *Readiness) cleanupLoop(ctx context.Context) {
	var state cleanupState
	interval := cleanupIdleInterval
	for util.InterruptibleSleep(ctx, interval) == nil {
		interval = r.cleanupPass(&state)
	}
}

func (r *Readiness) cleanupPass(state *cleanupState) time.Duration {
	var created uint64
	var candidates int
	for i := range r.shards {
		rs := &r.shards[i]
		rs.lock.Lock()
		created += rs.created
		candidates += rs.candidates.len
		rs.lock.Unlock()
	}

	// Keep at most one traversal's worth of work, even after a large allocation burst.
	maxCredits := max(candidates, cleanupBatchSize)
	allocations := int(min(created-state.created, uint64(maxCredits)))
	state.created = created
	state.credits = min(maxCredits, state.credits+cleanupScansPerCreation*allocations)
	now := r.timeSource.Now()
	if !now.Before(state.nextSweep) {
		state.credits = max(state.credits, candidates)
		state.nextSweep = now.Add(cleanupSweepInterval)
	}

	budget := min(state.credits, cleanupBatchSize)
	scanned := 0
	for visited := 0; visited < len(r.shards) && scanned < budget; visited++ {
		rs := &r.shards[state.nextShard]
		state.nextShard = (state.nextShard + 1) % len(r.shards)
		scanned += rs.cleanup(budget - scanned)
	}
	state.credits -= scanned
	if state.credits > 0 && scanned > 0 {
		return cleanupBusyInterval
	}
	return cleanupIdleInterval
}

func (rs *readinessShard) cleanup(budget int) int {
	rs.lock.Lock()
	defer rs.lock.Unlock()

	scanned := min(budget, rs.candidates.len)
	for range scanned {
		limiter := rs.candidates.head
		rs.candidates.remove(limiter)
		if limiter.droppableLocked() {
			limiter.deleteLocked()
		} else {
			rs.candidates.pushBack(limiter)
		}
	}
	return scanned
}

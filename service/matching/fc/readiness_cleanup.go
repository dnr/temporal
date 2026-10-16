package fc

import (
	"context"
	"time"

	"go.temporal.io/server/common/util"
)

type cleanupState struct {
	created   uint64
	credits   int
	nextShard int
	nextSweep time.Time
}

func (r *Readiness) cleanupLoop(ctx context.Context) error {
	var state cleanupState
	interval := r.config.cleanup().IdleInterval
	for util.InterruptibleSleep(ctx, interval) == nil {
		interval = r.cleanupPass(&state)
	}
	return nil
}

func (r *Readiness) cleanupPass(state *cleanupState) time.Duration {
	opts := r.config.cleanup()
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
	maxCredits := max(candidates, opts.BatchSize)
	state.credits = min(state.credits, maxCredits)
	allocations := int(min(created-state.created, uint64(maxCredits)))
	state.created = created
	if opts.ScansPerCreation > 0 && allocations > (maxCredits-state.credits)/opts.ScansPerCreation {
		state.credits = maxCredits
	} else {
		state.credits = min(maxCredits, state.credits+opts.ScansPerCreation*allocations)
	}
	now := r.timeSource.Now()
	if !now.Before(state.nextSweep) {
		state.credits = max(state.credits, candidates)
		state.nextSweep = now.Add(opts.SweepInterval)
	}

	budget := min(state.credits, opts.BatchSize)
	scanned := 0
	for visited := 0; visited < len(r.shards) && scanned < budget; visited++ {
		rs := &r.shards[state.nextShard]
		state.nextShard = (state.nextShard + 1) % len(r.shards)
		scanned += rs.cleanup(budget - scanned)
	}
	state.credits -= scanned
	if state.credits > 0 && scanned > 0 {
		return opts.BusyInterval
	}
	return opts.IdleInterval
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

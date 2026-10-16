package fc

import (
	"context"
	"time"

	"go.temporal.io/server/common/util"
)

type cleanupState struct {
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
	var gcCandidates int
	for i := range r.shards {
		rs := &r.shards[i]
		rs.lock.Lock()
		created += rs.created
		rs.created = 0
		gcCandidates += rs.gcList.len
		rs.lock.Unlock()
	}

	// Keep at most one traversal's worth of work, even after a large allocation burst.
	maxCredits := max(gcCandidates, opts.BatchSize)
	state.credits = min(state.credits, maxCredits)
	allocations := int(min(created, uint64(maxCredits)))
	if opts.ScansPerCreation > 0 && allocations > (maxCredits-state.credits)/opts.ScansPerCreation {
		state.credits = maxCredits
	} else {
		state.credits = min(maxCredits, state.credits+opts.ScansPerCreation*allocations)
	}
	params := r.gcParameters()
	now := params.now
	if !now.Before(state.nextSweep) {
		state.credits = max(state.credits, gcCandidates)
		state.nextSweep = now.Add(opts.SweepInterval)
	}

	budget := min(state.credits, opts.BatchSize)
	scanned := 0
	for visited := 0; visited < len(r.shards) && scanned < budget; visited++ {
		rs := &r.shards[state.nextShard]
		state.nextShard = (state.nextShard + 1) % len(r.shards)
		scanned += rs.cleanup(budget-scanned, params)
	}
	state.credits -= scanned
	if state.credits > 0 && scanned > 0 {
		return opts.BusyInterval
	}
	return opts.IdleInterval
}

func (rs *readinessShard) cleanup(budget int, params gcParameters) int {
	rs.lock.Lock()
	defer rs.lock.Unlock()

	scanned := min(budget, rs.gcList.len)
	for range scanned {
		limiter := rs.gcList.head
		rs.gcList.remove(limiter)
		if limiter.limiterLocked().droppableLocked(params) {
			limiter.deleteLocked()
		} else {
			rs.gcList.pushBack(limiter)
		}
	}
	return scanned
}

type gcParameters struct {
	now                  time.Time
	concurrencyRetention time.Duration
}

func (r *Readiness) gcParameters() gcParameters {
	return gcParameters{
		now:                  r.timeSource.Now(),
		concurrencyRetention: r.config.concurrencyRetention(),
	}
}

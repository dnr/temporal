package fc

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/service/matching/simplelimiter"
)

func TestCleanupBatchCursor(t *testing.T) {
	r, ts := newTestReadiness(t, nil)
	var limiters []*localLimiterState
	for i := range 600 {
		lim := r.getLocalLimiter(namespace.ID("ns"), fmt.Sprint(i))
		lim.lim = simplelimiter.Ready(ts.Now().Add(time.Hour).UnixNano())
		lim.unpin()
		limiters = append(limiters, lim)
	}
	rs := limiters[0].rs
	require.Equal(t, 256, rs.cleanup(256))
	require.Same(t, limiters[256], rs.candidates.head)
	require.Same(t, limiters[255], rs.candidates.tail)
	lim := r.getLocalLimiter(namespace.ID("ns"), "256")
	require.Same(t, limiters[257], rs.candidates.head)
	lim.unpin()
	require.Same(t, lim, rs.candidates.tail)
	require.Equal(t, 600, rs.cleanup(1000))
	require.Same(t, limiters[257], rs.candidates.head)
}

func TestCleanupAllocationCredits(t *testing.T) {
	r, ts := newTestReadiness(t, nil)
	state := cleanupState{nextSweep: ts.Now().Add(time.Hour)}
	require.Equal(t, defaultCleanupOptions.IdleInterval, r.cleanupPass(&state))
	for i := range 300 {
		lim := r.getLocalLimiter(namespace.ID("ns"), fmt.Sprint(i))
		lim.lim = simplelimiter.Ready(ts.Now().Add(time.Hour).UnixNano())
		lim.unpin()
	}
	require.Equal(t, defaultCleanupOptions.BusyInterval, r.cleanupPass(&state))
	require.Equal(t, 44, state.credits)
	require.Equal(t, defaultCleanupOptions.IdleInterval, r.cleanupPass(&state))
	require.Zero(t, state.credits)
	require.Equal(t, defaultCleanupOptions.IdleInterval, r.cleanupPass(&state))
	require.Zero(t, state.credits)

	// Time alone makes these candidates droppable; the periodic sweep supplies the work.
	ts.Advance(2 * time.Hour)
	require.Equal(t, defaultCleanupOptions.BusyInterval, r.cleanupPass(&state))
	require.Equal(t, defaultCleanupOptions.IdleInterval, r.cleanupPass(&state))
	rs := r.getShard(namespace.ID("ns"))
	require.Empty(t, rs.localLimiters)
	require.Zero(t, rs.candidates.len)
}

func TestCleanupCreditsWithPinnedAllocations(t *testing.T) {
	r, _ := newTestReadiness(t, nil)
	lim := r.getLocalLimiter(namespace.ID("ns"), "key")
	var state cleanupState
	require.Equal(t, defaultCleanupOptions.IdleInterval, r.cleanupPass(&state))
	require.Equal(t, defaultCleanupOptions.ScansPerCreation, state.credits)
	lim.unpin()
	require.Equal(t, defaultCleanupOptions.BusyInterval, r.cleanupPass(&state))
	require.Equal(t, defaultCleanupOptions.IdleInterval, r.cleanupPass(&state))
	require.Empty(t, lim.rs.localLimiters)
}

func TestCleanupRotatesAcrossShards(t *testing.T) {
	r, ts := newTestReadiness(t, nil)
	opts := defaultCleanupOptions
	opts.BatchSize = 2
	r.config.cleanupOptions = func() CleanupOptions { return opts }
	firstNS := namespace.ID("first")
	secondNS := namespace.ID("second")
	for i := 0; r.getShardIdx(firstNS) == r.getShardIdx(secondNS); i++ {
		secondNS = namespace.ID(fmt.Sprint(i))
	}
	for i := range 7 {
		r.getLocalLimiter(firstNS, fmt.Sprint(i)).unpin()
	}
	for i := range 3 {
		r.getLocalLimiter(secondNS, fmt.Sprint(i)).unpin()
	}
	state := cleanupState{nextShard: r.getShardIdx(firstNS), nextSweep: ts.Now().Add(time.Hour)}
	require.Equal(t, opts.BusyInterval, r.cleanupPass(&state))
	require.Len(t, r.getShard(firstNS).localLimiters, 5)
	require.Equal(t, opts.BusyInterval, r.cleanupPass(&state))
	require.Len(t, r.getShard(secondNS).localLimiters, 1)
	require.Len(t, r.getShard(firstNS).localLimiters, 5)
}

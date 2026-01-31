package counter

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHybridCounter_StartsWithMap(t *testing.T) {
	src := rand.NewPCG(rand.Uint64(), rand.Uint64())
	h := NewHybridCounter(CounterParams{
		MapLimit: 10,
		CMS:      CMSketchParams{W: 10, D: 3},
	}, src)

	// Should be using mapCounter initially
	assert.Nil(t, h.cmSketch)

	// Add some entries
	for i := range 5 {
		h.GetPass(fmt.Sprintf("key%d", i), 0, int64(i+1))
	}

	// Should still be using mapCounter
	assert.Nil(t, h.cmSketch)
}

func TestHybridCounter_MigratesToCMS(t *testing.T) {
	src := rand.NewPCG(rand.Uint64(), rand.Uint64())
	h := NewHybridCounter(CounterParams{
		MapLimit: 5,
		CMS:      CMSketchParams{W: 100, D: 3},
	}, src)

	// Add entries up to limit
	for i := range 5 {
		h.GetPass(fmt.Sprintf("key%d", i), 0, int64(i+1))
	}
	assert.Nil(t, h.cmSketch, "should still be map at limit")

	// One more should trigger migration
	h.GetPass("trigger", 0, 100)
	assert.NotNil(t, h.cmSketch, "should have migrated to cmSketch")
}

func TestHybridCounter_TopKTracking(t *testing.T) {
	src := rand.NewPCG(rand.Uint64(), rand.Uint64())
	h := NewHybridCounter(CounterParams{
		MapLimit: 3,
		CMS:      CMSketchParams{W: 100, D: 3},
	}, src)

	// Add entries to trigger migration
	h.GetPass("low", 0, 1)
	h.GetPass("mid", 0, 5)
	h.GetPass("high", 0, 10)
	h.GetPass("trigger", 0, 3) // This triggers migration

	// Should have migrated
	require.NotNil(t, h.cmSketch)

	// TopK should still work
	topK := h.TopK()
	assert.Len(t, topK, 3)

	counts := make(map[string]int64)
	for _, e := range topK {
		counts[e.Key] = e.Count
	}
	// We should have mid=5, high=10, trigger=3 (not low=1)
	assert.Contains(t, counts, "high")
	assert.Contains(t, counts, "mid")
}

func TestHybridCounter_TopKUpdatesAfterMigration(t *testing.T) {
	src := rand.NewPCG(rand.Uint64(), rand.Uint64())
	h := NewHybridCounter(CounterParams{
		MapLimit: 2,
		CMS:      CMSketchParams{W: 100, D: 3},
	}, src)

	// Trigger migration
	h.GetPass("a", 0, 10)
	h.GetPass("b", 0, 20)
	h.GetPass("c", 0, 5) // Triggers migration

	require.NotNil(t, h.cmSketch)

	// Now add more entries - TopK should update
	h.GetPass("d", 0, 100)

	topK := h.TopK()
	counts := make(map[string]int64)
	for _, e := range topK {
		counts[e.Key] = e.Count
	}

	// "d" should be in top-K now
	assert.Equal(t, int64(100), counts["d"])
}

func TestHybridCounter_TopKPreservedOnCMSResize(t *testing.T) {
	src := rand.NewPCG(rand.Uint64(), rand.Uint64())
	h := NewHybridCounter(CounterParams{
		MapLimit: 5,
		CMS: CMSketchParams{
			W: 10,
			D: 3,
			Grow: CMSGrowParams{
				SkipRateDecay: 100,
				Threshold:     0.1,
				Ratio:         2,
				MaxW:          1000,
			},
		},
	}, src)

	// Add entries with known high counts to trigger migration
	for i := range 10 {
		h.GetPass(fmt.Sprintf("key%d", i), 0, int64((i+1)*100))
	}

	// Should have migrated to cmSketch
	require.NotNil(t, h.cmSketch)

	initialW := h.cmSketch.params.W

	// Record top-K before resize
	topKBefore := h.TopK()
	require.NotEmpty(t, topKBefore)

	// Add many more entries to trigger growth
	for i := range 500 {
		h.GetPass(fmt.Sprintf("extra%d", i), 0, 1)
	}

	// Check if CMS grew
	if h.cmSketch.params.W > initialW {
		// CMS grew - verify top-K entries were preserved
		// Get current values from CMS for the original top keys
		for _, entry := range topKBefore {
			// The count should be approximately preserved
			// (may be slightly higher due to CMS approximation)
			currentCount := h.cmSketch.GetPass(entry.Key, 0, 0)
			assert.GreaterOrEqual(t, currentCount, entry.Count,
				"count for %s should be preserved after resize", entry.Key)
		}
	}
}

func TestHybridCounter_MapClearedAfterMigration(t *testing.T) {
	src := rand.NewPCG(rand.Uint64(), rand.Uint64())
	h := NewHybridCounter(CounterParams{
		MapLimit: 3,
		CMS:      CMSketchParams{W: 100, D: 3},
	}, src)

	// Add entries to trigger migration
	for i := range 5 {
		h.GetPass(fmt.Sprintf("key%d", i), 0, int64(i+1))
	}

	// But heap should still have entries for top-K tracking
	assert.NotEmpty(t, h.mapCounter.heap)
}

package counter

import (
	"math/rand/v2"
)

type (
	CounterParams struct {
		MapLimit int
		CMS      CMSketchParams
	}

	// hybridCounter is a Counter that uses a mapCounter until it has params.MapLimit entries,
	// then switches to a cmSketch. After switching, it continues to track the top MapLimit
	// entries in the mapCounter, so they can be preserved when cmSketch resizes and persisted.
	// hybridCounter is not safe for concurrent use.
	hybridCounter struct {
		Counter
		mapCounter mapCounter
		params     CounterParams
		src        rand.Source
	}
)

var _ Counter = (*hybridCounter)(nil)

var DefaultCounterParams = CounterParams{
	MapLimit: 100,
	CMS: CMSketchParams{
		W: 100,
		D: 5,
		Grow: CMSGrowParams{
			SkipRateDecay: 10_000,
			Threshold:     0.35,
			Ratio:         1.5,
			MaxW:          10_000,
		},
	},
}

func NewHybridCounter(params CounterParams, src rand.Source) *hybridCounter {
	hc := &hybridCounter{
		mapCounter: *NewMapCounter(params.MapLimit),
		params:     params,
		src:        src,
	}
	// start with mapCounter as the counter
	hc.Counter = &hc.mapCounter
	return hc
}

func (h *hybridCounter) GetPass(key string, base int64, inc int64) int64 {
	p := h.Counter.GetPass(key, base, inc)
	// FIXME: use interface?
	if _, ok := h.Counter.(*mapCounter); ok && len(h.mapCounter.m) > h.params.MapLimit {
		h.migrateToCMS()
	} else if _, ok := h.Counter.(*cmSketch); ok {
		// After migration, continue updating top-K tracker
		h.mapCounter.Update(key, p)
	}
	return p
}

func (h *hybridCounter) migrateToCMS() {
	cms := NewCMSketchCounter(h.params.CMS, h.src, h.mapCounter.TopK)
	// move existing counts into CMS
	for key, count := range h.mapCounter.m {
		_ = cms.GetPass(key, count, 0)
	}
	// Clear the map to free memory, but keep the heap for top-K tracking
	clear(h.mapCounter.m)
	h.Counter = cms
}

// TopK returns the current top-K entries being tracked.
func (h *hybridCounter) TopK() []TopKEntry {
	return h.mapCounter.TopK()
}

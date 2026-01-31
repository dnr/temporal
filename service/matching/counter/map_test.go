package counter

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapCounter_Basic(t *testing.T) {
	m := NewMapCounter()

	assert.Equal(t, int64(1), m.GetPass("a", 0, 1))
	assert.Equal(t, int64(3), m.GetPass("a", 0, 2))
	assert.Equal(t, int64(10), m.GetPass("b", 0, 10))
	assert.Equal(t, 2, m.EstimateDistinctKeys())
}

func TestMapCounter_TopK(t *testing.T) {
	m := NewMapCounterWithLimit(3)

	// Add 5 entries with different counts
	m.GetPass("low1", 0, 1)
	m.GetPass("low2", 0, 2)
	m.GetPass("mid", 0, 5)
	m.GetPass("high1", 0, 10)
	m.GetPass("high2", 0, 8)

	topK := m.TopK()
	assert.Len(t, topK, 3)

	// Verify we have the top 3 (mid=5, high2=8, high1=10)
	counts := make(map[string]int64)
	for _, e := range topK {
		counts[e.Key] = e.Count
	}
	assert.Equal(t, int64(5), counts["mid"])
	assert.Equal(t, int64(8), counts["high2"])
	assert.Equal(t, int64(10), counts["high1"])
	assert.NotContains(t, counts, "low1")
	assert.NotContains(t, counts, "low2")
}

func TestMapCounter_TopK_Update(t *testing.T) {
	m := NewMapCounterWithLimit(2)

	// Start with two entries
	m.GetPass("a", 0, 1)
	m.GetPass("b", 0, 2)

	topK := m.TopK()
	assert.Len(t, topK, 2)

	// Update "a" to have the highest count
	m.GetPass("a", 0, 100)

	topK = m.TopK()
	counts := make(map[string]int64)
	for _, e := range topK {
		counts[e.Key] = e.Count
	}
	assert.Equal(t, int64(101), counts["a"]) // 1 + 100
	assert.Equal(t, int64(2), counts["b"])
}

func TestMapCounter_TopK_Eviction(t *testing.T) {
	m := NewMapCounterWithLimit(2)

	m.GetPass("a", 0, 10)
	m.GetPass("b", 0, 20)

	// "c" with count 5 should not evict anything
	m.GetPass("c", 0, 5)
	topK := m.TopK()
	counts := make(map[string]int64)
	for _, e := range topK {
		counts[e.Key] = e.Count
	}
	assert.NotContains(t, counts, "c")

	// "d" with count 15 should evict "a"
	m.GetPass("d", 0, 15)
	topK = m.TopK()
	counts = make(map[string]int64)
	for _, e := range topK {
		counts[e.Key] = e.Count
	}
	assert.NotContains(t, counts, "a")
	assert.Equal(t, int64(15), counts["d"])
	assert.Equal(t, int64(20), counts["b"])
}

func TestMapCounter_Update(t *testing.T) {
	m := NewMapCounterWithLimit(2)

	// Use Update directly (for post-migration use case)
	m.Update("a", 100)
	m.Update("b", 50)

	topK := m.TopK()
	assert.Len(t, topK, 2)

	counts := make(map[string]int64)
	for _, e := range topK {
		counts[e.Key] = e.Count
	}
	assert.Equal(t, int64(100), counts["a"])
	assert.Equal(t, int64(50), counts["b"])

	// Map should be empty (Update doesn't modify the map)
	assert.Empty(t, m.m)
}

func TestMapCounter_NoLimit(t *testing.T) {
	m := NewMapCounter()

	m.GetPass("a", 0, 1)
	m.GetPass("b", 0, 2)

	// TopK should return nil when no limit
	assert.Nil(t, m.TopK())
}

func TestMapCounter_TopK_ManyEntries(t *testing.T) {
	m := NewMapCounterWithLimit(10)

	// Add 100 entries with counts 1-100
	for i := 1; i <= 100; i++ {
		m.GetPass(fmt.Sprintf("key%d", i), 0, int64(i))
	}

	topK := m.TopK()
	assert.Len(t, topK, 10)

	// Verify we have keys 91-100 (the highest counts)
	counts := make(map[string]int64)
	for _, e := range topK {
		counts[e.Key] = e.Count
	}
	for i := 91; i <= 100; i++ {
		assert.Equal(t, int64(i), counts[fmt.Sprintf("key%d", i)], "key%d should be in top-K", i)
	}
}

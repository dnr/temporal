package matching

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	enumspb "go.temporal.io/api/enums/v1"
)

// Using a fixed UUID so tests are deterministic. Must be valid UUID format for makeKey.
const testNsID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"

func TestPartitionCache_BasicPutLookup(t *testing.T) {
	t.Parallel()
	c := newPartitionCache()
	c.Start()
	defer c.Stop()

	key := c.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	pc := PartitionCounts{Read: 4, Write: 4}

	c.put(key, pc)
	got := c.lookup(key)
	assert.Equal(t, pc, got)
}

func TestPartitionCache_Miss(t *testing.T) {
	t.Parallel()
	c := newPartitionCache()
	c.Start()
	defer c.Stop()

	key := c.makeKey(testNsID, "nonexistent", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	got := c.lookup(key)
	assert.Equal(t, PartitionCounts{}, got)
}

func TestPartitionCache_InvalidRemoves(t *testing.T) {
	t.Parallel()
	c := newPartitionCache()
	c.Start()
	defer c.Stop()

	key := c.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	c.put(key, PartitionCounts{Read: 4, Write: 4})

	// Putting invalid (zero) counts should remove the entry
	c.put(key, PartitionCounts{Read: 0, Write: 0})
	got := c.lookup(key)
	assert.Equal(t, PartitionCounts{}, got)
}

func TestPartitionCache_PromotionFromPrev(t *testing.T) {
	t.Parallel()
	c := newPartitionCache()
	c.Start()
	defer c.Stop()

	key := c.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	pc := PartitionCounts{Read: 8, Write: 8}
	c.put(key, pc)

	// Rotate the shard — entry moves to prev
	shard := c.shardFromKey(key)
	c.shards[shard].rotate()

	// Lookup should still find it (promotes from prev to active)
	got := c.lookup(key)
	assert.Equal(t, pc, got)

	// After promotion, rotate again — it should still be in active
	c.shards[shard].rotate()
	got = c.lookup(key)
	assert.Equal(t, pc, got)
}

func TestPartitionCache_ExpiryAfterTwoRotations(t *testing.T) {
	t.Parallel()
	c := newPartitionCache()
	c.Start()
	defer c.Stop()

	key := c.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	c.put(key, PartitionCounts{Read: 4, Write: 4})

	shard := c.shardFromKey(key)
	// Rotate twice without any lookup — entry should be gone
	c.shards[shard].rotate()
	c.shards[shard].rotate()

	got := c.lookup(key)
	assert.Equal(t, PartitionCounts{}, got)
}

func TestPartitionCache_MakeKey_DifferentInputs(t *testing.T) {
	t.Parallel()
	c := newPartitionCache()

	key1 := c.makeKey(testNsID, "tq-a", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	key2 := c.makeKey(testNsID, "tq-b", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	key3 := c.makeKey(testNsID, "tq-a", enumspb.TASK_QUEUE_TYPE_ACTIVITY)
	key4 := c.makeKey("a7b3c10d-58cc-4372-a567-0e02b2c3d479", "tq-a", enumspb.TASK_QUEUE_TYPE_WORKFLOW)

	// All keys should be different
	assert.NotEqual(t, key1, key2)
	assert.NotEqual(t, key1, key3)
	assert.NotEqual(t, key1, key4)

	// Same inputs → same key
	assert.Equal(t, key1, c.makeKey(testNsID, "tq-a", enumspb.TASK_QUEUE_TYPE_WORKFLOW))
}

func TestPartitionCache_MakeKey_InvalidUUIDFallback(t *testing.T) {
	t.Parallel()
	c := newPartitionCache()

	// Invalid UUID should still produce a usable key (fallback path)
	key := c.makeKey("not-a-uuid", "tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	require.NotEmpty(t, key)

	// Should be different from a valid UUID key
	key2 := c.makeKey(testNsID, "tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	assert.NotEqual(t, key, key2)
}

func TestPartitionCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	c := newPartitionCache()
	c.Start()
	defer c.Stop()

	const goroutines = 20
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := c.makeKey(testNsID, "tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
				pc := PartitionCounts{Read: int32(g + 1), Write: int32(g + 1)}
				c.put(key, pc)
				c.lookup(key)
			}
		}(g)
	}
	wg.Wait()
	// No panic or race condition = pass
}

func TestPartitionCache_OverwriteValue(t *testing.T) {
	t.Parallel()
	c := newPartitionCache()
	c.Start()
	defer c.Stop()

	key := c.makeKey(testNsID, "my-tq", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	c.put(key, PartitionCounts{Read: 4, Write: 4})
	c.put(key, PartitionCounts{Read: 8, Write: 8})

	got := c.lookup(key)
	assert.Equal(t, PartitionCounts{Read: 8, Write: 8}, got)
}

func TestPartitionCache_MultipleKeys(t *testing.T) {
	t.Parallel()
	c := newPartitionCache()
	c.Start()
	defer c.Stop()

	key1 := c.makeKey(testNsID, "tq-1", enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	key2 := c.makeKey(testNsID, "tq-2", enumspb.TASK_QUEUE_TYPE_ACTIVITY)

	c.put(key1, PartitionCounts{Read: 4, Write: 4})
	c.put(key2, PartitionCounts{Read: 8, Write: 6})

	assert.Equal(t, PartitionCounts{Read: 4, Write: 4}, c.lookup(key1))
	assert.Equal(t, PartitionCounts{Read: 8, Write: 6}, c.lookup(key2))
}

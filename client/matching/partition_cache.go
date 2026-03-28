package matching

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/server/common/goro"
)

// should be power of 2
const partitionCacheShards = 8

type partitionCache struct {
	shards [partitionCacheShards]partitionCacheShard
	rotate *goro.Handle
}

type partitionCacheShard struct {
	lock   sync.RWMutex
	active map[string]PartitionCounts
	prev   map[string]PartitionCounts
	_      [64 - 24 - 8 - 8]byte // eliminate false sharing
}

func newPartitionCache() *partitionCache {
	return &partitionCache{}
}

func (c *partitionCache) Start() {
	for i := range c.shards {
		c.shards[i].rotate()
	}
	c.rotate = goro.NewHandle(context.Background()).Go(func(ctx context.Context) error {
		t := time.NewTicker(time.Hour / partitionCacheShards)
		for i := 0; ; i = (i + 1) % partitionCacheShards {
			select {
			case <-t.C:
				c.shards[i].rotate()
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})
}

func (c *partitionCache) Stop() {
	c.rotate.Cancel()
}

func (*partitionCache) makeKey(
	nsid, tqname string, tqtype enumspb.TaskQueueType,
) string {
	// note we don't need delimiters to make unambiguous keys: nsid is always the same length,
	// the last byte is tqtype, and everything in between is the name.
	nsidBytes, err := uuid.Parse(nsid)
	if err != nil {
		// this shouldn't fail, but use the string form as a backup, append a 0xff to differentiate
		return nsid + tqname + string([]byte{byte(tqtype), 0xff})
	}
	return string(nsidBytes[:]) + tqname + string([]byte{byte(tqtype)})
}

func (*partitionCache) shardFromKey(key string) int {
	// mix a few bits to pick a shard
	l := len(key)
	shard := int(key[14] ^ key[l-2] ^ key[l-1])
	return shard % partitionCacheShards
}

func (c *partitionCache) lookup(key string) PartitionCounts {
	return c.shards[c.shardFromKey(key)].lookup(key)
}

func (c *partitionCache) put(key string, pc PartitionCounts) {
	c.shards[c.shardFromKey(key)].put(key, pc)
}

func (s *partitionCacheShard) lookup(key string) PartitionCounts {
	s.lock.RLock()
	if pc, ok := s.active[key]; ok {
		s.lock.RUnlock()
		return pc
	} else if pc, ok := s.prev[key]; ok {
		s.lock.RUnlock()
		s.put(key, pc) // promote to active
		return pc
	}
	s.lock.RUnlock()
	return PartitionCounts{}
}

func (s *partitionCacheShard) put(key string, pc PartitionCounts) {
	s.lock.Lock()
	if pc.Valid() {
		s.active[key] = pc
	} else {
		delete(s.active, key)
	}
	delete(s.prev, key)
	s.lock.Unlock()
}

func (s *partitionCacheShard) rotate() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.prev = s.active
	s.active = make(map[string]PartitionCounts)
}

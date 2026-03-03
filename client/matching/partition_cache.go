package matching

import (
	"sync"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
)

const partitionCacheShards = 8

type partitionCache struct {
	shards [partitionCacheShards]partitionCacheShard
}

type partitionCacheShard struct {
	lock   sync.RWMutex
	active map[string]partitionCounts
	prev   map[string]partitionCounts
	_      [64 - 24 - 8 - 8]byte // eliminate false sharing
}

func newPartitionCache() *partitionCache {
	c := &partitionCache{}
	return c
}

func (c *partitionCache) Start() {
	go func() {
		idx := 0
		for range time.NewTicker(time.Hour / partitionCacheShards).C {
			c.shards[idx].rotate()
			idx = (idx + 1) & (1<<partitionCacheShards - 1)
		}
	}()
}

func (c *partitionCache) makeKey(
	nsid, tqname string, tqtype enumspb.TaskQueueType,
) (string, int, error) {
	// note we don't need delimiters to make unambiguous keys: nsid is always the same length,
	// the last byte is tqtype, and everything in between is the name.
	nsidBytes, err := uuid.Parse(nsid)
	if err != nil {
		return "", 0, err
	}
	key := string(nsidBytes[:]) + tqname + string([]byte{byte(tqtype)})
	// mix a few bits to pick a shard
	l := len(key)
	shard := int(key[14] ^ key[l-2] ^ key[l-1])
	shard = shard & (1<<partitionCacheShards - 1)
	return key, shard, nil
}

func (c *partitionCache) lookup(
	nsid, tqname string, tqtype enumspb.TaskQueueType,
) (partitionCounts, bool) {
	key, shard, err := c.makeKey(nsid, tqname, tqtype)
	if err != nil {
		return partitionCounts{}, false
	}
	return c.shards[shard].lookup(key)
}

func (c *partitionCache) put(
	nsid, tqname string, tqtype enumspb.TaskQueueType,
	pc partitionCounts,
) {
	key, shard, err := c.makeKey(nsid, tqname, tqtype)
	if err != nil {
		return
	}
	c.shards[shard&(1<<partitionCacheShards-1)].put(key, pc)
}

func (s *partitionCacheShard) lookup(key string) (partitionCounts, bool) {
	s.lock.RLock()
	if pc, ok := s.active[key]; ok {
		s.lock.RUnlock()
		return pc, true
	} else if pc, ok := s.prev[key]; ok {
		s.lock.RUnlock()
		s.put(key, pc) // promote to active
		return pc, true
	}
	s.lock.RUnlock()
	return partitionCounts{}, false
}

func (s *partitionCacheShard) put(key string, pc partitionCounts) {
	s.lock.Lock()
	s.active[key] = pc
	delete(s.prev, key)
	s.lock.Unlock()
}

func (s *partitionCacheShard) rotate() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.prev = s.active
	s.active = make(map[string]partitionCounts)
}

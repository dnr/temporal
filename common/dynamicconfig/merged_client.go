package dynamicconfig

import (
	"slices"
	"sync"
	"weak"
)

type (
	MergedClient struct {
		NotifyingClientImpl
		clients []Client
		cancels []func()

		cacheLock sync.RWMutex
		cache     map[Key]mergedCacheEntry
	}

	// mergedCacheEntry remembers which source slices a merged slice was built from, so that
	// GetValue can keep returning the same merged slice until a sub-client returns a
	// different slice for the key. See the comment on Client.GetValue: callers cache
	// converted values using weak pointers into the returned slice, so building a new slice
	// on each call would defeat that cache.
	mergedCacheEntry struct {
		sources []sourceSliceID
		merged  []ConstrainedValue
	}

	// sourceSliceID identifies a particular []ConstrainedValue returned by a sub-client.
	// The first element is held weakly so the cache doesn't keep a sub-client's replaced
	// values alive. A new slice at a reused address gets a distinct weak.Pointer, so stale
	// identities can't produce false matches.
	sourceSliceID struct {
		first  weak.Pointer[ConstrainedValue]
		length int
	}
)

// NewMergedClient returns a client that merges configs from mulitple underlying clients.
// Clients should be ordered from higher priority to lower priority.
// Stop() should be called on the MergedClient before tearing down the server to remove
// subscriptions on the sub-clients.
func NewMergedClient(clients []Client) *MergedClient {
	return &MergedClient{
		NotifyingClientImpl: NewNotifyingClientImpl(),
		clients:             clients,
		cache:               make(map[Key]mergedCacheEntry),
	}
}

func (m *MergedClient) Start() {
	// subscribe to all clients that are notifying
	for _, c := range m.clients {
		if nc, ok := c.(NotifyingClient); ok {
			cancel := nc.Subscribe(m.changed)
			m.cancels = append(m.cancels, cancel)
		}
	}
}

func (m *MergedClient) Stop() {
	for _, cancel := range m.cancels {
		cancel()
	}
}

func (m *MergedClient) changed(changed map[Key][]ConstrainedValue) {
	combinedChanges := make(map[Key][]ConstrainedValue, len(changed))
	// just re-evaluate for all changed keys. this could maybe be optimized.
	for k := range changed {
		combinedChanges[k] = m.GetValue(k)
	}
	m.PublishUpdates(combinedChanges)
}

func (m *MergedClient) GetValue(key Key) []ConstrainedValue {
	sources := make([][]ConstrainedValue, len(m.clients))
	for i, c := range m.clients {
		sources[i] = c.GetValue(key)
	}
	ids := makeSourceSliceIDs(sources)

	m.cacheLock.RLock()
	entry, ok := m.cache[key]
	m.cacheLock.RUnlock()
	if ok && slices.Equal(entry.sources, ids) {
		return entry.merged
	}

	var merged []ConstrainedValue
	for _, s := range sources {
		// this uses the fact that Collection uses the first ConstrainedValue if multiple match
		merged = append(merged, s...)
	}

	m.cacheLock.Lock()
	defer m.cacheLock.Unlock()
	// Another goroutine may have rebuilt from the same sources concurrently. Prefer the
	// entry that's already cached so callers keep seeing a single slice per value.
	if entry, ok := m.cache[key]; ok && slices.Equal(entry.sources, ids) {
		return entry.merged
	}
	m.cache[key] = mergedCacheEntry{sources: ids, merged: merged}
	return merged
}

func makeSourceSliceIDs(sources [][]ConstrainedValue) []sourceSliceID {
	ids := make([]sourceSliceID, len(sources))
	for i, s := range sources {
		if len(s) > 0 {
			ids[i] = sourceSliceID{first: weak.Make(&s[0]), length: len(s)}
		}
		// empty slices all get the zero sourceSliceID: they contribute nothing to the
		// merge, so they're interchangeable.
	}
	return ids
}

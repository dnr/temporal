package counter

import "container/heap"

// mapCounter is a Counter that stores counts in a map.
// It also maintains a min-heap to efficiently track the top-K entries.
// mapCounter is not safe for concurrent use.
type mapCounter struct {
	m     map[string]int64
	limit int // top-K limit (0 = no heap tracking)

	// Min-heap tracking for top-K (only used when limit > 0)
	heap   topKHeap
	inHeap map[string]*heapEntry
}

// heapEntry represents an entry in the top-K min-heap.
type heapEntry struct {
	key   string
	count int64
	index int // position in heap slice
}

var _ Counter = (*mapCounter)(nil)

// NewMapCounter creates a mapCounter that also tracks the top K entries.
func NewMapCounter(limit int) *mapCounter {
	return &mapCounter{
		m:      make(map[string]int64),
		limit:  limit,
		heap:   make(topKHeap, 0, limit),
		inHeap: make(map[string]*heapEntry),
	}
}

func (m *mapCounter) GetPass(key string, base, inc int64) int64 {
	c := max(base, m.m[key]+inc)
	m.m[key] = c
	if m.limit > 0 {
		m.updateHeap(key, c)
	}
	return c
}

func (m *mapCounter) EstimateDistinctKeys() int {
	return len(m.m)
}

// Update updates the top-K tracking without modifying the underlying map.
// This is useful when the actual counts are stored elsewhere (e.g., cmSketch).
func (m *mapCounter) Update(key string, count int64) {
	if m.limit > 0 {
		m.updateHeap(key, count)
	}
}

// TopK returns the top-K entries by count.
func (m *mapCounter) TopK() []TopKEntry {
	if m.limit == 0 {
		return nil
	}
	result := make([]TopKEntry, len(m.heap))
	for i, e := range m.heap {
		result[i] = TopKEntry{Key: e.key, Count: e.count}
	}
	return result
}

func (m *mapCounter) updateHeap(key string, count int64) {
	if entry, ok := m.inHeap[key]; ok {
		// Key already in heap - update count and fix heap
		if count > entry.count {
			entry.count = count
			// Count increased, might need to move down (away from min)
			heap.Fix(&m.heap, entry.index)
		}
		return
	}

	if len(m.heap) < m.limit {
		// Heap not full - add entry
		entry := &heapEntry{key: key, count: count}
		heap.Push(&m.heap, entry)
		m.inHeap[key] = entry
		return
	}

	// Heap is full - only add if count > min
	if count > m.heap[0].count {
		// Evict the minimum
		evicted := heap.Pop(&m.heap).(*heapEntry)
		delete(m.inHeap, evicted.key)

		// Add new entry
		entry := &heapEntry{key: key, count: count}
		heap.Push(&m.heap, entry)
		m.inHeap[key] = entry
	}
}

// topKHeap implements heap.Interface as a min-heap (smallest count at root).
type topKHeap []*heapEntry

func (h topKHeap) Len() int           { return len(h) }
func (h topKHeap) Less(i, j int) bool { return h[i].count < h[j].count }
func (h topKHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *topKHeap) Push(x any) {
	entry := x.(*heapEntry)
	entry.index = len(*h)
	*h = append(*h, entry)
}

func (h *topKHeap) Pop() any {
	old := *h
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil // avoid memory leak
	entry.index = -1
	*h = old[0 : n-1]
	return entry
}

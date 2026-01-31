package counter

import "container/heap"

// mapCounter is a Counter that stores counts in a map.
// It also maintains a min-heap to efficiently track the top-K entries.
// mapCounter is not safe for concurrent use.
type mapCounter struct {
	limit  int
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
		limit:  limit,
		heap:   make(topKHeap, 0, limit),
		inHeap: make(map[string]*heapEntry),
	}
}

func (m *mapCounter) GetPass(key string, base, inc int64) int64 {
	c, _ := m.getPassWithOverflow(key, base, inc)
	return c
}

func (m *mapCounter) getPassWithOverflow(key string, base, inc int64) (int64, bool) {
	var prev int64
	if entry, ok := m.inHeap[key]; ok {
		prev = entry.count
	}
	c := max(base, prev+inc)
	overflow := m.updateHeap(key, c)
	return c, overflow
}

func (m *mapCounter) EstimateDistinctKeys() int {
	return len(m.inHeap)
}

// TopK returns the top-K entries by count.
func (m *mapCounter) TopK() []TopKEntry {
	result := make([]TopKEntry, len(m.heap))
	for i, e := range m.heap {
		result[i] = TopKEntry{Key: e.key, Count: e.count}
	}
	return result
}

func (m *mapCounter) updateHeap(key string, count int64) bool {
	if entry, ok := m.inHeap[key]; ok {
		// already in heap - update count and fix
		entry.count = count
		heap.Fix(&m.heap, entry.index)
		return false
	}

	if len(m.heap) < m.limit {
		// Heap not full - add entry
		entry := &heapEntry{key: key, count: count}
		heap.Push(&m.heap, entry)
		m.inHeap[key] = entry
		return false
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
	return true
}

// topKHeap implements heap.Interface as a min-heap (smallest count at root).
type topKHeap []*heapEntry

func (h topKHeap) Len() int           { return len(h) }
func (h topKHeap) Less(i, j int) bool { return h[i].count < h[j].count }
func (h topKHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index, h[j].index = i, j
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
	old[n-1].key = ""
	entry.index = -1
	*h = old[0 : n-1]
	return entry
}

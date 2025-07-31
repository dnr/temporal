package fairsim

import (
	"cmp"
	"container/heap"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"

	"go.temporal.io/server/service/matching/counter"
)

const stride = 10000

type (
	taskGenFunc func() (task, bool)

	task struct {
		pri     int
		fkey    string
		fweight float32
		pass    int64
		index   int64
	}

	state struct {
		rnd            *rand.Rand
		counterFactory func() counter.Counter
		partitions     []partitionState
	}

	latencyStats struct {
		byKey   map[string][]int64 // latencies by fairness key
		overall []int64            // all latencies
	}

	partitionState struct {
		perPri map[int]perPriState
		heap   taskHeap
	}

	taskHeap []*task

	perPriState struct {
		c counter.Counter
	}
)

func RunTool(args []string) error {
	params := counter.DefaultCounterParams
	// TODO: be able to load params from a json file specified by a flag

	// TODO: be able to specify seed from a flag
	src := rand.NewPCG(rand.Uint64(), rand.Uint64())
	rnd := rand.New(src)

	counterFactory := func() counter.Counter {
		return counter.NewHybridCounter(params, src)
	}

	// TODO: set number of partitions from command line
	const partitions = 4

	state := state{
		rnd:            rnd,
		counterFactory: counterFactory,
		partitions:     make([]partitionState, partitions),
	}

	var stats latencyStats
	stats.byKey = make(map[string][]int64)

	var nextIndex, dispatchCounter int64

	const tasks = 10000
	const defaultPriority = 3

	var gen taskGenFunc

	if true {
		// TODO: add flags to override these
		const zipf_s = 2.0
		const zipf_v = 2.0
		const keys = 100

		zipf := rand.NewZipf(rnd, zipf_s, zipf_v, keys-1)

		tasksLeft := tasks
		gen = func() (task, bool) {
			tasksLeft--
			if tasksLeft < 0 {
				return task{}, false
			}
			fkey := fmt.Sprintf("fkey%d", zipf.Uint64())
			pri := min(5, max(1, defaultPriority+int(math.Round(rnd.NormFloat64()*0.5))))
			return task{pri: pri, fkey: fkey}, true
		}
	} else {
		// TODO: option to read keys/weights from file
	}

	// add all tasks
	for t, ok := gen(); ok; t, ok = gen() {
		t.pri = cmp.Or(t.pri, defaultPriority)
		t.fweight = cmp.Or(t.fweight, 1.0)
		t.index = nextIndex
		nextIndex++
		state.addTask(t)
	}

	// pop all tasks and print
	for t, partition := state.popTask(rnd); t != nil; t, partition = state.popTask(rnd) {
		// Calculate logical latency
		latency := dispatchCounter - t.index
		dispatchCounter++

		// Track latency statistics
		stats.byKey[t.fkey] = append(stats.byKey[t.fkey], latency)
		stats.overall = append(stats.overall, latency)

		fmt.Printf("task idx-dispatch: %d-%d = %d, pri: %d, fkey: %q, fweight: %g, partition: %d\n", t.index, dispatchCounter, latency, t.pri, t.fkey, t.fweight, partition)
	}

	// Print final latency stats
	stats.print()

	return nil
}

func (stats *latencyStats) print() {
	fmt.Printf("\n=== Latency Statistics ===\n")

	// Overall stats
	if len(stats.overall) > 0 {
		sort.Slice(stats.overall, func(i, j int) bool { return stats.overall[i] < stats.overall[j] })
		mean := float64(sum(stats.overall)) / float64(len(stats.overall))
		median := stats.overall[len(stats.overall)/2]
		p95 := stats.overall[int(float64(len(stats.overall))*0.95)]

		fmt.Printf("Overall: mean=%.2f, median=%d, p95=%d, min=%d, max=%d\n",
			mean, median, p95, stats.overall[0], stats.overall[len(stats.overall)-1])
	}

	// Per-key stats (sorted by mean latency)
	type keyStats struct {
		key    string
		mean   float64
		median int64
		count  int
	}

	var keyStatsList []keyStats
	for key, latencies := range stats.byKey {
		if len(latencies) == 0 {
			continue
		}
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		mean := float64(sum(latencies)) / float64(len(latencies))
		median := latencies[len(latencies)/2]
		keyStatsList = append(keyStatsList, keyStats{
			key:    key,
			mean:   mean,
			median: median,
			count:  len(latencies),
		})
	}

	sort.Slice(keyStatsList, func(i, j int) bool { return keyStatsList[i].mean < keyStatsList[j].mean })

	fmt.Printf("\nPer-key stats (sorted by mean latency):\n")
	for _, ks := range keyStatsList {
		fmt.Printf("  %s: mean=%.2f, median=%d, count=%d\n", ks.key, ks.mean, ks.median, ks.count)
	}
}

func sum(slice []int64) int64 {
	var total int64
	for _, v := range slice {
		total += v
	}
	return total
}

// Implement heap.Interface for taskHeap
func (h taskHeap) Len() int {
	return len(h)
}

func (h taskHeap) Less(i, j int) bool {
	// Order by (pri, pass, id)
	if h[i].pri != h[j].pri {
		return h[i].pri < h[j].pri
	}
	if h[i].pass != h[j].pass {
		return h[i].pass < h[j].pass
	}
	return h[i].index < h[j].index
}

func (h taskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *taskHeap) Push(x interface{}) {
	*h = append(*h, x.(*task))
}

func (h *taskHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

// addTask adds a task to the state, picking a random partition and pass using the counter
func (s *state) addTask(t task) {
	// Pick a random partition
	partition := &s.partitions[s.rnd.IntN(len(s.partitions))]

	if partition.perPri == nil {
		partition.perPri = make(map[int]perPriState)
	}

	priState, exists := partition.perPri[t.pri]
	if !exists {
		priState = perPriState{c: s.counterFactory()}
		partition.perPri[t.pri] = priState
	}

	// Pick pass using counter like fairTaskWriter does
	// Baseline is 0 (current ack level assumed to be zero)
	pass := priState.c.GetPass(t.fkey, 0, int64(float32(stride)/t.fweight))
	t.pass = pass

	heap.Push(&partition.heap, &t)
}

// popTask returns the task with minimum (pri, pass, id) from a random partition
func (s *state) popTask(rnd *rand.Rand) (*task, int) {
	// Pick a random partition and try to pop from it
	// If it's empty, try other partitions in order
	startIdx := rnd.IntN(len(s.partitions))
	for i := range len(s.partitions) {
		idx := (startIdx + i) % len(s.partitions)
		partition := &s.partitions[idx]

		if partition.heap.Len() > 0 {
			t := heap.Pop(&partition.heap).(*task)
			return t, idx
		}
	}

	return nil, -1
}

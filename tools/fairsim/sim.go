package fairsim

import (
	"cmp"
	"container/heap"
	"fmt"
	"math/rand/v2"
	"slices"

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

	unfairCounter struct{}
)

func RunTool(args []string) error {
	params := counter.DefaultCounterParams
	// TODO: be able to load params from a json file specified by a flag

	// TODO: be able to specify seed from a flag
	src := rand.NewPCG(rand.Uint64(), rand.Uint64())
	rnd := rand.New(src)

	counterFactory := func() counter.Counter { return unfairCounter{} }
	// TODO: allow turning on/off fairness by flag
	if true {
		counterFactory = func() counter.Counter { return counter.NewHybridCounter(params, src) }
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
			var pri int
			// pri = min(5, max(1, defaultPriority+int(math.Round(rnd.NormFloat64()*0.5))))
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

		fmt.Printf("task idx-dsp:%6d-%6d = %6d  pri:%2d  fkey:%10q  fweight:%3g  part:%2d\n", t.index, dispatchCounter, latency, t.pri, t.fkey, t.fweight, partition)
	}

	// Print final latency stats
	stats.print()

	return nil
}

func (stats *latencyStats) print() {
	fmt.Printf("\n=== Latency Statistics ===\n")

	// Overall stats
	if len(stats.overall) > 0 {
		slices.Sort(stats.overall)
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
		slices.Sort(latencies)
		mean := float64(sum(latencies)) / float64(len(latencies))
		median := latencies[len(latencies)/2]
		keyStatsList = append(keyStatsList, keyStats{
			key:    key,
			mean:   mean,
			median: median,
			count:  len(latencies),
		})
	}

	slices.SortFunc(keyStatsList, func(a, b keyStats) int { return cmp.Compare(a.mean, b.mean) })

	fmt.Printf("\nPer-key stats (sorted by mean latency):\n")
	for _, ks := range keyStatsList {
		fmt.Printf("  %s: mean=%.2f, median=%d, count=%d\n", ks.key, ks.mean, ks.median, ks.count)
	}

	// Fairness metrics (percentile of percentiles)
	fmt.Printf("\nFairness metrics (percentile of per-key percentiles):\n")
	ps := []float64{20, 50, 80, 90, 95}
	fmt.Printf("         @%-4.0f  @%-4.0f  @%-4.0f  @%-4.0f  @%-4.0f\n",
		ps[0], ps[1], ps[2], ps[3], ps[4])
	for _, p := range ps {
		pofps := stats.percentileOfPercentiles(p, ps)
		fmt.Printf("  p%2.0fs: %5.0f  %5.0f  %5.0f  %5.0f  %5.0f\n",
			p, pofps[0], pofps[1], pofps[2], pofps[3], pofps[4])
	}
}

func sum(slice []int64) int64 {
	var total int64
	for _, v := range slice {
		total += v
	}
	return total
}

// percentileOfPercentiles calculates the cross-key percentile of per-key percentiles
// keyPercentile: percentile to calculate within each key (e.g., 95 for p95)
// crossPercentile: percentile to calculate across keys (e.g., 90 for p90)
// Returns the crossPercentile'th percentile of the keyPercentile values from each key
func (stats *latencyStats) percentileOfPercentiles(keyPercentile float64, crossPercentile []float64) []float64 {
	var keyPercentiles []float64

	for _, latencies := range stats.byKey {
		if len(latencies) == 0 {
			continue
		}

		// Sort latencies for this key
		sorted := make([]int64, len(latencies))
		copy(sorted, latencies)
		slices.Sort(sorted)

		// Calculate the keyPercentile for this key
		idx := int(keyPercentile / 100.0 * float64(len(sorted)-1))
		keyPercentiles = append(keyPercentiles, float64(sorted[idx]))
	}

	if len(keyPercentiles) == 0 {
		return nil
	}

	// Sort the per-key percentiles
	slices.Sort(keyPercentiles)

	out := make([]float64, len(crossPercentile))
	for i, p := range crossPercentile {
		idx := int(p / 100.0 * float64(len(keyPercentiles)-1))
		out[i] = keyPercentiles[idx]
	}
	return out
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

func (u unfairCounter) GetPass(key string, base int64, inc int64) int64 { return base }
func (u unfairCounter) EstimateDistinctKeys() int                       { return 0 }

package fairsim

import (
	"cmp"
	"container/heap"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"

	"go.temporal.io/server/service/matching/counter"
)

const stride = 10000

type (
	taskGenFunc func() *task

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
		byKey             map[string][]int64   // latencies by fairness key
		byKeyNormalized   map[string][]float64 // normalized latencies by fairness key
		overall           []int64              // all latencies
		overallNormalized []float64            // all normalized latencies
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
	var (
		seed        = flag.Int64("seed", rand.Int64(), "Random seed")
		fair        = flag.Bool("fair", true, "Enable fairness (false for FIFO)")
		partitions  = flag.Int("partitions", 4, "Number of partitions")
		tasks       = flag.Int("tasks", 10000, "Number of tasks to generate")
		zipf_s      = flag.Float64("zipf-s", 2.0, "Zipf distribution s parameter")
		zipf_v      = flag.Float64("zipf-v", 2.0, "Zipf distribution v parameter")
		numKeys     = flag.Int("keys", 100, "Number of unique fairness keys")
		counterFile = flag.String("counter-params", "", "JSON file with CounterParams")
	)
	flag.CommandLine.Parse(args)

	// Load counter params
	params := counter.DefaultCounterParams
	if *counterFile != "" {
		data, err := os.ReadFile(*counterFile)
		if err != nil {
			return fmt.Errorf("failed to load counter params: %w", err)
		} else if err = json.Unmarshal(data, &params); err != nil {
			return fmt.Errorf("failed to load counter params: %w", err)
		}
	}

	src := rand.NewPCG(uint64(*seed), uint64(*seed+1))
	rnd := rand.New(src)

	counterFactory := func() counter.Counter { return unfairCounter{} }
	if *fair {
		counterFactory = func() counter.Counter { return counter.NewHybridCounter(params, src) }
	}

	state := state{
		rnd:            rnd,
		counterFactory: counterFactory,
		partitions:     make([]partitionState, *partitions),
	}

	stats := latencyStats{
		byKey:           make(map[string][]int64),
		byKeyNormalized: make(map[string][]float64),
	}

	var nextIndex, dispatchCounter int64

	const defaultPriority = 3

	var gen taskGenFunc

	if true {
		zipf := rand.NewZipf(rnd, *zipf_s, *zipf_v, uint64(*numKeys-1))

		tasksLeft := *tasks
		gen = func() *task {
			tasksLeft--
			if tasksLeft < 0 {
				return nil
			}
			fkey := fmt.Sprintf("fkey%d", zipf.Uint64())
			var pri int
			// pri = min(5, max(1, defaultPriority+int(math.Round(rnd.NormFloat64()*0.5))))
			return &task{pri: pri, fkey: fkey}
		}
	} else {
		// TODO: option to read keys/weights from file
	}

	// add all tasks
	for t := gen(); t != nil; t = gen() {
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

	// Calculate normalized latencies
	stats.calculateNormalized()

	// Print final latency stats
	stats.print()

	return nil
}

func (stats *latencyStats) calculateNormalized() {
	// Calculate normalized latencies: raw_latency / task_count_for_key
	for key, latencies := range stats.byKey {
		taskCount := float64(len(latencies))
		normalizedLatencies := make([]float64, len(latencies))

		for i, rawLatency := range latencies {
			normalizedLatencies[i] = float64(rawLatency) / taskCount
			stats.overallNormalized = append(stats.overallNormalized, normalizedLatencies[i])
		}

		stats.byKeyNormalized[key] = normalizedLatencies
	}
}

func (stats *latencyStats) print() {
	// Overall raw stats
	if len(stats.overall) > 0 {
		slices.Sort(stats.overall)
		mean := float64(sum(stats.overall)) / float64(len(stats.overall))
		median := stats.overall[len(stats.overall)/2]
		p95 := stats.overall[int(float64(len(stats.overall))*0.95)]

		fmt.Printf("\n=== Raw Latency Statistics ===\n")
		fmt.Printf("Overall: mean=%.2f, median=%d, p95=%d, min=%d, max=%d\n",
			mean, median, p95, stats.overall[0], stats.overall[len(stats.overall)-1])
	}

	// Overall normalized stats
	if len(stats.overallNormalized) > 0 {
		slices.Sort(stats.overallNormalized)
		mean := sum(stats.overallNormalized) / float64(len(stats.overallNormalized))
		median := stats.overallNormalized[len(stats.overallNormalized)/2]
		p95 := stats.overallNormalized[int(float64(len(stats.overallNormalized))*0.95)]

		fmt.Printf("\n=== Normalized Latency Statistics ===\n")
		fmt.Printf("Overall: mean=%.4f, median=%.4f, p95=%.4f, min=%.4f, max=%.4f\n",
			mean, median, p95, stats.overallNormalized[0], stats.overallNormalized[len(stats.overallNormalized)-1])
	}

	// Per-key stats (sorted by mean normalized latency)
	type keyStats struct {
		key              string
		meanRaw          float64
		medianRaw        int64
		meanNormalized   float64
		medianNormalized float64
		count            int
	}

	var keyStatsList []keyStats
	for key, latencies := range stats.byKey {
		if len(latencies) == 0 {
			continue
		}

		// Raw stats
		slices.Sort(latencies)
		meanRaw := float64(sum(latencies)) / float64(len(latencies))
		medianRaw := latencies[len(latencies)/2]

		// Normalized stats
		normalizedLatencies := stats.byKeyNormalized[key]
		slices.Sort(normalizedLatencies)
		meanNormalized := sum(normalizedLatencies) / float64(len(normalizedLatencies))
		medianNormalized := normalizedLatencies[len(normalizedLatencies)/2]

		keyStatsList = append(keyStatsList, keyStats{
			key:              key,
			meanRaw:          meanRaw,
			medianRaw:        medianRaw,
			meanNormalized:   meanNormalized,
			medianNormalized: medianNormalized,
			count:            len(latencies),
		})
	}

	slices.SortFunc(keyStatsList, func(a, b keyStats) int { return cmp.Compare(a.medianNormalized, b.medianNormalized) })

	fmt.Printf("\nPer-key stats (sorted by median normalized latency):\n")
	for _, ks := range keyStatsList {
		fmt.Printf("  %s: raw(mean=%.2f, median=%d) norm(mean=%.4f, median=%.4f) count=%d\n",
			ks.key, ks.meanRaw, ks.medianRaw, ks.meanNormalized, ks.medianNormalized, ks.count)
	}

	// Raw fairness metrics (percentile of percentiles)
	ps := []float64{20, 50, 80, 90, 95}

	fmt.Printf("\nRaw fairness metrics (percentile of per-key percentiles):\n")
	fmt.Printf("         @%-4.0f  @%-4.0f  @%-4.0f  @%-4.0f  @%-4.0f\n",
		ps[0], ps[1], ps[2], ps[3], ps[4])
	for _, p := range ps {
		pofps := percentileOfPercentiles(stats.byKey, p, ps)
		fmt.Printf("  p%2.0fs: %5.0f  %5.0f  %5.0f  %5.0f  %5.0f\n",
			p, pofps[0], pofps[1], pofps[2], pofps[3], pofps[4])
	}

	// Normalized fairness metrics (percentile of percentiles)
	fmt.Printf("\nNormalized fairness metrics (percentile of per-key percentiles):\n")
	fmt.Printf("         @%-4.0f  @%-4.0f  @%-4.0f  @%-4.0f  @%-4.0f\n",
		ps[0], ps[1], ps[2], ps[3], ps[4])
	for _, p := range ps {
		pofps := percentileOfPercentiles(stats.byKeyNormalized, p, ps)
		fmt.Printf("  p%2.0fs: %5.0f  %5.0f  %5.0f  %5.0f  %5.0f\n",
			p, pofps[0], pofps[1], pofps[2], pofps[3], pofps[4])
	}
}

func sum[T int64 | float64](slice []T) T {
	var total T
	for _, v := range slice {
		total += v
	}
	return total
}

// percentileOfPercentiles calculates the cross-key percentile of per-key percentiles
// Works with both int64 and float64 maps by converting everything to float64
func percentileOfPercentiles[T int64 | float64](dataByKey map[string][]T, keyPercentile float64, crossPercentile []float64) []float64 {
	var keyPercentiles []float64

	for _, values := range dataByKey {
		if len(values) == 0 {
			continue
		}

		// Convert to float64 and sort
		sorted := make([]float64, len(values))
		for i, v := range values {
			sorted[i] = float64(v)
		}
		slices.Sort(sorted)

		// Calculate the keyPercentile for this key
		idx := int(keyPercentile / 100.0 * float64(len(sorted)-1))
		keyPercentiles = append(keyPercentiles, sorted[idx])
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
func (s *state) addTask(t *task) {
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

	heap.Push(&partition.heap, t)
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

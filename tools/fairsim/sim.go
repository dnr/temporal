package fairsim

import (
	"bufio"
	"cmp"
	"container/heap"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"

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
		payload string
	}

	state struct {
		rnd            *rand.Rand
		counterFactory func() counter.Counter
		partitions     []partitionState
	}

	simulator struct {
		state           *state
		stats           *latencyStats
		nextIndex       int64
		dispatchCounter int64
		defaultPriority int
		rnd             *rand.Rand
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
		scriptFile  = flag.String("script", "", "Script file to execute instead of generating tasks")
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

	state := newState(rnd, counterFactory, *partitions)

	stats := newLatencyStats()

	const defaultPriority = 3

	sim := newSimulator(state, stats, defaultPriority, rnd)

	// Check if script mode
	if *scriptFile != "" {
		return sim.runScript(*scriptFile)
	}

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
		sim.addTask(t)
	}

	// pop all tasks and print
	sim.drainAndPrintTasks()

	// Calculate normalized latencies and print stats
	sim.stats.calculateNormalized()
	sim.stats.print()

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
	for _, idx := range rnd.Perm(len(s.partitions)) {
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

func newLatencyStats() *latencyStats {
	return &latencyStats{
		byKey:           make(map[string][]int64),
		byKeyNormalized: make(map[string][]float64),
	}
}

func newState(rnd *rand.Rand, counterFactory func() counter.Counter, partitions int) *state {
	return &state{
		rnd:            rnd,
		counterFactory: counterFactory,
		partitions:     make([]partitionState, partitions),
	}
}

func newSimulator(state *state, stats *latencyStats, defaultPriority int, rnd *rand.Rand) *simulator {
	return &simulator{
		state:           state,
		stats:           stats,
		nextIndex:       0,
		dispatchCounter: 0,
		defaultPriority: defaultPriority,
		rnd:             rnd,
	}
}

func (sim *simulator) addTask(t *task) {
	t.pri = cmp.Or(t.pri, sim.defaultPriority)
	t.fweight = cmp.Or(t.fweight, 1.0)
	t.index = sim.nextIndex
	sim.nextIndex++
	sim.state.addTask(t)
}

func (sim *simulator) runScript(scriptFile string) error {
	file, err := os.Open(scriptFile)
	if err != nil {
		return fmt.Errorf("failed to open script file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if err := sim.executeCommand(line); err != nil {
			return fmt.Errorf("error executing command '%s': %w", line, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading script file: %w", err)
	}

	// After script is done, pop and print all remaining tasks
	sim.drainAndPrintTasks()

	// Calculate normalized latencies and print stats
	sim.stats.calculateNormalized()
	sim.stats.print()

	return nil
}

func (sim *simulator) executeCommand(line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}

	cmd := parts[0]
	switch cmd {
	case "task":
		return sim.executeTaskCommand(parts[1:])
	case "poll":
		return sim.executePollCommand()
	case "stats":
		return sim.executeStatsCommand()
	case "clearstats":
		return sim.executeClearStatsCommand()
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func (sim *simulator) executeTaskCommand(args []string) error {
	t := &task{}

	for _, arg := range args {
		if strings.HasPrefix(arg, "-fkey=") {
			t.fkey = arg[6:]
		} else if strings.HasPrefix(arg, "-fweight=") {
			weight, err := strconv.ParseFloat(arg[9:], 32)
			if err != nil {
				return fmt.Errorf("invalid fweight: %w", err)
			}
			t.fweight = float32(weight)
		} else if strings.HasPrefix(arg, "-pri=") {
			pri, err := strconv.Atoi(arg[5:])
			if err != nil {
				return fmt.Errorf("invalid priority: %w", err)
			}
			t.pri = pri
		} else if strings.HasPrefix(arg, "-payload=") {
			t.payload = arg[9:]
		} else {
			return fmt.Errorf("unknown task argument: %s", arg)
		}
	}

	if t.fkey == "" {
		t.fkey = "default"
	}

	sim.addTask(t)
	return nil
}

func (sim *simulator) executePollCommand() error {
	t, partition := sim.state.popTask(sim.rnd)
	if t == nil {
		fmt.Println("No tasks in queue")
		return nil
	}

	sim.processAndPrintTask(t, partition)
	return nil
}

func (sim *simulator) executeStatsCommand() error {
	sim.stats.calculateNormalized()
	sim.stats.print()
	return nil
}

func (sim *simulator) executeClearStatsCommand() error {
	sim.stats = newLatencyStats()
	return nil
}

func (sim *simulator) drainAndPrintTasks() {
	for t, partition := sim.state.popTask(sim.rnd); t != nil; t, partition = sim.state.popTask(sim.rnd) {
		sim.processAndPrintTask(t, partition)
	}
}

func (sim *simulator) processAndPrintTask(t *task, partition int) {
	latency := sim.dispatchCounter - t.index
	sim.dispatchCounter++

	sim.stats.byKey[t.fkey] = append(sim.stats.byKey[t.fkey], latency)
	sim.stats.overall = append(sim.stats.overall, latency)

	fmt.Printf("task idx-dsp:%6d-%6d = %6d  pri:%2d  fkey:%10q  fweight:%3g  part:%2d  payload:%q\n", t.index, sim.dispatchCounter, latency, t.pri, t.fkey, t.fweight, partition, t.payload)
}

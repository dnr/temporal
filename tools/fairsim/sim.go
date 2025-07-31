package fairsim

import (
	"cmp"
	"container/heap"
	"fmt"
	"math"
	"math/rand/v2"

	"go.temporal.io/server/service/matching/counter"
)

const stride = 10000

type (
	taskGenFunc func() (task, bool)

	task struct {
		id      int
		pri     int
		fkey    string
		fweight float32
		pass    int64
	}

	state struct {
		partitions []partitionState
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

var _nextid int

func nextid() int {
	_nextid++
	return _nextid
}

func RunTool(args []string) error {
	params := counter.DefaultCounterParams
	// TODO: be able to load params from a json file specified by a flag

	// TODO: be able to specify seed from a flag
	src := rand.NewPCG(rand.Uint64(), rand.Uint64())
	rnd := rand.New(src)

	counterFactory := func() counter.Counter {
		return counter.NewHybridCounter(params, src)
	}

	// TODO: implement partitions
	// TODO: set number of partitions from command line
	const partitions = 4

	var state state
	state.partitions = make([]partitionState, partitions)

	const tasks = 10000
	const defaultPriority = 3

	var gen taskGenFunc

	if true {
		// TODO: add flags to override these
		const zipf_s = 2.0
		const zipf_v = 2.0
		const keys = 1000

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
		t.id = nextid()
		t.pri = cmp.Or(t.pri, defaultPriority)
		t.fweight = cmp.Or(t.fweight, 1.0)
		state.addTask(t, counterFactory, rnd)
	}

	// pop all tasks and print
	for t, ok := state.popTask(rnd); ok; t, ok = state.popTask(rnd) {
		fmt.Printf("task id=%d, pri=%d, fkey=%q, fweight: %g\n", t.id, t.pri, t.fkey, t.fweight)
		// TODO: compute some notion of logical "latency" based on position (not real time)
		// TODO: track some latency statistics by key and across all tasks
	}

	// TODO: print final latency stats

	return nil
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
	return h[i].id < h[j].id
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
func (s *state) addTask(t task, counterFactory func() counter.Counter, rnd *rand.Rand) {
	// Pick a random partition
	partitionIdx := rnd.IntN(len(s.partitions))
	partition := &s.partitions[partitionIdx]

	if partition.perPri == nil {
		partition.perPri = make(map[int]perPriState)
	}

	priState, exists := partition.perPri[t.pri]
	if !exists {
		priState = perPriState{c: counterFactory()}
		partition.perPri[t.pri] = priState
	}

	// Pick pass using counter like fairTaskWriter does
	// Baseline is 0 (current ack level assumed to be zero)
	pass := priState.c.GetPass(t.fkey, 0, int64(float32(stride)/t.fweight))
	t.pass = pass

	heap.Push(&partition.heap, &t)
}

// popTask returns the task with minimum (pri, pass, id) from a random partition
func (s *state) popTask(rnd *rand.Rand) (task, bool) {
	// Check if any partition has tasks
	totalTasks := 0
	for i := range s.partitions {
		totalTasks += s.partitions[i].heap.Len()
	}
	if totalTasks == 0 {
		return task{}, false
	}

	// Pick a random partition and try to pop from it
	// If it's empty, try other partitions in order
	startIdx := rnd.IntN(len(s.partitions))
	for i := 0; i < len(s.partitions); i++ {
		partitionIdx := (startIdx + i) % len(s.partitions)
		partition := &s.partitions[partitionIdx]
		
		if partition.heap.Len() > 0 {
			t := heap.Pop(&partition.heap).(*task)
			return *t, true
		}
	}
	
	// This should never happen since we checked totalTasks > 0
	return task{}, false
}

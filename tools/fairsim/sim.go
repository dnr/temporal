package fairsim

import (
	"fmt"
	"math/rand/v2"

	"go.temporal.io/server/service/matching/counter"
)

type (
	taskGenFunc func() (task, bool)

	task struct {
		id      int
		pri     int
		fkey    string
		fweight float32
	}

	state struct {
		perPri map[int]perPriState
	}

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

	counterFactory := func() counter.Counter {
		return counter.NewHybridCounter(params, src)
	}

	// FIXME: put state here
	var state state

	const tasks = 10000

	var gen taskGenFunc

	if true {
		// TODO: add flags to override these
		const zipf_s = 2.0
		const zipf_v = 2.0
		const keys = 1000

		zipf := rand.NewZipf(rand.New(src), zipf_s, zipf_v, keys-1)

		tasksLeft := tasks
		gen = func() (task, bool) {
			tasksLeft--
			if tasksLeft < 0 {
				return task{}, false
			}
			return task{fkey: fmt.Sprintf("fkey%d", zipf.Uint64())}, true
		}
	} else {
		// TODO: option to read keys/weights from file
	}

	// add all tasks
	for t, ok := gen(); ok; t, ok = gen() {
		t.id = nextid()
		state.addTask(t)
	}

	// pop all tasks and print
	for t, ok := state.popTask(); ok; t, ok = state.popTask() {
		fmt.Printf("task id=%d, pri=%d, fkey=%q, fweight: %g\n", t.id, t.pri, t.fkey, t.fweight)
		// TODO: compute some notion of logical "latency" based on position (not real time)
		// TODO: track some latency statistics by key and across all tasks
	}

	// TODO: print final latency stats

	return nil
}

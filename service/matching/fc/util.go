package fc

import (
	"cmp"
	"fmt"
	"slices"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/server/common/util"
)

// Maximum total limiters that can apply to any task. (Currently this includes the whole-queue
// limiter, in the future we may allow the whole-queue limiter to be separate from this limit.)
const maxLimiters = 3

type limiterSource int32

const (
	// not valid limiter
	limiterSourceInvalid limiterSource = iota
	// limiter came from task queue config, applies to the whole queue
	limiterSourceConfig_WholeQueue
	// limiter came from task itself
	limiterSourceTask
	// future: namespace policy, etc.
)

// WholeQueueLimiterName returns the limiter key for the whole-queue limiter of a task queue.
// It's exported so that tests can address the limiter directly.
func WholeQueueLimiterName(tqName string, tqType enumspb.TaskQueueType) string {
	// the "/0" at the end is for future extension for partitioning limiters
	return fmt.Sprintf("/_sys/wholequeue/%s/%d/0", tqName, tqType)
}

func canonicalLimiters(task fcTask) []limiter {
	limiters := task.Limiters()
	if limiters == nil {
		return nil
	}
	out := util.FilterSlice(limiters.limiters[:], limiter.valid)
	// we must reserve limiters in a consistent canonical order to avoid quasi-deadlock
	slices.SortFunc(out, func(a, b limiter) int {
		if v := cmp.Compare(a.tp, b.tp); v != 0 {
			return v
		}
		return cmp.Compare(a.key, b.key)
	})
	return out
}

package matching

import (
	"cmp"
	"fmt"
	"slices"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/common/util"
)

// Maximum total limiters that can apply to any task. (Currently this includes the whole-queue
// limiter, in the future we may allow the whole-queue limiter to be separate from this limit.)
const maxLimiters = 3

// Error used to indicate a limiter is blocked.
var errFCLimiterBlocked = serviceerror.NewFailedPrecondition("limiter blocked")

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

func wholeQueueLimiterName(tqName string, tqType enumspb.TaskQueueType) string {
	// the "/0" at the end is for future extension for partitioning limiters
	return fmt.Sprintf("/_sys/wholequeue/%s/%d/0", tqName, tqType)
}

func canonicalLimiters(task *internalTask) []fcLimiter {
	if task.limiters == nil {
		return nil
	}
	out := util.FilterSlice(task.limiters.limiters[:], fcLimiter.valid)
	// we must reserve limiters in a consistent canonical order to avoid quasi-deadlock
	slices.SortFunc(out, func(a, b fcLimiter) int {
		if v := cmp.Compare(a.tp, b.tp); v != 0 {
			return v
		}
		return cmp.Compare(a.key, b.key)
	})
	return out
}

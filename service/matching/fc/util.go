package fc

import (
	"cmp"
	"slices"
	"time"

	"go.temporal.io/server/common/util"
)

// Maximum total limiters that can apply to any task. (Currently this includes whole-queue
// limiters, in the future we may allow whole-queue limiters to be separate from this limit.)
const MaxLimiters = 3

// wakePriority is a namespace-scoped value that we can order waiters by. Currently we use a
// combination of priority key and task age.
type wakePriority int64

// makeWakePriority
func makeWakePriority(pri int32, age time.Time) wakePriority {
	// pri is [1-60] (matching.maxPriorityLevels), so we can fit pri and age into 63 bits:
	// 00000000[--pri-][----------------------age---------------------]
	pri = min(max(pri, 0), 255)
	return wakePriority(pri)<<48 | wakePriority(age.UnixMilli())
}

// canonicalLimiters returns and orders the limiters of a task in a canonical order, so two
// partitions don't get in a quasi-deadlock by using opposite orders.
func canonicalLimiters(task fcTask) []Limiter {
	limiters := task.Limiters()
	if limiters == nil {
		return nil
	}
	out := util.FilterSlice(limiters.Limiters[:], Limiter.Valid)
	slices.SortFunc(out, func(a, b Limiter) int {
		if v := cmp.Compare(a.Type, b.Type); v != 0 {
			return v
		}
		return cmp.Compare(a.Key, b.Key)
	})
	return out
}

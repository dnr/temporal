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

type wakePriority int64

func canonicalLimiters(task fcTask) []Limiter {
	limiters := task.Limiters()
	if limiters == nil {
		return nil
	}
	out := util.FilterSlice(limiters.Limiters[:], Limiter.Valid)
	// we must reserve limiters in a consistent canonical order to avoid quasi-deadlock
	slices.SortFunc(out, func(a, b Limiter) int {
		if v := cmp.Compare(a.Type, b.Type); v != 0 {
			return v
		}
		return cmp.Compare(a.Key, b.Key)
	})
	return out
}

func makeWakePriority(pri int32, age time.Time) wakePriority {
	// pri is [1-60] (matching.maxPriorityLevels), so we can fit pri and age into 63 bits:
	// 00000000[--pri-][----------------------age---------------------]
	return wakePriority(max(pri, 0))<<48 | wakePriority(age.UnixMilli())
}

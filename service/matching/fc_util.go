package matching

import (
	"cmp"
	"fmt"
	"slices"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/server/common/util"
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

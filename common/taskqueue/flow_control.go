package taskqueue

import (
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
)

func NeedsRelease(ref *taskqueuespb.LimiterRef) bool {
	switch ref.GetLimiterType() {
	case enumsspb.LIMITER_TYPE_CONCURRENCY:
		return true
	default:
		return false
	}
}

// ContainsLimiterRef returns whether refs contains a ref to the same limiter slot as want.
func ContainsLimiterRef(refs []*taskqueuespb.LimiterRef, want *taskqueuespb.LimiterRef) bool {
	for _, ref := range refs {
		if ref.GetLimiterType() == want.GetLimiterType() &&
			ref.GetKey() == want.GetKey() &&
			ref.GetSlotId() == want.GetSlotId() {
			return true
		}
	}
	return false
}

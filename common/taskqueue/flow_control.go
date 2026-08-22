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

func EqualLimiterRef(a *taskqueuespb.LimiterRef, b *taskqueuespb.LimiterRef) bool {
	return a.GetLimiterType() == b.GetLimiterType() &&
		a.GetKey() == b.GetKey() &&
		a.GetSlotId() == b.GetSlotId()
}

func ContainsLimiterRef(refs []*taskqueuespb.LimiterRef, want *taskqueuespb.LimiterRef) bool {
	for _, ref := range refs {
		if EqualLimiterRef(ref, want) {
			return true
		}
	}
	return false
}

func FindLimiterRelease(
	releases []*taskqueuespb.LimiterRelease,
	want *taskqueuespb.LimiterRef,
) *taskqueuespb.LimiterRelease {
	for _, release := range releases {
		if EqualLimiterRef(release.GetLimiter(), want) {
			return release
		}
	}
	return nil
}

func ReleaseRecorded(releases []*taskqueuespb.LimiterRelease) bool {
	if len(releases) == 0 {
		return false
	}
	for _, release := range releases {
		if len(release.GetComponentRef()) == 0 {
			return false
		}
	}
	return true
}

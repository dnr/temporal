package scheduler

import (
	"time"

	schedpb "go.temporal.io/api/schedule/v1"
)

func getNextTime(spec *schedpb.ScheduleSpec, state *schedpb.ScheduleState, after time.Time) (nominal, next time.Time, has bool) {

	// add jitter
	// FIXME: but what if jitter pushes one time past the one ahead of it?

	return
}

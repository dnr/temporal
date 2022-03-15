package scheduler

import (
	"time"

	"github.com/dgryski/go-farm"
	schedpb "go.temporal.io/api/schedule/v1"
	"go.temporal.io/server/common/primitives/timestamp"
)

func getNextTime(
	spec *schedpb.ScheduleSpec,
	state *schedpb.ScheduleState,
	after time.Time,
	doingBackfill bool,
) (nominal, next time.Time, has bool) {
	if (state.Paused || state.LimitedActions && state.RemainingActions == 0) && !doingBackfill {
		has = false
		return
	}

	if spec.NotBefore != nil && after.Before(*spec.NotBefore) {
		after = spec.NotBefore.Add(-time.Second)
	}

	for {
		nominal = rawNextTime(spec, after)

		if spec.NotAfter != nil && nominal.After(*spec.NotAfter) {
			has = false
			return
		}

		// check against excludes
		if !excluded(nominal, spec.ExcludeCalendar) {
			break
		}

		after = nominal
	}

	// Ensure that jitter doesn't push this time past the _next_ nominal start time
	following := rawNextTime(spec, nominal)
	next = addJitter(spec, nominal, following.Sub(nominal))

	return
}

func rawNextTime(
	spec *schedpb.ScheduleSpec,
	after time.Time,
) (nominal time.Time) {
	// FIXME: consider all calendarspecs

	// FIXME: consider all intervalspecs
}

func excluded(nominal time.Time, excludes []*schedpb.CalendarSpec) bool {
	// FIXME
	return false
}

func addJitter(spec *schedpb.ScheduleSpec, nominal time.Time, limit time.Duration) time.Time {
	maxJitter := timestamp.DurationValue(spec.Jitter)
	if maxJitter == 0 {
		maxJitter = 1 * time.Second // FIXME: constant
	}
	if maxJitter > limit {
		maxJitter = limit
	}

	bin, err := nominal.MarshalBinary()
	if err != nil {
		return nominal
	}

	fp := int64(farm.Fingerprint32(bin))
	ms := int64(maxJitter.Milliseconds())
	jitter := time.Duration((fp*ms)>>32) * time.Millisecond
	return nominal.Add(jitter)
}

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
		// consider all calendarspecs

		// consider all intervalspecs

		// check against excludes
		if !excluded(nominal, spec.ExcludeCalendar) {
			break
		}
	}

	if spec.NotAfter != nil && nominal.After(*spec.NotAfter) {
		has = false
		return
	}

	next = addJitter(spec, nominal)

	// FIXME: but what if jitter pushes one time past the one ahead of it?

	return
}

func excluded(nominal time.Time, excludes []*schedpb.CalendarSpec) bool {
	// FIXME
	return false
}

func addJitter(spec *schedpb.ScheduleSpec, nominal time.Time) time.Time {
	if timestamp.DurationValue(spec.Jitter) == 0 {
		def := 1 * time.Second
		spec.Jitter = &def
	}
	bin, err := nominal.MarshalBinary()
	if err != nil {
		return nominal
	}
	j := (int64(farm.Fingerprint32(bin)) * spec.Jitter.Milliseconds()) / 1 << 32
	jitter := j * time.Millisecond
	return nominal.Add(jitter)
}

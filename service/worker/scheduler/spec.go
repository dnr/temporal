package scheduler

import (
	"math"
	"time"

	"github.com/dgryski/go-farm"
	schedpb "go.temporal.io/api/schedule/v1"
	"go.temporal.io/server/common/primitives/timestamp"
)

const (
	defaultJitter = 1 * time.Second
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

	tz, err := loadTimezone(spec)
	if err != nil {
		// FIXME: log error
		has = false
		return
	}

	if spec.NotBefore != nil && after.Before(*spec.NotBefore) {
		after = spec.NotBefore.Add(-time.Second)
	}

	for {
		nominal = rawNextTime(spec, after, tz)

		if nominal.IsZero() || (spec.NotAfter != nil && nominal.After(*spec.NotAfter)) {
			has = false
			return
		}

		// check against excludes
		if !excluded(nominal, spec.ExcludeCalendar, tz) {
			break
		}

		after = nominal
	}

	// Ensure that jitter doesn't push this time past the _next_ nominal start time
	following := rawNextTime(spec, nominal, tz)
	next = addJitter(spec, nominal, following.Sub(nominal))

	return
}

func rawNextTime(
	spec *schedpb.ScheduleSpec,
	after time.Time,
	tz *time.Location,
) (nominal time.Time) {
	var minTimestamp int64 = math.MaxInt64 // unix seconds-since-epoch as int64

	for _, cal := range spec.Calendar {
		if matcher, err := newCalendarMatcher(cal, tz); err == nil {
			if next := matcher.nextCalendarTime(after); !next.IsZero() {
				nextTs := next.Unix()
				if nextTs < minTimestamp {
					minTimestamp = nextTs
				}
			}
		}
	}

	ts := after.Unix()
	for _, iv := range spec.Interval {
		next := nextIntervalTime(iv, ts)
		if next < minTimestamp {
			minTimestamp = next
		}
	}

	if minTimestamp == math.MaxInt64 {
		return time.Time{}
	}
	return time.Unix(minTimestamp, 0).UTC()
}

func nextIntervalTime(iv *schedpb.IntervalSpec, ts int64) int64 {
	interval := int64(timestamp.DurationValue(iv.Interval) / time.Second)
	if interval < 1 {
		interval = 1
	}
	phase := int64(timestamp.DurationValue(iv.Phase) / time.Second)
	if phase < 0 {
		phase = 0
	}
	return (((ts-phase)/interval)+1)*interval + phase
}

func excluded(nominal time.Time, excludes []*schedpb.CalendarSpec, tz *time.Location) bool {
	for _, exclude := range excludes {
		if ms, err := newCalendarMatcher(exclude, tz); err == nil {
			if ms.matches(nominal) {
				return true
			}
		}
	}
	return false
}

func addJitter(spec *schedpb.ScheduleSpec, nominal time.Time, limit time.Duration) time.Time {
	maxJitter := timestamp.DurationValue(spec.Jitter)
	if maxJitter == 0 {
		maxJitter = defaultJitter
	}
	if maxJitter > limit {
		maxJitter = limit
	}

	bin, err := nominal.MarshalBinary()
	if err != nil {
		return nominal
	}

	fp := int64(farm.Fingerprint32(bin))
	ms := maxJitter.Milliseconds()
	jitter := time.Duration((fp*ms)>>32) * time.Millisecond
	return nominal.Add(jitter)
}

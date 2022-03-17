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

type (
	compiledSpec struct {
		spec     *schedpb.ScheduleSpec
		tz       *time.Location
		calendar []*calendarMatcher
		excludes []*calendarMatcher
	}
)

func newCompiledSpec(spec *schedpb.ScheduleSpec) (*compiledSpec, error) {
	tz, err := loadTimezone(spec)
	if err != nil {
		return nil, err
	}

	cspec := &compiledSpec{
		spec:     spec,
		tz:       tz,
		calendar: make([]*calendarMatcher, len(spec.Calendar)),
		excludes: make([]*calendarMatcher, len(spec.ExcludeCalendar)),
	}

	for i, cal := range spec.Calendar {
		if cspec.calendar[i], err = newCalendarMatcher(cal, tz); err != nil {
			return nil, err
		}
	}
	for i, excal := range spec.ExcludeCalendar {
		if cspec.excludes[i], err = newCalendarMatcher(excal, tz); err != nil {
			return nil, err
		}
	}

	return cspec, nil
}

func loadTimezone(spec *schedpb.ScheduleSpec) (*time.Location, error) {
	if spec.TimezoneData != nil {
		return time.LoadLocationFromTZData(spec.TimezoneName, spec.TimezoneData)
	}
	return time.LoadLocation(spec.TimezoneName)
}

func (cs *compiledSpec) getNextTime(
	state *schedpb.ScheduleState,
	after time.Time,
	doingBackfill bool,
) (nominal, next time.Time, has bool) {
	if (state.Paused || state.LimitedActions && state.RemainingActions == 0) && !doingBackfill {
		has = false
		return
	}

	if cs.spec.NotBefore != nil && after.Before(*cs.spec.NotBefore) {
		after = cs.spec.NotBefore.Add(-time.Second)
	}

	for {
		nominal = cs.rawNextTime(after)

		if nominal.IsZero() || (cs.spec.NotAfter != nil && nominal.After(*cs.spec.NotAfter)) {
			has = false
			return
		}

		// check against excludes
		if !cs.excluded(nominal) {
			break
		}

		after = nominal
	}

	// Ensure that jitter doesn't push this time past the _next_ nominal start time
	following := cs.rawNextTime(nominal)
	next = cs.addJitter(nominal, following.Sub(nominal))

	return
}

func (cs *compiledSpec) rawNextTime(after time.Time) (nominal time.Time) {
	var minTimestamp int64 = math.MaxInt64 // unix seconds-since-epoch as int64

	for _, cal := range cs.calendar {
		if next := cal.nextCalendarTime(after); !next.IsZero() {
			nextTs := next.Unix()
			if nextTs < minTimestamp {
				minTimestamp = nextTs
			}
		}
	}

	ts := after.Unix()
	for _, iv := range cs.spec.Interval {
		next := cs.nextIntervalTime(iv, ts)
		if next < minTimestamp {
			minTimestamp = next
		}
	}

	if minTimestamp == math.MaxInt64 {
		return time.Time{}
	}
	return time.Unix(minTimestamp, 0).UTC()
}

func (cs *compiledSpec) nextIntervalTime(iv *schedpb.IntervalSpec, ts int64) int64 {
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

func (cs *compiledSpec) excluded(nominal time.Time) bool {
	for _, excal := range cs.excludes {
		if excal.matches(nominal) {
			return true
		}
	}
	return false
}

func (cs *compiledSpec) addJitter(nominal time.Time, limit time.Duration) time.Time {
	maxJitter := timestamp.DurationValue(cs.spec.Jitter)
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

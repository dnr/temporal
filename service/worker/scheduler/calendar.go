package scheduler

import (
	"time"

	schedpb "go.temporal.io/api/schedule/v1"
)

const (
	// maxCalendarYear is the highest year that will be checked for matching calendar dates.
	// If you're still using Temporal in 2100 please change this constant and rebuild.
	maxCalendarYear = 2100
)

func loadTimezone(spec *schedpb.ScheduleSpec) (*time.Location, error) {
	if spec.TimezoneData != nil {
		return time.LoadLocationFromTZData(spec.TimezoneName, spec.TimezoneData)
	}
	return time.LoadLocation(spec.TimezoneName)
}

func nextCalendarTime(tz *time.Location, cal *schedpb.CalendarSpec, ts time.Time) time.Time {
	// set time zone
	ts = ts.In(tz)

	// make matchers
	yearMatch := makeYearMatch(cal.Year)
	monthMatch := makeMonthMatch(cal.Month)
	dayOfMonthMatch := makeDayOfMonthMatch(cal.DayOfMonth)
	dayOfWeekMatch := makeDayOfWeekMatch(cal.DayOfWeek)
	hourMatch := makeHourMatch(cal.Hour)
	minuteMatch := makeMinuteMatch(cal.Minute)
	secondMatch := makeSecondMatch(cal.Second)

	// get ymdhms from ts
	y, mo, d := ts.Date()
	h, m, s := ts.Clock()

	// looking for first matching time after ts, so add 1 second
	s++
Outer:
	for {
		// normalize after carries
		if s >= 60 {
			m, s = m+1, 0
		}
		if m >= 60 {
			h, m = h+1, 0
		}
		if h >= 24 {
			d, h = d+1, 0
		}
		if d > daysInMonth(mo, y) {
			mo, d = mo+1, 1
		}
		if mo > time.December {
			y, mo = y+1, time.January
		}
		if y > maxCalendarYear {
			break Outer
		}
		// try to match year, month, etc. from outside in
		if !yearMatch(y) {
			y, mo, d, h, m, s = y+1, time.January, 1, 0, 0, 0
			continue Outer
		}
		for !monthMatch(mo) {
			mo, d, h, m, s = mo+1, 1, 0, 0, 0
			if mo > time.December {
				continue Outer
			}
		}
		for !dayOfMonthMatch(d) || !dayOfWeekMatch(time.Date(y, mo, d, h, m, s, 0, tz).Weekday()) {
			d, h, m, s = d+1, 0, 0, 0
			if d > daysInMonth(mo, y) {
				continue Outer
			}
		}
		for !hourMatch(h) {
			h, m, s = h+1, 0, 0
			if h >= 24 {
				continue Outer
			}
		}
		for !minuteMatch(m) {
			m, s = m+1, 0
			if m >= 60 {
				continue Outer
			}
		}
		for !secondMatch(s) {
			s = s + 1
			if s >= 60 {
				continue Outer
			}
		}
		// everything matches
		return time.Date(y, mo, d, h, m, s, 0, tz)
	}

	// no more matching times (up to max we checked)
	return time.Time{}
}

func makeYearMatch(year string) func(int) bool {
}

func makeMonthMatch(month string) func(time.Month) bool {
}

func makeDayOfMonthMatch(dom string) func(int) bool {
}

func makeDayOfWeekMatch(dow string) func(time.Weekday) bool {
}

func makeHourMatch(hour string) func(int) bool {
}

func makeMinuteMatch(minute string) func(int) bool {
}

func makeSecondMatch(second string) func(int) bool {
}

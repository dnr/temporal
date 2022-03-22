// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	schedpb "go.temporal.io/api/schedule/v1"
)

type calendarSuite struct {
	suite.Suite
}

func TestCalendar(t *testing.T) {
	suite.Run(t, new(calendarSuite))
}

func (s *calendarSuite) TestCalendarMatch() {
	pacific, err := time.LoadLocation("US/Pacific")
	s.NoError(err)

	// default is midnight once a day
	cc, err := newCompiledCalendar(&schedpb.CalendarSpec{}, time.UTC)
	s.NoError(err)
	s.True(cc.matches(time.Date(2022, time.March, 17, 0, 0, 0, 0, time.UTC)))
	s.True(cc.matches(time.Date(2022, time.March, 18, 0, 0, 0, 0, time.UTC)))
	s.False(cc.matches(time.Date(2022, time.March, 18, 5, 15, 0, 0, time.UTC)))

	// match another tz
	s.False(cc.matches(time.Date(2022, time.March, 17, 0, 0, 0, 0, pacific)))
	s.True(cc.matches(time.Date(2022, time.March, 17, 17, 0, 0, 0, pacific)))

	cc, err = newCompiledCalendar(&schedpb.CalendarSpec{
		Minute: "5,9",
		Hour:   "*/2",
	}, time.UTC)
	s.NoError(err)
	s.True(cc.matches(time.Date(2022, time.March, 17, 14, 5, 0, 0, time.UTC)))
	s.False(cc.matches(time.Date(2022, time.March, 17, 14, 5, 33, 0, time.UTC)))
	s.False(cc.matches(time.Date(2022, time.March, 17, 14, 15, 0, 0, time.UTC)))
	s.True(cc.matches(time.Date(2022, time.March, 18, 3, 9, 0, 0, pacific)))
	s.False(cc.matches(time.Date(2022, time.March, 18, 3, 9, 0, 0, time.UTC)))

	cc, err = newCompiledCalendar(&schedpb.CalendarSpec{
		Second:     "55",
		Minute:     "55",
		Hour:       "5",
		DayOfWeek:  "wed-thurs",
		DayOfMonth: "2/2",
	}, pacific)
	s.NoError(err)
	s.False(cc.matches(time.Date(2022, time.March, 9, 5, 55, 55, 0, pacific)))
	s.True(cc.matches(time.Date(2022, time.March, 10, 5, 55, 55, 0, pacific)))
	s.False(cc.matches(time.Date(2022, time.March, 14, 5, 55, 55, 0, pacific)))
	s.True(cc.matches(time.Date(2022, time.March, 16, 5, 55, 55, 0, pacific)))
	s.False(cc.matches(time.Date(2022, time.February, 9, 5, 55, 55, 0, pacific)))
	s.True(cc.matches(time.Date(2022, time.February, 10, 5, 55, 55, 0, pacific)))
	s.False(cc.matches(time.Date(2022, time.February, 10, 1, 55, 55, 0, pacific)))
	// match another zone
	s.True(cc.matches(time.Date(2022, time.March, 10, 13, 55, 55, 0, time.UTC)))
	// offset changes between march 10 and 16
	s.False(cc.matches(time.Date(2022, time.March, 16, 13, 55, 55, 0, time.UTC)))
	// correct offset
	s.True(cc.matches(time.Date(2022, time.March, 16, 12, 55, 55, 0, time.UTC)))
}

func (s *calendarSuite) TestCalendarNextBasic() {
	pacific, err := time.LoadLocation("US/Pacific")
	s.NoError(err)

	cc, err := newCompiledCalendar(&schedpb.CalendarSpec{
		Second:     "55",
		Minute:     "55",
		Hour:       "5",
		DayOfWeek:  "wed-thurs",
		DayOfMonth: "2/2",
	}, pacific)
	s.NoError(err)
	// only increment second
	next := cc.nextCalendarTime(time.Date(2022, time.March, 2, 5, 55, 33, 0, pacific))
	s.Equal(time.Date(2022, time.March, 2, 5, 55, 55, 0, pacific), next)
	// increment minute, second
	next = cc.nextCalendarTime(time.Date(2022, time.March, 2, 5, 33, 33, 0, pacific))
	s.Equal(time.Date(2022, time.March, 2, 5, 55, 55, 0, pacific), next)
	// increment hour, minute, second
	next = cc.nextCalendarTime(time.Date(2022, time.March, 2, 3, 33, 33, 0, pacific))
	s.Equal(time.Date(2022, time.March, 2, 5, 55, 55, 0, pacific), next)
	// increment days
	next = cc.nextCalendarTime(time.Date(2022, time.March, 1, 1, 11, 11, 0, pacific))
	s.Equal(time.Date(2022, time.March, 2, 5, 55, 55, 0, pacific), next)
	// from exact match
	next = cc.nextCalendarTime(time.Date(2022, time.March, 2, 5, 55, 55, 0, pacific))
	s.Equal(time.Date(2022, time.March, 10, 5, 55, 55, 0, pacific), next)
	// crossing dst but not near it
	next = cc.nextCalendarTime(time.Date(2022, time.March, 10, 5, 55, 55, 0, pacific))
	s.Equal(time.Date(2022, time.March, 16, 5, 55, 55, 0, pacific), next)
}

func (s *calendarSuite) TestCalendarNextDST() {
	pacific, err := time.LoadLocation("US/Pacific")
	s.NoError(err)
	cc, err := newCompiledCalendar(&schedpb.CalendarSpec{
		Second: "33",
		Minute: "33",
		Hour:   "2",
	}, pacific)
	next := cc.nextCalendarTime(time.Date(2022, time.March, 11, 20, 0, 0, 0, pacific))
	s.Equal(time.Date(2022, time.March, 12, 2, 33, 33, 0, pacific), next)
	// march 13 has no 2:33:33
	next = cc.nextCalendarTime(time.Date(2022, time.March, 13, 1, 15, 15, 0, pacific))
	s.Equal(time.Date(2022, time.March, 14, 2, 33, 33, 0, pacific), next)
	// next = cc.nextCalendarTime(time.Date(2022, time.March, 13, 1, 59, 59, 0, pacific))
	// s.Equal(time.Date(2022, time.March, 14, 2, 33, 33, 0, pacific), next)
}

func (s *calendarSuite) TestMakeMatcher() {
	check := func(str string, min, max int, parseMode parseMode, expected ...int) {
		s.T().Helper()
		m, err := makeMatcher(str, str, min, max, parseMode)
		s.NoError(err)
		for _, e := range expected {
			if e >= 0 {
				s.True(m(e), e)
			} else {
				s.False(m(-e), -e)
			}
		}
	}

	check("1,3,5-7", 0, 10, parseModeInt, 1, -2, 3, -4, 5, 6, 7, -8, -9, -10)
	check("2-4,8,1-5/2", 1, 15, parseModeInt, 1, 2, 3, 4, 5, -6, -7, 8, -9, -10, -11, -12, -13, -14, -15)
	check("February/5", 1, 12, parseModeMonth, -1, 2, -3, -4, -5, -6, 7, -8, -9, -10, -11, 12)
	check("*", 0, 7, parseModeDow, 0, 1, 2, 3, 4, 5, 6, 7)
	check("0", 0, 7, parseModeDow, 0, -1, -2, -3, -4, -5, -6, -7)
	check("2020-2022,2024,2026/3", 2000, 2100, parseModeInt, -2019, 2020, 2021, 2022, -2023, 2024, -2025, 2026, -2027, -2028, 2029, -2030, 2032, 2098, -2101)
}

func (s *calendarSuite) TestParseStringSpec() {
	check := func(str string, min, max int, parseMode parseMode, expected ...int) {
		s.T().Helper()
		var values []int
		f := func(i int) { values = append(values, i) }
		err := parseStringSpec(str, min, max, parseMode, f)
		s.NoError(err)
		s.EqualValues(expected, values)
	}
	checkErr := func(str string, min, max int, parseMode parseMode) {
		s.T().Helper()
		err := parseStringSpec(str, min, max, parseMode, func(i int) {})
		s.Error(err)
	}

	check("13", 0, 59, parseModeInt, 13)
	checkErr("133", 0, 59, parseModeInt)
	check("Sept", 1, 12, parseModeMonth, 9)
	check("13,18", 0, 59, parseModeInt, 13, 18)
	check("13,18,44", 0, 59, parseModeInt, 13, 18, 44)
	checkErr("13,18,44,", 0, 59, parseModeInt)
	check("13-18", 0, 59, parseModeInt, 13, 14, 15, 16, 17, 18)
	checkErr("18-13", 0, 59, parseModeInt)
	checkErr("1,3,18-13", 0, 59, parseModeInt)
	check("2-5,7-9,11", 0, 59, parseModeInt, 2, 3, 4, 5, 7, 8, 9, 11)
	check("*", 5, 9, parseModeInt, 5, 6, 7, 8, 9)
	check("*/3", 5, 9, parseModeInt, 5, 8)
	check("2/3", 0, 10, parseModeInt, 2, 5, 8)
	checkErr("2/3/5", 0, 10, parseModeInt)
	check("2-6/3", 0, 10, parseModeInt, 2, 5)
	check("2-6/4,7-8", 0, 10, parseModeInt, 2, 6, 7, 8)
	check("mon-Friday", 0, 7, parseModeDow, 1, 2, 3, 4, 5)
	checkErr("Fri-Tues", 0, 7, parseModeDow)
	checkErr("1-5-7", 0, 7, parseModeDow)
	checkErr("monday-", 0, 7, parseModeDow)
}

func (s *calendarSuite) TestParseValue() {
	i, err := parseValue("1", 1, 10, parseModeInt)
	s.NoError(err)
	s.Equal(1, i)

	i, err = parseValue("29", 1, 30, parseModeInt)
	s.NoError(err)
	s.Equal(29, i)

	i, err = parseValue("29", 1, 12, parseModeInt)
	s.Error(err)

	i, err = parseValue("random text", 1, 31, parseModeInt)
	s.Error(err)

	i, err = parseValue("fri", 0, 7, parseModeDow)
	s.NoError(err)
	s.Equal(5, i)

	i, err = parseValue("August", 1, 12, parseModeMonth)
	s.NoError(err)
	s.Equal(8, i)
}

func (s *calendarSuite) TestDaysInMonth() {
	s.Equal(31, daysInMonth(time.January, 2022))
	s.Equal(28, daysInMonth(time.February, 2022))
	s.Equal(29, daysInMonth(time.February, 2024))
	s.Equal(29, daysInMonth(time.February, 2000))
	s.Equal(28, daysInMonth(time.February, 2100))
	s.Equal(31, daysInMonth(time.March, 2022))
	s.Equal(30, daysInMonth(time.April, 2022))
	s.Equal(31, daysInMonth(time.May, 2022))
	s.Equal(30, daysInMonth(time.June, 2022))
	s.Equal(31, daysInMonth(time.July, 2022))
	s.Equal(31, daysInMonth(time.August, 2022))
	s.Equal(30, daysInMonth(time.September, 2022))
	s.Equal(31, daysInMonth(time.October, 2022))
	s.Equal(30, daysInMonth(time.November, 2022))
	s.Equal(31, daysInMonth(time.December, 2022))
}

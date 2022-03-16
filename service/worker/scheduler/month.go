package scheduler

import "time"

// FIXME: partially copied from go/src/time/time.go

func isLeap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

func daysInMonth(m time.Month, year int) int {
	if m == time.February {
		if isLeap(year) {
			return 29
		} else {
			return 28
		}
	}
	const bits = 0b1010110101010
	return 30 + (bits>>m)&1
}

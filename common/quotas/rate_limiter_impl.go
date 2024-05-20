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

package quotas

import (
	"time"

	"go.temporal.io/server/common/clock"
	"golang.org/x/time/rate"
)

type (
	// RateLimiterImpl is a wrapper around the golang rate limiter
	RateLimiterImpl struct {
		timeSource clock.TimeSource
		ClockedRateLimiter
	}
)

var _ RateLimiter = (*RateLimiterImpl)(nil)

// NewRateLimiter returns a new rate limiter that can handle dynamic
// configuration updates
func NewRateLimiter(newRPS float64, newBurst int) *RateLimiterImpl {
	limiter := rate.NewLimiter(rate.Limit(newRPS), newBurst)
	ts := clock.NewRealTimeSource()
	rl := &RateLimiterImpl{
		timeSource:         ts,
		ClockedRateLimiter: NewClockedRateLimiter(limiter, ts),
	}

	return rl
}

func (rl *RateLimiterImpl) Reserve() Reservation {
	return rl.ClockedRateLimiter.Reserve()
}

func (rl *RateLimiterImpl) ReserveN(now time.Time, n int) Reservation {
	return rl.ClockedRateLimiter.ReserveN(now, n)
}

// SetRateBurst set the rps & burst of the rate limiter
func (rl *RateLimiterImpl) SetRateBurst(rps float64, burst int) {
	now := rl.timeSource.Now()
	rl.SetLimitAt(now, rate.Limit(rps))
	rl.SetBurstAt(now, burst)
}

// Rate returns the rps for this rate limiter
func (rl *RateLimiterImpl) Rate() float64 {
	return rl.ClockedRateLimiter.Limit()
}

// Burst returns the burst for this rate limiter
func (rl *RateLimiterImpl) Burst() int {
	return rl.ClockedRateLimiter.Burst()
}

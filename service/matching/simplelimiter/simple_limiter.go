package simplelimiter

import "time"

// Limiter and Params implement a "GCRA" limiter.
// A Limiter is "ready" if its value is <= now (as unix nanos).
type Limiter int64 // ready time as unix nanos

type Params struct {
	Interval time.Duration // ideal task spacing interval, or 0 for no limit (infinite), or -1 for zero limit
	Burst    time.Duration // burst duration
}

const MaxBurst = time.Minute
const Never = Limiter(7 << 60) // this is in the year 2225

func NoLimitParams() Params {
	return Params{}
}

func MakeParams(rate float64, burstDuration time.Duration) Params {
	// 1e-9 would make interval overflow int64
	if rate <= 1e-9 {
		return Params{
			Interval: time.Duration(-1),
		}
	}
	return Params{
		Interval: time.Duration(float64(time.Second) / rate),
		Burst:    min(burstDuration, MaxBurst),
	}
}

func (p Params) Never() bool   { return p.Interval < 0 }
func (p Params) Limited() bool { return p.Interval > 0 }

// delay returns the time until the limiter is ready.
// If the return value is <= 0 then the limiter can go now.
func (ready Limiter) Delay(now int64) time.Duration {
	return time.Duration(int64(ready) - now)
}

// consume updates ready based on the current time and number of new tokens consumed.
func (ready Limiter) Consume(p Params, now int64, tokens int64) Limiter {
	// This is a slight variation of the normal GCRA: instead of tracking the end of the
	// allowed interval (the theoretical arrival time), ready tracks the beginning of it, and
	// the end is ready + burst. To find the next ready time:
	// - Add ready+burst to find the next theoretical arrival time.
	// - If that's in the past, clip it at the current time.
	// - Subtract burst to turn it back into a ready time.
	// - Finally add the tokens we used.
	//
	// For intuition, consider that if if now is > ready by only a tiny amount, i.e. we're
	// bursting, then the max takes ready+burst and we push up the ready time by the full
	// interval. We can do this burst/interval times before it catches up and we're no longer
	// ready.
	//
	// Alternatively, if now is > ready by more than burst, then we end up subtracting the full
	// burst from now and adding one interval.
	if p.Never() {
		return Never
	}
	clippedReady := max(now, int64(ready)+p.Burst.Nanoseconds()) - p.Burst.Nanoseconds()
	return Limiter(clippedReady + tokens*p.Interval.Nanoseconds())
}

// clip updates ready to an allowable range based on the given parameters.
func (ready Limiter) Clip(p Params, now int64, maxTokens int64) Limiter {
	if p.Never() {
		return Never
	}
	// If ready was set very far in the future (e.g. because the rate was zero), then we can
	// clip it back to now + maxTokens*interval + burst.
	maxDelay := maxTokens*p.Interval.Nanoseconds() + p.Burst.Nanoseconds()
	return min(ready, Limiter(now+maxDelay))
}

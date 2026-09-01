package fc

import "time"

// While we're still using local rate limits through rateLimitManager, we need a way to bridge
// the flow control interface with the old rateLimitManager. This lets us pass something
// through Limiter.Config to call the local rate limiter.
type LocalLimiter interface {
	// > 0 Delay means will be ready in that much time, <= 0 means ready now
	Delay() time.Duration
	// Consume or return tokens
	Consume(int)
}

type fcTask interface {
	Limiters() *Limiters
}

type ReadinessCallback interface {
	OnReady()
}

package fc

import (
	"time"

	enumsspb "go.temporal.io/server/api/enums/v1"
)

// LimiterSource is used to recognize limiters from config sources so that they can be properly
// removed or updated when the config changes.
type LimiterSource int32

const (
	// not valid limiter
	LimiterSourceInvalid LimiterSource = iota
	// limiter came from task queue config
	LimiterSourceConfig
	// future: per-task, namespace policy, etc.
)

// Limiter identifies one flow control limiter, with optional configuration.
type Limiter struct {
	Key    string
	Type   enumsspb.LimiterType
	Source LimiterSource
	// for limiters set from config, we need to pass through to Reserve
	Config        any
	ConfigVersion int64
}

func (lim Limiter) Valid() bool {
	return lim.Type != enumsspb.LIMITER_TYPE_UNSPECIFIED && lim.Source != LimiterSourceInvalid
}

// matching.internalTask owns a *Limiters
type Limiters struct {
	Limiters [MaxLimiters]Limiter
}

// fcTask is the interface that flow control needs from a task (just get its limiters).
type fcTask interface {
	Limiters() *Limiters
}

// ReadinessState is our best guess at whether a limiter will return success to a Reserve call.
type ReadinessState int32

const (
	ReadinessUnknown ReadinessState = iota
	ReadinessBlocked
	ReadinessReady
)

// Likely indicats whether we should treat a state as "likely for Reserve to succeed": we
// optimistically try to Reserve limiters that we don't have cached state for, so we learn
// whether they're ready.
func (s ReadinessState) Likely() bool {
	return s == ReadinessUnknown || s == ReadinessReady
}

// ReadinessCallback is something we can notify when we think the readiness state of a limiter
// may have changed.
// Note: because of how we store callbacks, the concrete type implementing ReadinessCallback
// _must_ be a pointer type. Currently it's always *matcherData (except in tests).
type ReadinessCallback interface {
	OnReady()
}

// While we're still using local rate limits through rateLimitManager, we need a way to bridge
// the flow control interface with the old rateLimitManager. This lets us pass something
// through Limiter.Config to call the local rate limiter.
type LocalLimiter interface {
	// > 0 Delay means will be ready in that much time, <= 0 means ready now
	Delay() time.Duration
	// Consume or return tokens
	Consume(int)
}

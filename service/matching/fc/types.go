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

// Limiter identifies one flow control limiter attached to a task, with optional configuration.
// Limiter is implicitly scoped to one namespace by the task that it's attached to.
type Limiter struct {
	Key           string
	Type          enumsspb.LimiterType
	Source        LimiterSource
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

// fcTask is the interface that flow control needs from a task.
// *matching.internalTask implements this.
type fcTask interface {
	Limiters() *Limiters
	PriorityAndAge() (int32, time.Time) // TODO(fc): consider moving to ReadinessCallback
}

// ReadinessCallback is something we can notify when we think the readiness state of a limiter
// may have changed.
// Note: because of how we store callbacks, the concrete type implementing ReadinessCallback
// _must_ be a pointer type. Currently it's always *matcherData (except in tests).
type ReadinessCallback interface {
	OnReady()
}

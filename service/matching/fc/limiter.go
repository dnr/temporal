package fc

import enumsspb "go.temporal.io/server/api/enums/v1"

type LimiterSource int32

const (
	// not valid limiter
	LimiterSourceInvalid LimiterSource = iota
	// limiter came from task queue config, applies to the whole queue
	LimiterSourceConfig_WholeQueue
	// limiter came from task queue config, per-fairness-key limit
	LimiterSourceConfig_Fairness
	// future: namespace policy, etc.
)

type Limiter struct {
	Key    string
	Type   enumsspb.LimiterType
	Source LimiterSource
	// for limiters set from Config, we need to pass through to Reserve
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

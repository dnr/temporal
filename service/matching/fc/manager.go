package fc

import (
	"errors"
	"slices"

	enumsspb "go.temporal.io/server/api/enums/v1"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/tqid"
)

type limiter struct {
	key    string // TODO(fc): consider interning? or intern whole limiter?
	tp     enumsspb.LimiterType
	source limiterSource
}

func (lim limiter) valid() bool {
	return lim.tp != enumsspb.LIMITER_TYPE_UNSPECIFIED && lim.source != limiterSourceInvalid
}

// matching.internalTask owns a *Limiters
type Limiters struct {
	limiters [maxLimiters]limiter
}

type Manager struct {
	partition         tqid.Partition
	userDataManager   userDataManager
	readiness         *Readiness
	wholeQueueLimiter string
}

func NewManager(
	partition tqid.Partition,
	userDataManager userDataManager,
	readiness *Readiness,
) *Manager {
	tqName := partition.TaskQueue().Name()
	tqType := partition.TaskType()
	return &Manager{
		partition:         partition,
		userDataManager:   userDataManager,
		readiness:         readiness,
		wholeQueueLimiter: wholeQueueLimiterName(tqName, tqType),
	}
}

func (m *Manager) WholeQueueLikely(cb readinessCallback) bool {
	nsID := namespace.ID(m.partition.NamespaceId())
	state := m.readiness.ReadinessState(nsID, enumsspb.LIMITER_TYPE_CONCURRENCY, m.wholeQueueLimiter, cb)
	return state.Likely()
}

func (m *Manager) CancelWholeQueueCallback(cb readinessCallback) {
	nsID := namespace.ID(m.partition.NamespaceId())
	m.readiness.CancelCallback(nsID, enumsspb.LIMITER_TYPE_CONCURRENCY, m.wholeQueueLimiter, cb)
}

func (m *Manager) UpdateLimitersFromConfig(limiters *Limiters) (*Limiters, error) {
	userData, _, err := m.userDataManager.GetUserData()
	if err != nil {
		return nil, err
	}
	tqType := m.partition.TaskType()
	limit := userData.GetData().GetPerType()[int32(tqType)].GetConfig().GetQueueConcurrencyLimit().GetConcurrencyLimit()
	if limit == nil {
		// there is no limit. clear if it was set already.
		if limiters != nil {
			_ = slices.DeleteFunc(limiters.limiters[:], func(lim limiter) bool {
				return lim.source == limiterSourceConfig_WholeQueue
			})
		}
		return limiters, nil
	}
	// need to add limiter
	if limiters == nil {
		limiters = &Limiters{}
	}
	for i, lim := range limiters.limiters[:] {
		if !lim.valid() {
			// add it in empty slot
			limiters.limiters[i] = limiter{
				key:    m.wholeQueueLimiter,
				tp:     enumsspb.LIMITER_TYPE_CONCURRENCY,
				source: limiterSourceConfig_WholeQueue,
			}
			return limiters, nil
		} else if lim.source == limiterSourceConfig_WholeQueue {
			// we found one we previously set
			return limiters, nil
		}
	}
	return nil, errors.New("too many limiters") // FIXME: proper type
}

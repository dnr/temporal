package fc

import (
	"slices"
	"time"

	enumsspb "go.temporal.io/server/api/enums/v1"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/tqid"
)

type limiter struct {
	key    string
	tp     enumsspb.LimiterType
	source limiterSource
	// for limiters set from config, we need to pass through to Reserve
	config        any
	configVersion int64
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

func (m *Manager) WholeQueueLikely(pri int32, age time.Time, cb readinessCallback) bool {
	nsID := namespace.ID(m.partition.NamespaceId())
	state := m.readiness.ReadinessState(nsID, enumsspb.LIMITER_TYPE_CONCURRENCY, m.wholeQueueLimiter, pri, age, cb)
	return state.Likely()
}

func (m *Manager) CancelWholeQueueCallback(cb readinessCallback) {
	nsID := namespace.ID(m.partition.NamespaceId())
	m.readiness.CancelCallback(nsID, enumsspb.LIMITER_TYPE_CONCURRENCY, m.wholeQueueLimiter, cb)
}

func (m *Manager) UpdateLimitersFromConfig(limiters *Limiters) *Limiters {
	userData, _, err := m.userDataManager.GetUserData()
	if err != nil {
		return nil
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
		return limiters
	}
	// need to add limiter
	if limiters == nil {
		limiters = &Limiters{}
	}
	for i, lim := range limiters.limiters[:] {
		if !lim.valid() {
			// add it in empty slot
			limiters.limiters[i] = limiter{
				key:           m.wholeQueueLimiter,
				tp:            enumsspb.LIMITER_TYPE_CONCURRENCY,
				source:        limiterSourceConfig_WholeQueue,
				config:        limit,
				configVersion: userData.GetVersion(),
			}
			return limiters
		} else if lim.source == limiterSourceConfig_WholeQueue {
			// we found one we previously set, update config
			lim.config = limit
			lim.configVersion = userData.GetVersion()
			return limiters
		}
	}
	// We get here if we already have three per-task limiters set and we also try to add a
	// whole-queue limiter. This should be an error, but it's quite awkward to handle an error
	// at our call sites. Even if we could "handle" the error, the behavior would be to either
	// drop the task or to block it forever, both of which are not good. Ideally we should
	// detect and surface this at a higher level. For now, at this level, we just ignore the
	// whole-queue limiter.
	// TODO(fc): surface this error at a higher level somehow
	return limiters
}

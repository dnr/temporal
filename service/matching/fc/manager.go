package fc

import (
	"fmt"
	"slices"
	"time"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
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
	rateLimitManager  rateLimitManager
	readiness         *Readiness
	wholeQueueLimiter string
}

func NewManager(
	partition tqid.Partition,
	userDataManager userDataManager,
	rateLimitManager rateLimitManager,
	readiness *Readiness,
) *Manager {
	tqName := partition.TaskQueue().Name()
	tqType := partition.TaskType()
	return &Manager{
		partition:         partition,
		userDataManager:   userDataManager,
		rateLimitManager:  rateLimitManager,
		readiness:         readiness,
		wholeQueueLimiter: m.wholeQueueLimiterName(tqName, tqType),
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

func (m *Manager) UpdateLimitersFromConfig(limiters *Limiters, fkey string) *Limiters {
	userData, _, err := m.userDataManager.GetUserData()
	if err != nil {
		return nil
	}
	tqType := m.partition.TaskType()
	cfg := userData.GetData().GetPerType()[int32(tqType)].GetConfig()
	cfgVersion := userData.GetVersion()
	limiters = m.updateWholeQueueConcurrencyLimiter(cfg, cfgVersion, limiters)
	limiters = m.updateWholeQueueLocalRateLimiter(cfg, cfgVersion, limiters)
	limiters = m.updateWholeQueueLocalFKeyRateLimiter(cfg, cfgVersion, limiters, fkey)
	return limiters
}

func (m *Manager) updateWholeQueueConcurrencyLimiter(cfg *taskqueuepb.TaskQueueConfig, cfgVersion int64, limiters *Limiters) *Limiters {
	var lim limiter
	if limit := cfg.GetQueueConcurrencyLimit().GetConcurrencyLimit(); limit != nil {
		lim = limiter{
			key:           m.wholeQueueLimiter,
			tp:            enumsspb.LIMITER_TYPE_CONCURRENCY,
			source:        limiterSourceConfig_WholeQueue,
			config:        limit,
			configVersion: cfgVersion,
		}
	}
	return m.addOrUpdateLimiter(lim, limiters)
}

func (m *Manager) updateWholeQueueLocalRateLimiter(cfg *taskqueuepb.TaskQueueConfig, cfgVersion int64, limiters *Limiters) *Limiters {
	var lim limiter
	if limit := cfg.GetQueueRateLimit().GetRateLimit(); limit != nil {
		lim = limiter{
			key:           m.localRateLimiterName(),
			tp:            enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
			source:        limiterSourceConfig_WholeQueue,
			config:        limit,
			configVersion: cfgVersion,
		}
	}
	return m.addOrUpdateLimiter(lim, limiters)
}

func (m *Manager) updateWholeQueueLocalFKeyRateLimiter(cfg *taskqueuepb.TaskQueueConfig, cfgVersion int64, limiters *Limiters, fkey string) *Limiters {
	var lim limiter
	if limit := cfg.GetQueueRateLimit().GetRateLimit(); limit != nil {
		lim = limiter{
			key:           m.localFKeyRateLimiterName(),
			tp:            enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
			source:        limiterSourceConfig_WholeQueue,
			config:        limit,
			configVersion: cfgVersion,
		}
	}
	return m.addOrUpdateLimiter(lim, limiters)
}

func (*Manager) addOrUpdateLimiter(newLim limiter, limiters *Limiters) *Limiters {
	match := func(l limiter) bool {
		return l.key == newLim.key && l.tp == newLim.tp && l.source == newLim.source
	}
	if !newLim.valid() {
		// need to remove
		if limiters != nil {
			_ = slices.DeleteFunc(limiters.limiters[:], match)
		}
		return limiters
	}
	// need to add
	if limiters == nil {
		limiters = &Limiters{}
	}
	for i, lim := range limiters.limiters[:] {
		if !lim.valid() {
			// add it in empty slot
			limiters.limiters[i] = newLim
			return limiters
		} else if match(lim) {
			// we found the one we previously set, update config
			lim.config = newLim.config
			lim.configVersion = newLim.configVersion
			return limiters
		}
	}
	// We get here if we already have three per-task limiters set and we also try to add
	// another from config. This should be an error, but it's quite awkward to handle an error
	// at our call sites. Even if we could "handle" the error, the behavior would be to either
	// drop the task or to block it forever, both of which are not good. Ideally we should
	// detect and surface this at a higher level. For now, at this level, we just ignore the
	// whole-queue limiter.
	// TODO(fc): surface this error at a higher level somehow
	return limiters
}

func (m *Manager) wholeQueueLimiterName(tqName string, tqType enumspb.TaskQueueType) string {
	// the "/0" at the end is for future extension for partitioning limiters
	return fmt.Sprintf("wholequeue/%s/%d/0", tqName, tqType)
}

func (m *Manager) localRateLimiterName(tqName string, tqType enumspb.TaskQueueType) string {
	return fmt.Sprintf("tq/%s/%d", tqName, tqType)
}

func (m *Manager) localFKeyRateLimiterName(tqName string, tqType enumspb.TaskQueueType, fkey string) string {
	return fmt.Sprintf("fkey/%s/%d/%s", tqName, tqType, fkey)
}

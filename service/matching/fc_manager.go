package matching

import (
	"fmt"
	"slices"
	"time"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/tqid"
	"go.temporal.io/server/service/matching/fc"
)

type fcManager struct {
	partition        tqid.Partition
	userDataManager  userDataManager
	rateLimitManager *rateLimitManager
	readiness        *fc.Readiness
}

func newFCManager(
	partition tqid.Partition,
	userDataManager userDataManager,
	rateLimitManager *rateLimitManager,
	readiness *fc.Readiness,
) *fcManager {
	return &fcManager{
		partition:        partition,
		userDataManager:  userDataManager,
		rateLimitManager: rateLimitManager,
		readiness:        readiness,
	}
}

func (m *fcManager) WholeQueueLikely(pri int32, age time.Time, cb fc.ReadinessCallback) bool {
	nsID := namespace.ID(m.partition.NamespaceId())
	key := m.wholeQueueLimiterName()
	state := m.readiness.ReadinessState(nsID, enumsspb.LIMITER_TYPE_CONCURRENCY, key, pri, age, cb)
	return state.Likely()
}

func (m *fcManager) CancelWholeQueueCallback(cb fc.ReadinessCallback) {
	nsID := namespace.ID(m.partition.NamespaceId())
	key := m.wholeQueueLimiterName()
	m.readiness.CancelCallback(nsID, enumsspb.LIMITER_TYPE_CONCURRENCY, key, cb)
}

func (m *fcManager) UpdateLimitersFromConfig(limiters *fc.Limiters, fkey string) *fc.Limiters {
	userData, _, err := m.userDataManager.GetUserData()
	if err != nil {
		return nil
	}
	tqType := m.partition.TaskType()
	cfg := userData.GetData().GetPerType()[int32(tqType)].GetConfig()
	cfgVersion := userData.GetVersion()
	limiters = m.updateWholeQueueConcurrencyLimiter(cfg, cfgVersion, limiters)
	limiters = m.updateLocalRateLimiter(cfg, cfgVersion, limiters)
	limiters = m.updateLocalFKeyRateLimiter(cfg, cfgVersion, limiters, fkey)
	return limiters
}

func (m *fcManager) updateWholeQueueConcurrencyLimiter(cfg *taskqueuepb.TaskQueueConfig, cfgVersion int64, limiters *fc.Limiters) *fc.Limiters {
	lim := fc.Limiter{
		Type:   enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:    m.wholeQueueLimiterName(),
		Source: fc.LimiterSourceConfig_WholeQueue,
	}
	if limit := cfg.GetQueueConcurrencyLimit().GetConcurrencyLimit(); limit != nil {
		lim.Config = limit
		lim.ConfigVersion = cfgVersion
		return m.addOrUpdateLimiter(lim, limiters)
	}
	return m.removeLimiter(lim, limiters)
}

func (m *fcManager) updateLocalRateLimiter(cfg *taskqueuepb.TaskQueueConfig, cfgVersion int64, limiters *fc.Limiters) *fc.Limiters {
	lim := fc.Limiter{
		Type:   enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
		Key:    m.localRateLimiterName(),
		Source: fc.LimiterSourceConfig_WholeQueue,
	}
	if limit := cfg.GetQueueRateLimit().GetRateLimit(); limit != nil {
		lim.Config = limit
		lim.ConfigVersion = cfgVersion
		return m.addOrUpdateLimiter(lim, limiters)
	}
	return m.removeLimiter(lim, limiters)
}

func (m *fcManager) updateLocalFKeyRateLimiter(cfg *taskqueuepb.TaskQueueConfig, cfgVersion int64, limiters *fc.Limiters, fkey string) *fc.Limiters {
	lim := fc.Limiter{
		Type:   enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
		Key:    m.localFKeyRateLimiterName(fkey),
		Source: fc.LimiterSourceConfig_WholeQueue,
	}
	if limit := cfg.GetFairnessKeysRateLimitDefault().GetRateLimit(); limit != nil {
		lim.Config = limit
		lim.ConfigVersion = cfgVersion
		return m.addOrUpdateLimiter(lim, limiters)
	}
	return m.removeLimiter(lim, limiters)
}

func (*fcManager) addOrUpdateLimiter(newLim fc.Limiter, limiters *fc.Limiters) *fc.Limiters {
	match := func(l fc.Limiter) bool {
		return l.Key == newLim.Key && l.Type == newLim.Type && l.Source == newLim.Source
	}
	if limiters == nil {
		limiters = &fc.Limiters{}
	}
	for i, lim := range limiters.Limiters[:] {
		if !lim.Valid() {
			// add it in empty slot
			limiters.Limiters[i] = newLim
			return limiters
		} else if match(lim) {
			// we found the one we previously set, update config
			lim.Config = newLim.Config
			lim.ConfigVersion = newLim.ConfigVersion
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

func (*fcManager) removeLimiter(oldLim fc.Limiter, limiters *fc.Limiters) *fc.Limiters {
	match := func(l fc.Limiter) bool {
		return l.Key == oldLim.Key && l.Type == oldLim.Type && l.Source == oldLim.Source
	}
	if limiters != nil {
		_ = slices.DeleteFunc(limiters.Limiters[:], match)
	}
	return limiters
}

func (m *fcManager) wholeQueueLimiterName() string {
	// the "/0" at the end is for future extension for partitioning limiters
	tqName := m.partition.TaskQueue().Name()
	tqType := m.partition.TaskType()
	return fmt.Sprintf("wholequeue/%s/%d/0", tqName, tqType)
}

func (m *fcManager) localRateLimiterName() string {
	tqName := m.partition.TaskQueue().Name()
	tqType := m.partition.TaskType()
	partitionId := 0
	if normal, ok := m.partition.(*tqid.NormalPartition); ok {
		partitionId = normal.PartitionId()
	}
	return fmt.Sprintf("tq/%s/%d/%d", tqName, tqType, partitionId)
}

func (m *fcManager) localFKeyRateLimiterName(fkey string) string {
	tqName := m.partition.TaskQueue().Name()
	tqType := m.partition.TaskType()
	partitionId := 0
	if normal, ok := m.partition.(*tqid.NormalPartition); ok {
		partitionId = normal.PartitionId()
	}
	return fmt.Sprintf("fkey/%s/%d/%d/%s", tqName, tqType, partitionId, fkey)
}

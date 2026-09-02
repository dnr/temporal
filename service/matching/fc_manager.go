package matching

import (
	"cmp"
	"fmt"
	"math"
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
	config           *taskQueueConfig
	userDataManager  userDataManager
	rateLimitManager *rateLimitManager
	readiness        *fc.Readiness
}

func newFCManager(
	partition tqid.Partition,
	config *taskQueueConfig,
	userDataManager userDataManager,
	rateLimitManager *rateLimitManager,
	readiness *fc.Readiness,
) *fcManager {
	return &fcManager{
		partition:        partition,
		config:           config,
		userDataManager:  userDataManager,
		rateLimitManager: rateLimitManager,
		readiness:        readiness,
	}
}

func (m *fcManager) TaskReady(task *internalTask, cb fc.ReadinessCallback) (ready bool, blockedBy syncMatchOutcome, canContinue bool) {
	ready = true
	limiters := task.Limiters()
	if limiters == nil {
		return
	}

	nsID := namespace.ID(m.partition.NamespaceId())
	pri := cmp.Or(task.getPriority().GetPriorityKey(), int32(m.config.DefaultPriorityKey))
	createTime := task.getCreateTime().AsTime()

	for i, lim := range limiters.Limiters[:] {
		if !lim.Valid() {
			continue
		}

		state := m.readiness.ReadinessState(nsID, lim, pri, createTime, cb)
		if state.Likely() {
			continue
		}

		// We're blocked here. The ones before this one are ready and ReadinessState
		// unsubscribed from them already. This one isn't and ReadinessState subscribed to it.
		// We need to unsubscribe from the following ones, in case we were already subscribed:
		for _, nlim := range limiters.Limiters[i+1:] {
			m.readiness.CancelCallback(nsID, nlim, cb)
		}

		ready = false
		blockedBy = limiterTypeToSyncMatchOutcome(lim.Type)
		canContinue = false // FIXME: set this based on "whole queue" scope, but allow fkey skipping
		return
	}

	return
}

func (m *fcManager) UpdateLimitersFromConfig(limiters *fc.Limiters, task *internalTask) *fc.Limiters {
	userData, _, err := m.userDataManager.GetUserData()
	if err != nil {
		return nil
	}
	tqType := m.partition.TaskType()
	cfg := userData.GetData().GetPerType()[int32(tqType)].GetConfig()
	cfgVersion := userData.GetVersion()
	limiters = m.updateWholeQueueConcurrencyLimiter(cfg, cfgVersion, limiters)
	limiters = m.updateLocalRateLimiter(cfg, cfgVersion, limiters, task)
	return limiters
}

func (m *fcManager) updateWholeQueueConcurrencyLimiter(cfg *taskqueuepb.TaskQueueConfig, cfgVersion int64, limiters *fc.Limiters) *fc.Limiters {
	lim := fc.Limiter{
		Type:   enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:    m.wholeQueueLimiterName(),
		Source: fc.LimiterSourceConfig,
	}
	if limit := cfg.GetQueueConcurrencyLimit().GetConcurrencyLimit(); limit != nil {
		lim.Config = limit
		lim.ConfigVersion = cfgVersion
		return m.addOrUpdateLimiter(lim, limiters)
	}
	return m.removeLimiter(lim, limiters)
}

func (m *fcManager) updateLocalRateLimiter(cfg *taskqueuepb.TaskQueueConfig, cfgVersion int64, limiters *fc.Limiters, task *internalTask) *fc.Limiters {
	lim := fc.Limiter{
		Type:   enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
		Source: fc.LimiterSourceConfig,
	}
	partitionRPS, fkeyRPS := m.rateLimitManager.GetPerPartitionRPS()
	// currently this condition is always true
	if partitionRPS < math.Inf(1) || fkeyRPS < math.Inf(1) {
		lim.Config = &rateLimiterBridge{
			rlm:  m.rateLimitManager,
			task: task,
		}
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

func limiterTypeToSyncMatchOutcome(tp enumsspb.LimiterType) syncMatchOutcome {
	switch tp {
	case enumsspb.LIMITER_TYPE_CONCURRENCY:
		return syncMatchConcurrencyLimited
	case enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT:
		return syncMatchRateLimited
	default:
		return syncMatchUnspecified
	}
}

type rateLimiterBridge struct {
	rlm  *rateLimitManager
	task *internalTask
}

var _ fc.LocalLimiter = (*rateLimiterBridge)(nil)

func (b *rateLimiterBridge) Delay() time.Duration {
	sl := b.rlm.readyTimeForTask(b.task)
	now := time.Now().UnixNano() // TODO(fc): timesource
	return sl.Delay(now)
}

func (b *rateLimiterBridge) Consume(tokens int) {
	now := time.Now().UnixNano() // TODO(fc): timesource
	b.rlm.consumeTokens(now, b.task, int64(tokens))
}

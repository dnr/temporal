package matching

import (
	"context"
	"errors"
	"fmt"
	"slices"

	commonpb "go.temporal.io/api/common/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/tqid"
	"go.temporal.io/server/service/matching/fc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
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

func (m *fcManager) TaskReady(task *internalTask, cb fc.ReadinessCallback) error {
	if task.isPollForwarder() {
		// Not a real task. Flow control applies to whatever task the forwarded poll matches
		// with on the other partition.
		task.fcTx = nil
		return nil
	}

	nsID := namespace.ID(m.partition.NamespaceId())
	tx := m.readiness.NewTx(nsID, task, cb)
	task.fcTx = tx // attach to task so we can use it in matching engine

	if task.isForwarded() {
		// The partition that forwarded this task already did Check (on the parentTaskForwarder
		// match), so we shouldn't block it again here. If we record it as started, we still do
		// Reserve and Commit, and pass hints back to the forwarding partition.
		return tx.SkipCheck()
	}
	return tx.Check()
}

func (m *fcManager) CancelAllCallbacks(cb fc.ReadinessCallback) {
	nsID := namespace.ID(m.partition.NamespaceId())
	m.readiness.CancelAllCallbacks(nsID, cb)
}

func (m *fcManager) UpdateLimitersFromConfig(limiters *fc.Limiters, task *internalTask) *fc.Limiters {
	if task.isPollForwarder() {
		return nil
	}
	userData, _, err := m.userDataManager.GetUserData()
	if err != nil {
		return nil
	}
	tqType := m.partition.TaskType()
	cfg := userData.GetData().GetPerType()[int32(tqType)].GetConfig()
	cfgVersion := userData.GetVersion()
	limiters = m.updateWholeQueueConcurrencyLimiter(cfg, cfgVersion, limiters)
	limiters = m.updateLocalRateLimiter(limiters)
	limiters = m.updateFairnessRateLimiter(limiters, task.getPriority())
	return limiters
}

func (m *fcManager) updateWholeQueueConcurrencyLimiter(cfg *taskqueuepb.TaskQueueConfig, cfgVersion int64, limiters *fc.Limiters) *fc.Limiters {
	lim := fc.Limiter{
		Type:   enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:    m.wholeQueueConcurrencyLimiterKey(),
		Source: fc.LimiterSourceConfig,
	}
	if limit := cfg.GetQueueConcurrencyLimit().GetConcurrencyLimit(); limit != nil {
		lim.Config = limit
		lim.ConfigVersion = cfgVersion
		return m.addOrUpdateLimiter(lim, limiters)
	}
	return m.removeLimiter(lim, limiters)
}

func (m *fcManager) updateLocalRateLimiter(limiters *fc.Limiters) *fc.Limiters {
	lim := fc.Limiter{
		Type:   enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
		Key:    m.localLimiterKey(),
		Source: fc.LimiterSourceConfig,
	}
	wholeQueueLimit := m.rateLimitManager.GetWholeQueueLimit()
	if !wholeQueueLimit.Limited() && !wholeQueueLimit.Never() {
		// currently there is always some whole queue limit, so this is unreachable
		return m.removeLimiter(lim, limiters)
	}
	lim.Config = wholeQueueLimit
	return m.addOrUpdateLimiter(lim, limiters)
}

func (m *fcManager) updateFairnessRateLimiter(limiters *fc.Limiters, pri *commonpb.Priority) *fc.Limiters {
	lim := fc.Limiter{
		Type:   enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
		Key:    m.localLimiterFairnessKey(pri.GetFairnessKey()),
		Source: fc.LimiterSourceConfig,
	}
	fkeyLimit := m.rateLimitManager.GetPerKeyLimit(pri)
	if !fkeyLimit.Limited() && !fkeyLimit.Never() {
		return m.removeLimiter(lim, limiters)
	}
	lim.Config = fkeyLimit
	return m.addOrUpdateLimiter(lim, limiters)
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
			limiters.Limiters[i].Config = newLim.Config
			limiters.Limiters[i].ConfigVersion = newLim.ConfigVersion
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

func (m *fcManager) wholeQueueConcurrencyLimiterKey() string {
	// the "/0" at the end is for future extension for sharding limiters
	return fmt.Sprintf("wholequeue/%s/%d/0", m.partition.TaskQueue().Name(), m.partition.TaskType())
}

func (m *fcManager) localLimiterKey() string {
	partition := 0
	if normal, ok := m.partition.(*tqid.NormalPartition); ok {
		partition = normal.PartitionId()
	}
	return fmt.Sprintf("lim/%s/%d/%d", m.partition.TaskQueue().Name(), partition, m.partition.TaskType())
}

func (m *fcManager) localLimiterFairnessKey(fkey string) string {
	partition := 0
	if normal, ok := m.partition.(*tqid.NormalPartition); ok {
		partition = normal.PartitionId()
	}
	return fmt.Sprintf("fklim/%s/%d/%d/%s", m.partition.TaskQueue().Name(), partition, m.partition.TaskType(), fkey)
}

func limiterErrorToSyncMatchOutcome(err error) syncMatchOutcome {
	switch {
	case errors.Is(err, fc.ErrConcurrencyBlocked):
		return syncMatchConcurrencyLimited
	case errors.Is(err, fc.ErrLocalLimiterBlocked):
		return syncMatchRateLimited
	default:
		return syncMatchUnspecified
	}
}

// The "-bin" suffix instructs grpc to base64-encode the value, so we can use binary.
const limiterHintsTrailerName = "fchints-bin"

// setLimiterHintsTrailer sends hints back to a partition that forwarded a task to us.
func setLimiterHintsTrailer(ctx context.Context, hints []*taskqueuespb.LimiterHint) error {
	if len(hints) == 0 {
		return nil
	}
	b, err := proto.Marshal(&taskqueuespb.LimiterHints{Hints: hints})
	if err != nil {
		return err
	}
	return grpc.SetTrailer(ctx, metadata.Pairs(limiterHintsTrailerName, string(b)))
}

// getLimiterHintsFromTrailer parses hints from a trailer received from a forwarding rpc.
// Hints are advisory, so malformed hints are treated as absent.
func getLimiterHintsFromTrailer(trailer metadata.MD) []*taskqueuespb.LimiterHint {
	vals := trailer.Get(limiterHintsTrailerName)
	if len(vals) == 0 {
		return nil
	}
	var hints taskqueuespb.LimiterHints
	if err := proto.Unmarshal([]byte(vals[0]), &hints); err != nil {
		return nil
	}
	return hints.GetHints()
}

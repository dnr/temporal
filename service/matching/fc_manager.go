package matching

import (
	"context"
	"errors"
	"fmt"
	"slices"

	commonpb "go.temporal.io/api/common/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/namespace"
	serviceerrors "go.temporal.io/server/common/serviceerror"
	"go.temporal.io/server/common/tqid"
	"go.temporal.io/server/service/matching/fc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fcManager struct {
	partition        tqid.Partition
	userDataManager  userDataManager
	rateLimitManager *rateLimitManager
	readiness        *fc.Readiness

	// cached limiter keys
	wholeQueueConcurrencyKey string
	localKey                 string
}

func newFCManager(
	partition tqid.Partition,
	userDataManager userDataManager,
	rateLimitManager *rateLimitManager,
	readiness *fc.Readiness,
) *fcManager {
	m := &fcManager{
		partition:        partition,
		userDataManager:  userDataManager,
		rateLimitManager: rateLimitManager,
		readiness:        readiness,
	}
	m.wholeQueueConcurrencyKey = m.wholeQueueConcurrencyLimiterKey()
	m.localKey = m.localLimiterKey()
	return m
}

func (m *fcManager) TaskReady(task *internalTask, cb fc.ReadinessCallback) error {
	nsID := namespace.ID(m.partition.NamespaceId())
	tx := m.readiness.NewTx(nsID, task, cb)

	var err error
	if task.isForwarded() {
		// The partition that forwarded this task already did Check (on the parentTaskForwarder
		// match), so we shouldn't block it again here. If we record it as started, we still do
		// Reserve and Commit, and pass hints back to the forwarding partition.
		err = tx.SkipCheck()
	} else {
		err = tx.Check()
	}
	if err != nil {
		// tx is closed on error. Don't keep it on the task, which may stay in the matcher for a
		// while, so that it and the limiter state it references can be garbage collected.
		tx = nil
	}
	task.fcTx = tx // attach to task so we can use it in matching engine
	return err
}

func (m *fcManager) CancelAllCallbacks(cb fc.ReadinessCallback) {
	nsID := namespace.ID(m.partition.NamespaceId())
	m.readiness.CancelAllCallbacks(nsID, cb)
}

func (m *fcManager) grantEagerTokens(priority *commonpb.Priority, requested int32) int32 {
	if requested <= 0 {
		return 0
	}
	task := newInternalTaskForSyncMatch(&persistencespb.TaskInfo{
		Priority:   priority,
		CreateTime: timestamppb.Now(),
	}, nil, 0, nil)
	err := task.updateLimitersFromConfig(m)
	if err != nil {
		return 0
	}
	if len(task.limiters) == 0 {
		return requested
	}
	// For now, any limiters besides local rate limits are not supported for eager dispatch.
	// TODO(fc): Support full flow control for eager dispatch (e.g. call Reserve here, return
	// limiter refs to history, history calls Commit after committing its own transaction)
	for _, lim := range task.limiters {
		if lim.Type != enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT {
			return 0
		}
	}
	nsID := namespace.ID(m.partition.NamespaceId())
	// TODO(fc): Consider: optimize this to Check a batch at once instead of once at a time?
	granted := int32(0)
	for ; granted < requested; granted++ {
		tx := m.readiness.NewTx(nsID, task, nil)
		if err := tx.Check(); err != nil {
			break
		}
		tx.Close()
	}
	return granted
}

func (m *fcManager) UpdateLimitersFromConfig(limiters []fc.Limiter, task *internalTask) ([]fc.Limiter, error) {
	if task.isPollForwarder() {
		return nil, nil
	}
	// TODO(fc): decide which limiters should apply to query and nexus tasks. Currently they get
	// the same ones as workflow/activity tasks, but their dispatch paths only do Check, not
	// Reserve/Commit/release (see TODO(fc) in matchingEngineImpl).
	userData, _, err := m.userDataManager.GetUserData()
	if err != nil {
		return nil, err
	}
	tqType := m.partition.TaskType()
	cfg := userData.GetData().GetPerType()[int32(tqType)].GetConfig()
	cfgVersion := userData.GetVersion()
	limiters = m.updateWholeQueueConcurrencyLimiter(cfg, cfgVersion, limiters)
	limiters = m.updateLocalRateLimiter(limiters)
	limiters = m.updateFairnessRateLimiter(limiters, task.getPriority())
	return limiters, nil
}

func (m *fcManager) updateWholeQueueConcurrencyLimiter(cfg *taskqueuepb.TaskQueueConfig, cfgVersion int64, limiters []fc.Limiter) []fc.Limiter {
	lim := fc.Limiter{
		Type:   enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:    m.wholeQueueConcurrencyKey,
		Source: fc.LimiterSourceConfig,
	}
	if limit := cfg.GetQueueConcurrencyLimit().GetConcurrencyLimit(); limit != nil {
		lim.Config = limit
		lim.ConfigVersion = cfgVersion
		return m.addOrUpdateLimiter(lim, limiters)
	}
	return m.removeLimiter(lim, limiters)
}

func (m *fcManager) updateLocalRateLimiter(limiters []fc.Limiter) []fc.Limiter {
	lim := fc.Limiter{
		Type:   enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
		Key:    m.localKey,
		Source: fc.LimiterSourceConfig,
	}
	wholeQueueLimit := m.rateLimitManager.GetWholeQueueLimit()
	if wholeQueueLimit == nil || !wholeQueueLimit.Limited() && !wholeQueueLimit.Never() {
		// currently there is always some whole queue limit (after Start), so this is unreachable
		return m.removeLimiter(lim, limiters)
	}
	lim.Config = wholeQueueLimit
	return m.addOrUpdateLimiter(lim, limiters)
}

func (m *fcManager) updateFairnessRateLimiter(limiters []fc.Limiter, pri *commonpb.Priority) []fc.Limiter {
	lim := fc.Limiter{
		Type:   enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
		Key:    m.localLimiterFairnessKey(pri.GetFairnessKey()),
		Source: fc.LimiterSourceConfig,
	}
	fkeyLimit := m.rateLimitManager.GetPerKeyLimit(pri)
	if !fkeyLimit.Limited() && !fkeyLimit.Never() {
		return m.removeLimiter(lim, limiters)
	}
	lim.Config = &fkeyLimit
	return m.addOrUpdateLimiter(lim, limiters)
}

func (*fcManager) addOrUpdateLimiter(newLim fc.Limiter, limiters []fc.Limiter) []fc.Limiter {
	match := func(l fc.Limiter) bool {
		return l.Key == newLim.Key && l.Type == newLim.Type && l.Source == newLim.Source
	}
	if i := slices.IndexFunc(limiters, match); i >= 0 {
		// we found the one we previously set, update config
		limiters[i].Config = newLim.Config
		limiters[i].ConfigVersion = newLim.ConfigVersion
		return limiters
	} else if len(limiters) >= fc.MaxLimiters {
		// We get here if we already have the maximum number of limiters set and we also try
		// to add another from config. This should be an error, but it's quite awkward to
		// handle an error at our call sites. Even if we could "handle" the error, the behavior
		// would be to either drop the task or to block it forever, both of which are not
		// good. Ideally we should detect and surface this at a higher level. For now, at this
		// level, we just ignore the new limiter.
		// TODO(fc): surface this error at a higher level somehow
		return limiters
	}
	return append(limiters, newLim)
}

func (*fcManager) removeLimiter(oldLim fc.Limiter, limiters []fc.Limiter) []fc.Limiter {
	match := func(l fc.Limiter) bool {
		return l.Key == oldLim.Key && l.Type == oldLim.Type && l.Source == oldLim.Source
	}
	return slices.DeleteFunc(limiters, match)
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

// isFlowControlBlocked returns true if err is a Reserve failure from a concurrency limiter. A
// task that fails Reserve should go back to the matcher: the Reserve response already updated
// the readiness cache, so it will block in Check until the limiter has slots again.
func isFlowControlBlocked(err error) bool {
	_, ok := errors.AsType[*serviceerrors.FlowControlBlocked](err)
	return ok
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

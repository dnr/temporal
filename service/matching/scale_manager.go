package matching

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/server/api/matchingservice/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/goro"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/number"
	"go.temporal.io/server/common/quotas"
	"go.temporal.io/server/common/tqid"
	"google.golang.org/protobuf/proto"
)

// scaleManager keeps some state and manages the interaction with partitionScaler.
// scaleManager runs on the root partition only.
type scaleManager struct {
	partition       tqid.Partition
	logger          log.Logger
	metricsHandler  metrics.Handler
	userDataManager userDataManager
	matchingClient  matchingservice.MatchingServiceClient
	partitionScaler PartitionScaler
	// for simplicity, settings are fixed at construction time
	settings           dynamicconfig.PartitionScaleManagerSettings
	getWritePartitions dynamicconfig.IntPropertyFn
	emitGaugeMetrics   dynamicconfig.BoolPropertyFn
	background         *goro.Handle
	limiter            quotas.RateLimiter

	lock       sync.Mutex
	scaleState *persistencespb.PartitionScaleState
	scaleDB    scaleDB

	batch atomic.Int64
}

// scaleDB is used to write scale state to persistence. It's a sub-interface of
// physicalTaskQueueManager (for the default queue).
type scaleDB interface {
	UpdateScaleState(*persistencespb.PartitionScaleState, bool) error
}

func newScaleManager(
	baseCtx context.Context,
	partition tqid.Partition,
	logger log.Logger,
	metricsHandler metrics.Handler,
	userDataManager userDataManager,
	matchingClient matchingservice.MatchingServiceClient,
	partitionScaler PartitionScaler,
	settings dynamicconfig.PartitionScaleManagerSettings,
	getWritePartitions dynamicconfig.IntPropertyFn,
	emitGaugeMetrics dynamicconfig.BoolPropertyFn,
) *scaleManager {
	return &scaleManager{
		partition:          partition,
		logger:             log.With(logger, tag.ComponentPartitionScaler),
		metricsHandler:     metricsHandler,
		userDataManager:    userDataManager,
		matchingClient:     matchingClient,
		partitionScaler:    partitionScaler,
		settings:           settings,
		getWritePartitions: getWritePartitions,
		emitGaugeMetrics:   emitGaugeMetrics,
		background:         goro.NewHandle(baseCtx),
		limiter:            quotas.NewRateLimiter(float64(settings.MaxRate), 1),
	}
}

func (sm *scaleManager) Stop() {
	if sm == nil {
		return
	}
	sm.background.Cancel()
	sm.partitionScaler.Stop()
	if sm.emitGaugeMetrics() {
		// this is unfortunate but at least allows max() across pods to get the right value
		metrics.PartitionScaleRead.With(sm.metricsHandler).Record(float64(-1))
		metrics.PartitionScaleWrite.With(sm.metricsHandler).Record(float64(-1))
	}
}

// LoadedMetadata is called when the root partitions's default queue has loaded its metadata.
func (sm *scaleManager) LoadedMetadata(scaleState *persistencespb.PartitionScaleState, scaleDB scaleDB) {
	if sm == nil {
		return
	}

	sm.lock.Lock()
	defer sm.lock.Unlock()

	sm.scaleDB = scaleDB
	sm.setStateLocked(scaleState)

	sm.background.Go(sm.backgroundWork)
}

// AddedTasks is called on a batch of tasks added.
// This is called in the task add path, so it shouldn't block.
func (sm *scaleManager) AddedTasks(numTasks int) {
	if sm == nil {
		return
	}

	// scale target batch size by numTasks (since numTasks is scaled by partitions)
	batchSize := int64(numTasks) * int64(sm.settings.BatchSize)

	tasks := sm.batch.Add(int64(numTasks))
	if tasks < batchSize {
		return // not enough for a batch yet
	}

	if !sm.lock.TryLock() {
		return // don't block if something else is updating scaler
	}
	sm.callScalerLockedAndRelease()
}

// callScalerLockedAndRelease calls the scaling algorithm with a batch of tasks. If it
// indicates a change and the change is allowed, it performs the change (asynchronously).
//
// Note that this function expects sm.lock to be held on entry, and it unlocks it either
// synchronously or asynchronously. The caller must not unlock.
//
// This is called in the task add path, so it shouldn't block.
func (sm *scaleManager) callScalerLockedAndRelease() {
	tasks := int(sm.batch.Swap(0))

	decision := sm.partitionScaler.OnTasks(PartitionScalerInput{
		NumTasks:      tasks,
		CurrentTarget: int(sm.scaleState.GetTarget()),
		BacklogCounts: sm.scaleState.GetBacklogCounts(),
		PrivateState:  sm.scaleState.GetPrivateScalerState(),
	})
	if decision.NoChange || // explicit "no change"
		decision.NewTarget == int(sm.scaleState.GetTarget()) &&
			number.EncodeCompact8(int64(decision.BacklogCap)) == number.Compact8(scaleState.GetBacklogCap()) || // no actual change
		sm.scaleDB == nil || // not initialized yet
		!sm.limiter.Allow() { // rate limited
		sm.lock.Unlock()
		return
	}

	// Do the rest async so that we don't block the task add.
	// Note that we unlock sm.lock in the new goroutine.
	go func() {
		defer sm.lock.Unlock()

		target := int32(decision.NewTarget)

		newState := common.CloneProto(sm.scaleState)
		if newState == nil {
			newState = &persistencespb.PartitionScaleState{}
		}
		prevTarget := newState.Target
		newState.Target = target
		newState.MaxTarget = max(newState.MaxTarget, target)
		newState.TargetVersion = time.Now().UnixNano()
		newState.BacklogCounts = scaleState.GetBacklogCounts()
		newState.BacklogCap = int32(backlogCapC8)
		newState.PrivateScalerState = decision.PrivateState

		mayHaveBacklog := target
		if prevTarget == 0 {
			// Turning on managed partition scaling: consider all partitions from dynamic
			// config as having backlog also.
			mayHaveBacklog = max(mayHaveBacklog, int32(sm.getWritePartitions()))
		}
		for i := range mayHaveBacklog {
			newState.BacklogState = bitSet(newState.BacklogState).set(i)
		}

		// we must succesfully write to the db before making new state active
		if err := sm.scaleDB.UpdateScaleState(newState, true); err != nil {
			sm.logger.Error("failed to update state", tag.Error(err), tag.Operation("scale"))
			return
		}

		sm.logger.Info("new target",
			tag.Int32("target", target),
			tag.Int32("prev-target", prevTarget),
			tag.Int32("max-target", newState.MaxTarget))
		metrics.PartitionScaleEvents.With(sm.metricsHandler).Record(1)

		sm.setStateLocked(newState)
	}()
}

// setStateLocked updates the current scale state and syncs it to ephemeral data.
// This should only be called _after_ the state is persisted to the db.
func (sm *scaleManager) setStateLocked(newState *persistencespb.PartitionScaleState) {
	prevInfo := scaleStateToInfo(sm.scaleState)

	sm.scaleState = newState

	newInfo := scaleStateToInfo(sm.scaleState)

	// only push ephemeral data if _info_ changed, not on any state change
	if !proto.Equal(prevInfo, newInfo) {
		sm.userDataManager.SetPartitionScale(newInfo)
	}

	if sm.emitGaugeMetrics() {
		metrics.PartitionScaleRead.With(sm.metricsHandler).Record(float64(newInfo.Read))
		metrics.PartitionScaleWrite.With(sm.metricsHandler).Record(float64(newInfo.Write))
	}
}

func (sm *scaleManager) backgroundWork(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-time.After(backoff.Jitter(sm.settings.BackgroundInterval, 0.05)):
			sm.lock.Lock()
			scaleState := sm.scaleState

			sm.callScalerLockedAndRelease()
			// note that sm.lock may or may not be locked here. assume that it's not.

			// query all child partitions for backlog counts and drain state
			sm.updateBacklogAndDrainState(ctx, scaleState)
		}
	}
}

func (sm *scaleManager) describeRequest(id int32) *matchingservice.DescribeTaskQueuePartitionRequest {
	return &matchingservice.DescribeTaskQueuePartitionRequest{
		NamespaceId: sm.partition.NamespaceId(),
		TaskQueuePartition: &taskqueuespb.TaskQueuePartition{
			TaskQueue:     sm.partition.TaskQueue().Name(),
			TaskQueueType: sm.partition.TaskType(),
			PartitionId:   &taskqueuespb.TaskQueuePartition_NormalPartitionId{NormalPartitionId: id},
		},
		Versions: &taskqueuepb.TaskQueueVersionSelection{
			Unversioned: true,
			// TODO: what about "inactive" versions?
			AllActive: true,
		},
		ReportInternalTaskQueueStatus: true,
	}
}

func (sm *scaleManager) updateBacklogAndDrainState(ctx context.Context, scaleState *persistencespb.PartitionScaleState) {
	read := scaleStateToReadCount(scaleState)
	if read == 0 {
		return
	}

	prevBacklog := scaleState.GetBacklogCounts()
	newBacklog := make([]byte, read)
	backlogChanged := false

	// check if we should also evaluate drain state
	target := scaleState.GetTarget()
	checkDrain := target > 0 &&
		time.Since(time.Unix(0, scaleState.GetTargetVersion())) >= sm.settings.DrainBufferTime
	info := scaleStateToInfo(scaleState)
	var toClear []int32

	for id := range read {
		callCtx, cancel := context.WithTimeout(ctx, ioTimeout)
		res, err := sm.matchingClient.DescribeTaskQueuePartition(callCtx, sm.describeRequest(id))
		cancel()
		if err != nil {
			continue
		}

		// update backlog count
		total := totalBacklogFromDescribeResponse(res)
		var prev number.Compact8
		if id < int32(len(prevBacklog)) {
			prev = prevBacklog[id]
		}
		newBacklog[id] = number.UpdateCompact8(total, prev)
		backlogChanged = backlogChanged || newBacklog[id] != prev

		// check drain state for partitions in the draining range
		if checkDrain && id >= target && bitSet(scaleState.BacklogState).get(id) && partitionIsFullyDrained(res, info) {
			toClear = append(toClear, id)
		}
	}

	if !backlogChanged && len(toClear) == 0 {
		return
	}

	sm.lock.Lock()
	defer sm.lock.Unlock()

	if sm.scaleState != scaleState {
		return // we were operating from an old state
	}

	newState := common.CloneProto(scaleState)
	if newState == nil {
		newState = &persistencespb.PartitionScaleState{}
	}
	newState.BacklogCounts = newBacklog
	for _, i := range toClear {
		newState.BacklogState = bitSet(newState.BacklogState).clear(i)
	}

	// sync to DB only when drain bits changed (must be persisted before taking effect).
	// for backlog-count-only updates, update in-memory state only (will be persisted
	// periodically).
	needSync := len(toClear) > 0
	if err := sm.scaleDB.UpdateScaleState(newState, needSync); err != nil {
		sm.logger.Error("failed to update state", tag.Error(err), tag.Operation("drain"))
		return
	}

	if len(toClear) > 0 {
		sm.logger.Info("drain",
			tag.Any("drained-partitions", toClear),
			tag.Int32("target", info.Write),
			tag.Int32("prev-read", info.Read),
			tag.Int32("read", bitSet(newState.BacklogState).len()))
	}

	sm.setStateLocked(newState)
}

func partitionIsFullyDrained(
	res *matchingservice.DescribeTaskQueuePartitionResponse,
	info *taskqueuespb.PartitionScaleInfo,
) bool {
	// Require that the partition agrees with the current scale state, i.e. it knows that
	// it's draining, i.e. it knows it can't accept any new tasks. We include the version
	// as well as just the read+write counts to avoid an ABA problem.
	resInfo := res.GetScaleInfo()
	if resInfo == nil || resInfo.Version != info.Version || resInfo.Read != info.Read || resInfo.Write != info.Write {
		return false
	}

	for _, v := range res.GetVersionsInfoInternal() {
		for _, q := range v.GetPhysicalTaskQueueInfo().GetInternalTaskQueueStatus() {
			if !q.GetBacklogDrained() {
				return false
			}
		}
	}
	return true
}

func scaleStateToReadCount(scaleState *persistencespb.PartitionScaleState) int32 {
	return max(scaleState.GetTarget(), bitSet(scaleState.BacklogState).len())
}

func scaleStateToInfo(scaleState *persistencespb.PartitionScaleState) *taskqueuespb.PartitionScaleInfo {
	// note if scaleState == nil, read and write will both be 0
	return &taskqueuespb.PartitionScaleInfo{
		Read:          scaleStateToReadCount(scaleState),
		Write:         scaleState.GetTarget(),
		Version:       scaleState.GetTargetVersion(),
		BacklogCounts: scaleState.GetBacklogCounts(),
		BacklogCap:    scaleState.GetBacklogCap(),
	}
}

package matching

import (
	"context"
	"math/bits"
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
	setTarget          func(int)
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
	UpdateScaleState(*persistencespb.PartitionScaleState) error
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
	sm := &scaleManager{
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
	sm.setTarget = sm.SetTarget // allocate closure once
	return sm
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

// OnTasks is called on a batch of tasks added. The caller is required to pass in the current
// target even though we have it already, so that in the common case, we only have to one
// atomic increment. The caller in this case (partitionManager checkPartitionCounts) has
// already gotten the partition counts from ephemeral data, which should match our target.
func (sm *scaleManager) OnTasks(numTasks, currentTarget int) {
	if sm == nil {
		return
	}

	// scale target batch size by numTasks (since numTasks is scaled by partitions)
	batchSize := int64(numTasks) * int64(sm.settings.BatchSize)

	if tasks := sm.batch.Add(int64(numTasks)); tasks >= batchSize {
		tasks = sm.batch.Swap(0)
		sm.partitionScaler.OnTasks(int(tasks), currentTarget, sm.setTarget)
	}
}

// SetTarget may be called in the task add path; it shouldn't block.
func (sm *scaleManager) SetTarget(targeti int) {
	if sm == nil {
		return
	}

	if !sm.lock.TryLock() {
		return // don't block on contention
	}

	if sm.scaleDB == nil || !sm.limiter.Allow() {
		sm.lock.Unlock()
		return
	}

	// Do the rest async so that we don't block the task add.
	// Note that we unlock sm.lock in another goroutine.
	go func() {
		defer sm.lock.Unlock()

		target := int32(targeti)

		newState := common.CloneProto(sm.scaleState)
		if newState == nil {
			newState = &persistencespb.PartitionScaleState{}
		}
		prevTarget := newState.Target
		newState.Target = target
		newState.MaxTarget = max(newState.MaxTarget, target)
		// Use timestamp instead of just incrementing here for extra safety: if we just
		// incremented, and scaleDB.UpdateScaleState fails, then we could have two "states"
		// with the same version but different targets, which could confuse things.
		newState.TargetVersion = time.Now().UnixNano()

		mayHaveBacklog := target
		if prevTarget == 0 {
			// Turning on managed partition scaling: consider all partitions from dynamic
			// config as having backlog also.
			mayHaveBacklog = max(mayHaveBacklog, int32(sm.getWritePartitions()))
		}
		for i := range mayHaveBacklog {
			setBacklogStateBit(newState, i)
		}

		// we must succesfully write to the db before making new state active
		if err := sm.scaleDB.UpdateScaleState(newState); err != nil {
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

	// only push ephemeral data if read/write changed, not on any state change
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
			sm.lock.Unlock()

			// notify once
			tasks := int(sm.batch.Swap(0))
			sm.partitionScaler.OnTasks(tasks, int(scaleState.GetTarget()), sm.setTarget)

			// check drained state
			sm.checkDrained(ctx, scaleState)
		}
	}
}

func (sm *scaleManager) checkDrained(ctx context.Context, scaleState *persistencespb.PartitionScaleState) {
	if scaleState.GetTarget() == 0 {
		return // managed scaling disabled
	} else if time.Since(time.Unix(0, scaleState.GetTargetVersion())) < sm.settings.DrainBufferTime {
		return // too soon: wait for some buffer before draining
	}

	// we have partitions that should be draining, see if they are yet
	var toClear []int32
	info := scaleStateToInfo(scaleState)
	for id := info.Write; id < info.Read; id++ {
		if !getBacklogStateBit(scaleState, id) {
			continue
		}

		callCtx, cancel := context.WithTimeout(ctx, ioTimeout)
		res, err := sm.matchingClient.DescribeTaskQueuePartition(callCtx, &matchingservice.DescribeTaskQueuePartitionRequest{
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
		})
		cancel()
		if err == nil && partitionIsFullyDrained(res, info) {
			toClear = append(toClear, id)
		}
	}

	if len(toClear) == 0 {
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
	for _, i := range toClear {
		clearBacklogStateBit(newState, i)
	}

	// write to the db before making new state active
	if err := sm.scaleDB.UpdateScaleState(newState); err != nil {
		sm.logger.Error("failed to update state", tag.Error(err), tag.Operation("drain"))
		return
	}

	sm.logger.Info("drain",
		tag.Any("drained-partitions", toClear),
		tag.Int32("target", info.Write),
		tag.Int32("prev-read", info.Read),
		tag.Int32("read", readPartitionsFromBacklogState(newState)))

	sm.setStateLocked(newState)
}

func partitionIsFullyDrained(
	res *matchingservice.DescribeTaskQueuePartitionResponse,
	info *taskqueuespb.PartitionScaleInfo,
) bool {
	if !proto.Equal(res.GetScaleInfo(), info) {
		// Require that the partition agrees with the current scale state, i.e. it knows that
		// it's draining, i.e. it knows it can't accept any new tasks. We include the version
		// as well as just the read+write counts to avoid an ABA problem.
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

func scaleStateToInfo(scaleState *persistencespb.PartitionScaleState) *taskqueuespb.PartitionScaleInfo {
	// note if scaleState == nil, read and write will both be 0
	return &taskqueuespb.PartitionScaleInfo{
		Read:    max(scaleState.GetTarget(), readPartitionsFromBacklogState(scaleState)),
		Write:   scaleState.GetTarget(),
		Version: scaleState.GetTargetVersion(),
	}
}

func readPartitionsFromBacklogState(state *persistencespb.PartitionScaleState) int32 {
	i := len(state.GetBacklogState()) - 1
	if i < 0 {
		return 0
	}
	return int32(bits.Len64(state.BacklogState[i]) + i*64)
}

func getBacklogStateBit(state *persistencespb.PartitionScaleState, i int32) bool {
	if len(state.BacklogState) < int(i)/64+1 {
		return false
	}
	return state.BacklogState[i/64]&(1<<(i%64)) != 0
}

func setBacklogStateBit(state *persistencespb.PartitionScaleState, i int32) {
	for len(state.BacklogState) < int(i)/64+1 {
		state.BacklogState = append(state.BacklogState, 0)
	}
	state.BacklogState[i/64] |= 1 << (i % 64)
}

func clearBacklogStateBit(state *persistencespb.PartitionScaleState, i int32) {
	if len(state.BacklogState) < int(i)/64+1 {
		return
	}
	state.BacklogState[i/64] &^= 1 << (i % 64)
	for len(state.BacklogState) > 0 && state.BacklogState[len(state.BacklogState)-1] == 0 {
		state.BacklogState = state.BacklogState[:len(state.BacklogState)-1]
	}
}

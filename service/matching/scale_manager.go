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
	"go.temporal.io/server/common/quotas"
	"go.temporal.io/server/common/tqid"
	"go.temporal.io/server/common/util"
)

// scaleManager keeps some state and manages the interaction with partitionScaler.
// scaleManager runs on the root partition only.
type scaleManager struct {
	partition       tqid.Partition
	logger          log.Logger
	userDataManager userDataManager
	matchingClient  matchingservice.MatchingServiceClient
	partitionScaler PartitionScaler
	// for simplicity, settings are fixed at construction time
	settings             dynamicconfig.PartitionScaleManagerSettings
	getWritePartitions   dynamicconfig.IntPropertyFn
	setTarget            func(int)
	periodicNotification *goro.Handle
	limiter              quotas.RateLimiter

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
	userDataManager userDataManager,
	matchingClient matchingservice.MatchingServiceClient,
	partitionScaler PartitionScaler,
	settings dynamicconfig.PartitionScaleManagerSettings,
	getWritePartitions dynamicconfig.IntPropertyFn,
) *scaleManager {
	sm := &scaleManager{
		partition:            partition,
		logger:               log.With(logger, tag.ComponentPartitionScaler),
		userDataManager:      userDataManager,
		matchingClient:       matchingClient,
		partitionScaler:      partitionScaler,
		settings:             settings,
		getWritePartitions:   getWritePartitions,
		periodicNotification: goro.NewHandle(baseCtx),
		limiter:              quotas.NewRateLimiter(float64(settings.MaxRate), 1),
	}
	sm.setTarget = sm.SetTarget // allocate closure once
	sm.periodicNotification.Go(sm.sendPeriodicNotification)
	return sm
}

func (sm *scaleManager) Stop() {
	if sm == nil {
		return
	}
	sm.periodicNotification.Cancel()
	sm.partitionScaler.Stop()
}

// LoadedMetadata is called when the root partitions's default queue has loaded its metadata.
func (sm *scaleManager) LoadedMetadata(
	scaleState *persistencespb.PartitionScaleState,
	defaultQueue scaleDB,
) {
	if sm == nil {
		return
	}

	sm.lock.Lock()
	defer sm.lock.Unlock()

	sm.scaleDB = defaultQueue
	sm.setStateLocked(scaleState)
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

	// do rest async so that we don't block the task add.
	// note that we unlock sm.lock in another goroutine.
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

		if prevTarget == 0 {
			// turning on managed partition scaling: consider all partitions from dynamic
			// config as having backlog.
			for i := range max(target, int32(sm.getWritePartitions())) {
				setBacklogStateBit(newState, i)
			}
		} else {
			// mark all new partitions as having backlog
			for i := prevTarget; i < target; i++ {
				setBacklogStateBit(newState, i)
			}
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

		sm.setStateLocked(newState)
	}()
}

// setStateLocked updates the current scale state and syncs it to ephemeral data.
// This should only be called _after_ the state is persisted to the db.
func (sm *scaleManager) setStateLocked(newState *persistencespb.PartitionScaleState) {
	prevState := sm.scaleState
	sm.scaleState = newState

	// note if newState == nil, read and write will both be 0
	write := sm.scaleState.GetTarget()
	read := max(write, readPartitionsFromBacklogState(sm.scaleState))

	// only push ephemeral data if read/write changed, not on any state change
	prevWrite := prevState.GetTarget()
	prevRead := max(write, readPartitionsFromBacklogState(prevState))
	if write == prevWrite && read == prevRead {
		return
	}

	sm.userDataManager.SetPartitionScale(&taskqueuespb.PartitionScaleInfo{
		Read:  read,
		Write: write,
	})
}

func (sm *scaleManager) sendPeriodicNotification(ctx context.Context) error {
	util.InterruptibleSleep(ctx, backoff.FullJitter(sm.settings.IdleInterval))
	t := time.NewTicker(sm.settings.IdleInterval).C
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t:
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
	read := readPartitionsFromBacklogState(scaleState)
	target := scaleState.GetTarget()
	if target == 0 || read <= target {
		return // nothing to do
	}

	// we have partitions that should be draining, see if they are yet
	var toClear []int32
	for id := target; id < read; id++ {
		if !getBacklogStateBit(scaleState, id) {
			continue
		}

		callCtx, cancel := context.WithTimeout(ctx, ioTimeout)
		res, err := sm.matchingClient.DescribeTaskQueuePartition(callCtx, &matchingservice.DescribeTaskQueuePartitionRequest{
			NamespaceId:        sm.partition.NamespaceId(),
			TaskQueuePartition: &taskqueuespb.TaskQueuePartition{},
			Versions: &taskqueuepb.TaskQueueVersionSelection{
				Unversioned: true,
				// TODO: what about "inactive" versions?
				AllActive: true,
			},
			ReportInternalTaskQueueStatus: true,
		})
		cancel()
		if err == nil && sm.isFullyDrained(res) {
			toClear = append(toClear, id)
		}
	}

	if len(toClear) == 0 {
		return
	}

	sm.lock.Lock()
	defer sm.lock.Unlock()

	if sm.scaleState != scaleState {
		// we were operating from an old state
		return
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
		tag.Int32("prev-read", read),
		tag.Int32("read", readPartitionsFromBacklogState(newState)))

	sm.setStateLocked(newState)
}

func (*scaleManager) isFullyDrained(res *matchingservice.DescribeTaskQueuePartitionResponse) bool {
	for _, v := range res.GetVersionsInfoInternal() {
		for _, q := range v.GetPhysicalTaskQueueInfo().GetInternalTaskQueueStatus() {
			if !q.GetDrained() {
				return false
			}
		}
	}
	return true
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

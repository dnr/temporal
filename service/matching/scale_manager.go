package matching

import (
	"context"
	"math/bits"
	"sync"
	"sync/atomic"
	"time"

	persistencespb "go.temporal.io/server/api/persistence/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/goro"
	"go.temporal.io/server/common/util"
)

const (
	// scaleNotificationBatchSize is the size of a batch to send to the partitions scaler.
	scaleNotificationBatchSize = 100
	// scaleNotificationInterval is the interval to send signals to the scaler even if not a
	// full batch of tasks has been received yet.
	scaleNotificationInterval = 46 * time.Second
)

// scaleManager keeps some state and manages the interaction with partitionScaler.
// scaleManager runs on the root partition only.
type scaleManager struct {
	userDataManager      userDataManager
	partitionScaler      PartitionScaler
	setTarget            func(int)
	periodicNotification *goro.Handle

	lock         sync.Mutex
	scaleState   *persistencespb.PartitionScaleState
	defaultQueue physicalTaskQueueManager // used to write state to db

	batch atomic.Int64
}

func newScaleManager(
	userDataManager userDataManager,
	partitionScaler PartitionScaler,
) *scaleManager {
	sm := &scaleManager{
		userDataManager:      userDataManager,
		partitionScaler:      partitionScaler,
		periodicNotification: goro.NewHandle(context.Background()),
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
	defaultQueue physicalTaskQueueManager,
) {
	if sm == nil {
		return
	}

	sm.lock.Lock()
	defer sm.lock.Unlock()

	sm.defaultQueue = defaultQueue
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
	batchSize := numTasks * scaleNotificationBatchSize

	if tasks := sm.batch.Add(int64(numTasks)); tasks >= int64(batchSize) {
		tasks = sm.batch.Swap(0)
		sm.partitionScaler.OnTasks(int(tasks), currentTarget, sm.setTarget)
	}
}

func (sm *scaleManager) SetTarget(targeti int) {
	if sm == nil {
		return
	}

	if !sm.lock.TryLock() {
		return // don't block on contention
	}

	// FIXME: add some limits on frequency of changes here
	if sm.defaultQueue == nil || false /* too fast ... */ {
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

		// mark all new partitions as having backlog
		for i := prevTarget; i < target; i++ {
			setBacklogStateBit(newState, i)
		}

		// we must succesfully write to the db before making new state active
		if sm.defaultQueue.UpdateScaleState(newState) != nil {
			return
		}

		sm.setStateLocked(newState)
	}()
}

// setStateLocked updates the current scale state and syncs it to ephemeral data.
// This should only be called _after_ the state is persisted to the db.
func (sm *scaleManager) setStateLocked(newState *persistencespb.PartitionScaleState) {
	sm.scaleState = newState

	// note if newState == nil, read and write will both be 0
	write := sm.scaleState.GetTarget()
	read := max(write, readPartitionsFromBacklogState(sm.scaleState))

	sm.userDataManager.SetPartitionScale(&taskqueuespb.PartitionScaleInfo{
		Read:  read,
		Write: write,
	})
}

func (sm *scaleManager) sendPeriodicNotification(ctx context.Context) error {
	util.InterruptibleSleep(ctx, backoff.FullJitter(scaleNotificationInterval))
	t := time.NewTicker(scaleNotificationInterval).C
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t:
			sm.lock.Lock()
			target := int(sm.scaleState.GetTarget())
			sm.lock.Unlock()
			tasks := int(sm.batch.Swap(0))
			sm.partitionScaler.OnTasks(tasks, target, sm.setTarget)
		}
	}
}

func readPartitionsFromBacklogState(state *persistencespb.PartitionScaleState) int32 {
	i := len(state.GetBacklogState()) - 1
	if i < 0 {
		return 0
	}
	return int32(bits.Len64(state.BacklogState[i]) + i*64)
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

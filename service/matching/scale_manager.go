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

// scaleManager keeps some state and manages the interaction with partitionScaler.
// scaleManager runs on the root partition only.
type scaleManager struct {
	userDataManager userDataManager
	partitionScaler PartitionScaler
	setTarget       func(int)
	scaleDown       *goro.Handle

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
		userDataManager: userDataManager,
		partitionScaler: partitionScaler,
		scaleDown:       goro.NewHandle(context.Background()),
	}
	sm.setTarget = sm.SetTarget // allocate closure once
	sm.scaleDown.Go(sm.scaleDownPeriodically)
	return sm
}

func (sm *scaleManager) Stop() {
	if sm == nil {
		return
	}
	sm.scaleDown.Cancel()
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

	sm.scaleState = scaleState
	sm.defaultQueue = defaultQueue
	sm.syncToEphemeralDataLocked()
}

// OnTask is called on every task added. The caller is required to pass in the current target
// even though we have it already, so that we don't have to do another mutex lock. The caller
// in this case (partitionManager checkPartitionCounts) has already gotten the partition counts
// from ephemeral data.
func (sm *scaleManager) OnTask(currentTarget int) {
	if sm == nil {
		return
	}

	// FIXME: make adjustable
	const batchSize = 100

	if sm.batch.Add(1) >= batchSize {
		sm.batch.Add(-batchSize)
		sm.partitionScaler.OnTasks(batchSize, currentTarget, sm.setTarget)
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
			newState.BacklogState = setBacklogStateBits(
				newState.BacklogState, i)
		}

		// we must succesfully write to the db before making new state active
		if sm.defaultQueue.UpdateScaleState(newState) != nil {
			return
		}

		sm.scaleState = newState
		sm.syncToEphemeralDataLocked()
	}()
}

func (sm *scaleManager) syncToEphemeralDataLocked() {
	write := sm.scaleState.GetTarget()
	read := max(write, readPartitionsFromBacklogState(sm.scaleState.GetBacklogState()))

	sm.userDataManager.SetPartitionScale(&taskqueuespb.PartitionScaleInfo{
		Read:  read,
		Write: write,
	})
}

func (sm *scaleManager) scaleDownPeriodically(ctx context.Context) error {
	const interval = time.Minute
	util.InterruptibleSleep(ctx, backoff.FullJitter(interval))
	t := time.NewTicker(time.Minute).C
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t:
			sm.lock.Lock()
			target := sm.scaleState.GetTarget()
			sm.lock.Unlock()
			sm.partitionScaler.OnTasks(0, int(target), sm.setTarget)
		}
	}
}

func readPartitionsFromBacklogState(state []uint64) int32 {
	i := len(state) - 1
	if i < 0 {
		return 0
	}
	return int32(bits.Len64(state[i]) - 1 + i*64)
}

func setBacklogStateBits(state []uint64, i int32) []uint64 {
	for len(state) < int(i)/64+1 {
		state = append(state, 0)
	}
	state[i/64] |= 1 << (i % 64)
	return state
}

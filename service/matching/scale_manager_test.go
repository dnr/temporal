package matching

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/api/matchingservice/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/goro"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/quotas"
	"go.temporal.io/server/common/tqid"
	"google.golang.org/protobuf/proto"
)

// -- scaleStateToInfo tests --

func TestScaleStateToInfo_Nil(t *testing.T) {
	t.Parallel()
	info := scaleStateToInfo(nil)
	assert.Equal(t, int32(0), info.Read)
	assert.Equal(t, int32(0), info.Write)
}

func TestScaleStateToInfo_TargetOnly(t *testing.T) {
	t.Parallel()
	state := &persistencespb.PartitionScaleState{Target: 4}
	info := scaleStateToInfo(state)
	assert.Equal(t, int32(4), info.Read)
	assert.Equal(t, int32(4), info.Write)
}

func TestScaleStateToInfo_BacklogAboveTarget(t *testing.T) {
	t.Parallel()
	state := &persistencespb.PartitionScaleState{Target: 4}
	(*bitSet)(&state.BacklogState).set(7) // partition 7 has backlog
	info := scaleStateToInfo(state)
	assert.Equal(t, int32(8), info.Read)  // max(4, 8) = 8
	assert.Equal(t, int32(4), info.Write) // always target
}

func TestScaleStateToInfo_TargetAboveBacklog(t *testing.T) {
	t.Parallel()
	state := &persistencespb.PartitionScaleState{Target: 10}
	(*bitSet)(&state.BacklogState).set(3)
	info := scaleStateToInfo(state)
	assert.Equal(t, int32(10), info.Read)  // max(10, 4) = 10
	assert.Equal(t, int32(10), info.Write) // target
}

func TestScaleStateToInfo_Version(t *testing.T) {
	t.Parallel()
	state := &persistencespb.PartitionScaleState{Target: 4, TargetVersion: 12345}
	info := scaleStateToInfo(state)
	assert.Equal(t, int64(12345), info.Version)
}

// -- partitionIsFullyDrained tests --

func TestPartitionIsFullyDrained_ScaleInfoMismatch(t *testing.T) {
	t.Parallel()
	info := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4, Version: 100}
	res := &matchingservice.DescribeTaskQueuePartitionResponse{
		ScaleInfo: &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4, Version: 99}, // different version
		VersionsInfoInternal: map[string]*taskqueuespb.TaskQueueVersionInfoInternal{
			"": {PhysicalTaskQueueInfo: &taskqueuespb.PhysicalTaskQueueInfo{
				InternalTaskQueueStatus: []*taskqueuespb.InternalTaskQueueStatus{{BacklogDrained: true}},
			}},
		},
	}
	assert.False(t, partitionIsFullyDrained(res, info))
}

func TestPartitionIsFullyDrained_NotDrained(t *testing.T) {
	t.Parallel()
	info := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4, Version: 100}
	res := &matchingservice.DescribeTaskQueuePartitionResponse{
		ScaleInfo: proto.Clone(info).(*taskqueuespb.PartitionScaleInfo),
		VersionsInfoInternal: map[string]*taskqueuespb.TaskQueueVersionInfoInternal{
			"": {PhysicalTaskQueueInfo: &taskqueuespb.PhysicalTaskQueueInfo{
				InternalTaskQueueStatus: []*taskqueuespb.InternalTaskQueueStatus{{BacklogDrained: false}},
			}},
		},
	}
	assert.False(t, partitionIsFullyDrained(res, info))
}

func TestPartitionIsFullyDrained_AllDrained(t *testing.T) {
	t.Parallel()
	info := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4, Version: 100}
	res := &matchingservice.DescribeTaskQueuePartitionResponse{
		ScaleInfo: proto.Clone(info).(*taskqueuespb.PartitionScaleInfo),
		VersionsInfoInternal: map[string]*taskqueuespb.TaskQueueVersionInfoInternal{
			"v1": {PhysicalTaskQueueInfo: &taskqueuespb.PhysicalTaskQueueInfo{
				InternalTaskQueueStatus: []*taskqueuespb.InternalTaskQueueStatus{{BacklogDrained: true}},
			}},
			"v2": {PhysicalTaskQueueInfo: &taskqueuespb.PhysicalTaskQueueInfo{
				InternalTaskQueueStatus: []*taskqueuespb.InternalTaskQueueStatus{{BacklogDrained: true}, {BacklogDrained: true}},
			}},
		},
	}
	assert.True(t, partitionIsFullyDrained(res, info))
}

func TestPartitionIsFullyDrained_PartiallyDrained(t *testing.T) {
	t.Parallel()
	info := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4, Version: 100}
	res := &matchingservice.DescribeTaskQueuePartitionResponse{
		ScaleInfo: proto.Clone(info).(*taskqueuespb.PartitionScaleInfo),
		VersionsInfoInternal: map[string]*taskqueuespb.TaskQueueVersionInfoInternal{
			"v1": {PhysicalTaskQueueInfo: &taskqueuespb.PhysicalTaskQueueInfo{
				InternalTaskQueueStatus: []*taskqueuespb.InternalTaskQueueStatus{{BacklogDrained: true}},
			}},
			"v2": {PhysicalTaskQueueInfo: &taskqueuespb.PhysicalTaskQueueInfo{
				InternalTaskQueueStatus: []*taskqueuespb.InternalTaskQueueStatus{{BacklogDrained: false}},
			}},
		},
	}
	assert.False(t, partitionIsFullyDrained(res, info))
}

func TestPartitionIsFullyDrained_NoVersions(t *testing.T) {
	t.Parallel()
	info := &taskqueuespb.PartitionScaleInfo{Read: 8, Write: 4, Version: 100}
	res := &matchingservice.DescribeTaskQueuePartitionResponse{
		ScaleInfo:            proto.Clone(info).(*taskqueuespb.PartitionScaleInfo),
		VersionsInfoInternal: map[string]*taskqueuespb.TaskQueueVersionInfoInternal{},
	}
	assert.True(t, partitionIsFullyDrained(res, info))
}

// -- scaleManager helpers --

type testScaleDB struct {
	state    *persistencespb.PartitionScaleState
	lastSync bool
	err      error
}

func (m *testScaleDB) UpdateScaleState(s *persistencespb.PartitionScaleState, sync bool) error {
	if m.err != nil {
		return m.err
	}
	m.state = s
	m.lastSync = sync
	return nil
}

type testPartitionScaler struct {
	calls []testScalerCall
}

type testScalerCall struct {
	numTasks      int
	currentTarget int
}

func (s *testPartitionScaler) OnTasks(numTasks, currentTarget int, setTarget func(int)) {
	s.calls = append(s.calls, testScalerCall{numTasks, currentTarget})
}

func (s *testPartitionScaler) Stop() {}

type testUserDataManager struct {
	scaleInfo *taskqueuespb.PartitionScaleInfo
}

func (m *testUserDataManager) SetPartitionScale(info *taskqueuespb.PartitionScaleInfo) {
	m.scaleInfo = info
}

func newTestScaleManager(
	scaler *testPartitionScaler,
	udm *testUserDataManager,
	settings dynamicconfig.PartitionScaleManagerSettings,
) *scaleManager {
	f, _ := tqid.NewTaskQueueFamily("test-ns-id", "test-tq")
	partition := f.TaskQueue(1).RootPartition()
	return &scaleManager{
		partition:          partition,
		logger:             log.NewNoopLogger(),
		metricsHandler:     metrics.NoopMetricsHandler,
		userDataManager:    &scaleManagerUDMAdapter{udm},
		partitionScaler:    scaler,
		settings:           settings,
		getWritePartitions: func() int { return 4 },
		emitGaugeMetrics:   func() bool { return false },
		background:         goro.NewHandle(context.Background()),
		limiter:            quotas.NewRateLimiter(100, 1), // generous
	}
}

// scaleManagerUDMAdapter adapts testUserDataManager to the userDataManager interface
// (only the methods scaleManager actually calls).
type scaleManagerUDMAdapter struct {
	inner *testUserDataManager
}

func (a *scaleManagerUDMAdapter) SetPartitionScale(info *taskqueuespb.PartitionScaleInfo) {
	a.inner.SetPartitionScale(info)
}

func (a *scaleManagerUDMAdapter) PartitionScale() *taskqueuespb.PartitionScaleInfo {
	return a.inner.scaleInfo
}

// Stubs for unused methods - scaleManager only calls SetPartitionScale/PartitionScale
func (a *scaleManagerUDMAdapter) Start()                                     {}
func (a *scaleManagerUDMAdapter) Stop()                                      {}
func (a *scaleManagerUDMAdapter) WaitUntilInitialized(context.Context) error { return nil }
func (a *scaleManagerUDMAdapter) GetUserData() (*persistencespb.VersionedTaskQueueUserData, chan struct{}, error) {
	return nil, nil, nil
}
func (a *scaleManagerUDMAdapter) UpdateUserData(context.Context, UserDataUpdateOptions, UserDataUpdateFunc) (int64, error) {
	return 0, nil
}
func (a *scaleManagerUDMAdapter) HandleGetUserDataRequest(context.Context, *matchingservice.GetTaskQueueUserDataRequest) (*matchingservice.GetTaskQueueUserDataResponse, error) {
	return nil, nil
}
func (a *scaleManagerUDMAdapter) CheckTaskQueueUserDataPropagation(context.Context, int64, int, int) error {
	return nil
}
func (a *scaleManagerUDMAdapter) LocalBacklogPriorityChanged(map[PhysicalTaskQueueVersion]int64) {}

// -- OnTasks batching tests --

func TestScaleManager_OnTasks_Batching(t *testing.T) {
	t.Parallel()
	scaler := &testPartitionScaler{}
	udm := &testUserDataManager{}
	settings := dynamicconfig.PartitionScaleManagerSettings{
		BatchSize: 10,
		MaxRate:   100,
	}
	sm := newTestScaleManager(scaler, udm, settings)

	// With batchSize=10 and numTasks=1 per call, need 10 calls (since batchSize scaled
	// by numTasks: batchSize = 1 * 10 = 10, so threshold is 10)
	for range 9 {
		sm.OnTasks(1, 4)
	}
	assert.Empty(t, scaler.calls)

	sm.OnTasks(1, 4)
	require.Len(t, scaler.calls, 1)
	assert.Equal(t, 4, scaler.calls[0].currentTarget)
	// accumulated tasks should be ~10
	assert.GreaterOrEqual(t, scaler.calls[0].numTasks, 10)
}

func TestScaleManager_OnTasks_NilSafe(t *testing.T) {
	t.Parallel()
	// Should not panic on nil
	var sm *scaleManager
	sm.OnTasks(10, 4)
}

// -- SetTarget tests --

func TestScaleManager_SetTarget_GrowFromZero(t *testing.T) {
	t.Parallel()
	scaler := &testPartitionScaler{}
	udm := &testUserDataManager{}
	db := &testScaleDB{}
	settings := dynamicconfig.PartitionScaleManagerSettings{
		MaxRate:            100,
		BackgroundInterval: time.Hour,
	}
	sm := newTestScaleManager(scaler, udm, settings)
	sm.scaleDB = db
	sm.scaleState = nil

	sm.SetTarget(4)
	// Wait for the goroutine to complete by locking the mutex
	sm.lock.Lock()
	sm.lock.Unlock()

	require.NotNil(t, db.state)
	assert.Equal(t, int32(4), db.state.Target)
	assert.Equal(t, int32(4), db.state.MaxTarget)
	// Backlog bits should be set for 0..3, and also 0..3 from DC (getWritePartitions=4)
	for i := range 4 {
		assert.True(t, bitSet(db.state.BacklogState).get(int32(i)), "bit %d should be set", i)
	}
	// Ephemeral data should be updated
	require.NotNil(t, udm.scaleInfo)
	assert.Equal(t, int32(4), udm.scaleInfo.Write)
}

func TestScaleManager_SetTarget_GrowFromExisting(t *testing.T) {
	t.Parallel()
	scaler := &testPartitionScaler{}
	udm := &testUserDataManager{}
	db := &testScaleDB{}
	settings := dynamicconfig.PartitionScaleManagerSettings{
		MaxRate:            100,
		BackgroundInterval: time.Hour,
	}
	sm := newTestScaleManager(scaler, udm, settings)
	sm.scaleDB = db
	sm.scaleState = &persistencespb.PartitionScaleState{
		Target:        4,
		TargetVersion: 100,
	}
	for i := range 4 {
		(*bitSet)(&sm.scaleState.BacklogState).set(int32(i))
	}

	sm.SetTarget(8)
	sm.lock.Lock()
	sm.lock.Unlock()

	require.NotNil(t, db.state)
	assert.Equal(t, int32(8), db.state.Target)
	assert.Equal(t, int32(8), db.state.MaxTarget)
	// Backlog bits 0..7 should be set
	for i := range 8 {
		assert.True(t, bitSet(db.state.BacklogState).get(int32(i)), "bit %d should be set", i)
	}
}

func TestScaleManager_SetTarget_Shrink(t *testing.T) {
	t.Parallel()
	scaler := &testPartitionScaler{}
	udm := &testUserDataManager{}
	db := &testScaleDB{}
	settings := dynamicconfig.PartitionScaleManagerSettings{
		MaxRate:            100,
		BackgroundInterval: time.Hour,
	}
	sm := newTestScaleManager(scaler, udm, settings)
	sm.scaleDB = db
	sm.scaleState = &persistencespb.PartitionScaleState{
		Target:        8,
		TargetVersion: 100,
	}
	for i := range 8 {
		(*bitSet)(&sm.scaleState.BacklogState).set(int32(i))
	}

	sm.SetTarget(4)
	sm.lock.Lock()
	sm.lock.Unlock()

	require.NotNil(t, db.state)
	assert.Equal(t, int32(4), db.state.Target)
	// Backlog bits should be UNCHANGED (draining partitions still tracked)
	for i := range 8 {
		assert.True(t, bitSet(db.state.BacklogState).get(int32(i)), "bit %d should still be set", i)
	}
	// Ephemeral data: write=4, read=8 (max of target and backlog)
	require.NotNil(t, udm.scaleInfo)
	assert.Equal(t, int32(4), udm.scaleInfo.Write)
	assert.Equal(t, int32(8), udm.scaleInfo.Read)
}

func TestScaleManager_SetTarget_DBFailure(t *testing.T) {
	t.Parallel()
	scaler := &testPartitionScaler{}
	udm := &testUserDataManager{}
	db := &testScaleDB{err: assert.AnError}
	settings := dynamicconfig.PartitionScaleManagerSettings{
		MaxRate:            100,
		BackgroundInterval: time.Hour,
	}
	sm := newTestScaleManager(scaler, udm, settings)
	sm.scaleDB = db
	sm.scaleState = nil

	sm.SetTarget(4)
	sm.lock.Lock()
	sm.lock.Unlock()

	// State should not be updated when DB fails
	assert.Nil(t, sm.scaleState)
	assert.Nil(t, udm.scaleInfo)
}

func TestScaleManager_SetTarget_NilSafe(t *testing.T) {
	t.Parallel()
	var sm *scaleManager
	sm.SetTarget(4) // should not panic
}

func TestScaleManager_SetTarget_NoDBYet(t *testing.T) {
	t.Parallel()
	scaler := &testPartitionScaler{}
	udm := &testUserDataManager{}
	settings := dynamicconfig.PartitionScaleManagerSettings{
		MaxRate:            100,
		BackgroundInterval: time.Hour,
	}
	sm := newTestScaleManager(scaler, udm, settings)
	// scaleDB is nil (LoadedMetadata not called yet)

	sm.SetTarget(4)
	sm.lock.Lock()
	sm.lock.Unlock()

	// Should not update anything
	assert.Nil(t, udm.scaleInfo)
}

func TestScaleManager_SetTarget_TurningOn_IncludesDCPartitions(t *testing.T) {
	t.Parallel()
	scaler := &testPartitionScaler{}
	udm := &testUserDataManager{}
	db := &testScaleDB{}
	settings := dynamicconfig.PartitionScaleManagerSettings{
		MaxRate:            100,
		BackgroundInterval: time.Hour,
	}
	sm := newTestScaleManager(scaler, udm, settings)
	sm.scaleDB = db
	// Previous target was 0 (not active), DC says 4
	sm.scaleState = &persistencespb.PartitionScaleState{Target: 0}
	sm.getWritePartitions = func() int { return 6 }

	// Setting target to 2 — but since prevTarget=0, it should include DC partitions (6)
	sm.SetTarget(2)
	sm.lock.Lock()
	sm.lock.Unlock()

	require.NotNil(t, db.state)
	assert.Equal(t, int32(2), db.state.Target)
	// Backlog bits should cover max(2, 6) = 6 partitions
	for i := range 6 {
		assert.True(t, bitSet(db.state.BacklogState).get(int32(i)), "bit %d should be set", i)
	}
}

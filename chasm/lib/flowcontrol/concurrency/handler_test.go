package concurrency

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/server/chasm"
	"go.temporal.io/server/chasm/chasmtest"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/log"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type testLibrary struct {
	chasm.UnimplementedLibrary
}

func (*testLibrary) Name() string {
	return "flowcontrol_test"
}

func (*testLibrary) Components() []*chasm.RegistrableComponent {
	return []*chasm.RegistrableComponent{
		chasm.NewRegistrableComponent[*Component]("concurrency_limiter"),
	}
}

func newTestMutableContext(now time.Time) *chasm.MockMutableContext {
	return &chasm.MockMutableContext{
		MockContext: chasm.MockContext{
			HandleNow: func(chasm.Component) time.Time { return now },
		},
	}
}

func TestHandlerWaiterEntries(t *testing.T) {
	h := &Handler{waiters: make(map[batchKey]*waiterEntries)}
	key := batchKey{namespaceID: "namespace", key: "limiter"}
	otherKey := batchKey{namespaceID: "namespace", key: "other"}

	h.registerWaiter(key, 10, 2)
	h.registerWaiter(key, 20, 2)
	h.registerWaiter(key, 20, 1)
	h.registerWaiter(key, 30, 1)
	h.registerWaiter(otherKey, 5, 1)

	tests := []struct {
		wantTokens   int32
		wantWakeUpTo int64
		wantWakeAll  bool
	}{
		{wantTokens: 1, wantWakeUpTo: 10},
		{wantTokens: 2, wantWakeUpTo: 10},
		{wantTokens: 3, wantWakeUpTo: 20},
		{wantTokens: 5, wantWakeUpTo: 20},
		{wantTokens: 6, wantWakeAll: true},
		{wantTokens: 7, wantWakeAll: true},
	}
	for _, tt := range tests {
		wakeUpTo, wakeAll := h.getWakeTime(key, tt.wantTokens)
		require.Equal(t, tt.wantWakeUpTo, wakeUpTo)
		require.Equal(t, tt.wantWakeAll, wakeAll)
	}

	h.unregisterWaiter(key, 20, 1)
	wakeUpTo, wakeAll := h.getWakeTime(key, 5)
	require.Zero(t, wakeUpTo)
	require.True(t, wakeAll)

	h.unregisterWaiter(key, 10, 2)
	h.unregisterWaiter(key, 20, 2)
	h.unregisterWaiter(key, 30, 1)
	require.NotContains(t, h.waiters, key)
	require.Contains(t, h.waiters, otherKey)

	wakeUpTo, wakeAll = h.getWakeTime(key, 1)
	require.Zero(t, wakeUpTo)
	require.True(t, wakeAll)
}

func TestHandlerWaitRetriesWithCurrentGeneration(t *testing.T) {
	registry := chasm.NewRegistry(log.NewNoopLogger())
	require.NoError(t, registry.Register(&chasm.CoreLibrary{}))
	require.NoError(t, registry.Register(&testLibrary{}))
	engine := chasmtest.NewEngine(t, registry)
	ctx := chasm.NewEngineContext(context.Background(), engine)
	key := chasm.ExecutionKey{NamespaceID: "namespace", BusinessID: "limiter"}
	_, err := chasm.StartExecution(
		ctx,
		key,
		func(chasm.MutableContext, struct{}) (*Component, error) {
			c := newTestComponent(1)
			c.Generation = 2
			c.WakeAll = true
			return c, nil
		},
		struct{}{},
	)
	require.NoError(t, err)
	h := &Handler{logger: log.NewNoopLogger(), waiters: make(map[batchKey]*waiterEntries)}
	req := &fcpb.ConcurrencyWaitRequest{
		NamespaceId: key.NamespaceID,
		Key:         key.BusinessID,
		Generation:  1,
	}

	res, err := h.Wait(ctx, req)
	require.NoError(t, err)
	require.Equal(t, int64(2), res.Generation)
	require.Equal(t, int32(1), res.WakeTokens)
	require.Equal(t, int64(1), req.Generation)
	require.Zero(t, req.StartTime)
	require.Zero(t, req.RequestedWakeTokens)
	require.Empty(t, h.waiters)
}

func TestHandlerWaitLongPollTimeoutReturnsRequestGeneration(t *testing.T) {
	registry := chasm.NewRegistry(log.NewNoopLogger())
	require.NoError(t, registry.Register(&chasm.CoreLibrary{}))
	require.NoError(t, registry.Register(&testLibrary{}))
	engine := chasmtest.NewEngine(t, registry)
	engineCtx := chasm.NewEngineContext(context.Background(), engine)
	key := chasm.ExecutionKey{NamespaceID: "namespace", BusinessID: "limiter"}
	_, err := chasm.StartExecution(
		engineCtx,
		key,
		func(chasm.MutableContext, struct{}) (*Component, error) {
			c := newTestComponent(0)
			c.Generation = 2
			return c, nil
		},
		struct{}{},
	)
	require.NoError(t, err)
	h := &Handler{logger: log.NewNoopLogger(), waiters: make(map[batchKey]*waiterEntries)}
	ctx, cancel := context.WithCancel(engineCtx)
	cancel()

	res, err := h.Wait(ctx, &fcpb.ConcurrencyWaitRequest{
		NamespaceId: key.NamespaceID,
		Key:         key.BusinessID,
		Generation:  2,
		StartTime:   100,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), res.Generation)
	require.Zero(t, res.WakeTokens)
	require.Empty(t, h.waiters)
}

func TestHandlerWaitDeadlineExceededReturnsRequestGeneration(t *testing.T) {
	engine := chasm.NewMockEngine(gomock.NewController(t))
	engine.EXPECT().PollComponent(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, context.DeadlineExceeded)
	h := &Handler{logger: log.NewNoopLogger(), waiters: make(map[batchKey]*waiterEntries)}
	ctx := chasm.NewEngineContext(context.Background(), engine)

	res, err := h.Wait(ctx, &fcpb.ConcurrencyWaitRequest{
		NamespaceId: "namespace",
		Key:         "limiter",
		Generation:  2,
		StartTime:   100,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), res.Generation)
	require.Zero(t, res.WakeTokens)
	require.Empty(t, h.waiters)
}

func TestInitFn(t *testing.T) {
	tests := []struct {
		name        string
		limit       int32
		wantWakeAll bool
	}{
		{name: "positive limit starts unblocked", limit: 2, wantWakeAll: true},
		{name: "zero limit starts blocked", limit: 0, wantWakeAll: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := initFn(nil, chasmReq{items: []batchReq{{
				req: &fcpb.ConcurrencyBatchRequest{
					ConfigUpdate:        &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: tt.limit},
					ConfigUpdateVersion: 4,
				},
			}}})
			require.NoError(t, err)
			require.Equal(t, tt.limit, c.Config.ConcurrentTasks)
			require.Equal(t, int64(4), c.ConfigVersion)
			require.Equal(t, tt.wantWakeAll, c.WakeAll)
		})
	}
}

func TestUpdateFnReserveLastSlotIncrementsGeneration(t *testing.T) {
	now := time.Now().UTC()
	c := newTestComponent(1)
	c.Generation = 3
	c.WakeUpTo = 100
	c.WakeAll = true
	c.WakeStage = 4

	ress, err := updateFn(c, newTestMutableContext(now), chasmReq{items: []batchReq{{
		req: &fcpb.ConcurrencyBatchRequest{ReserveSlots: []string{"new"}},
	}}})
	require.NoError(t, err)
	require.Len(t, ress, 1)
	require.Equal(t, []bool{true}, ress[0].ReserveSuccess)
	require.Equal(t, int64(4), ress[0].Generation)
	require.Equal(t, int64(4), c.Generation)
	require.Zero(t, c.WakeUpTo)
	require.False(t, c.WakeAll)
	require.Zero(t, c.WakeStage)
}

func TestUpdateFnFailedReserveDoesNotIncrementGeneration(t *testing.T) {
	now := time.Now().UTC()
	c := newTestComponent(1)
	c.Generation = 3
	c.Slots = []*fcpb.ConcurrencyState_Slot{{SlotId: "existing", Committed: true}}

	ress, err := updateFn(c, newTestMutableContext(now), chasmReq{items: []batchReq{{
		req: &fcpb.ConcurrencyBatchRequest{ReserveSlots: []string{"new"}},
	}}})
	require.NoError(t, err)
	require.Equal(t, []bool{false}, ress[0].ReserveSuccess)
	require.Equal(t, int64(3), ress[0].Generation)
	require.Equal(t, int64(3), c.Generation)
}

func TestUpdateFnExpireAndReserveIncrementsGeneration(t *testing.T) {
	now := time.Now().UTC()
	c := newTestComponent(1)
	c.Generation = 3
	c.Slots = []*fcpb.ConcurrencyState_Slot{{
		SlotId:  "expired",
		Expires: timestamppb.New(now.Add(-time.Second)),
	}}

	ress, err := updateFn(c, newTestMutableContext(now), chasmReq{items: []batchReq{{
		req: &fcpb.ConcurrencyBatchRequest{ReserveSlots: []string{"replacement"}},
	}}})
	require.NoError(t, err)
	require.Equal(t, []bool{true}, ress[0].ReserveSuccess)
	require.Equal(t, int64(4), ress[0].Generation)
	require.Equal(t, int64(4), c.Generation)
	require.Equal(t, "replacement", c.Slots[0].SlotId)
}

func TestUpdateFnAtomicReleaseAndReserveDoesNotIncrementGeneration(t *testing.T) {
	now := time.Now().UTC()
	c := newTestComponent(1)
	c.Generation = 3
	c.Slots = []*fcpb.ConcurrencyState_Slot{{SlotId: "old", Committed: true}}

	ress, err := updateFn(c, newTestMutableContext(now), chasmReq{items: []batchReq{
		{req: &fcpb.ConcurrencyBatchRequest{ReleaseSlots: []string{"old"}}},
		{req: &fcpb.ConcurrencyBatchRequest{ReserveSlots: []string{"new"}}},
	}})
	require.NoError(t, err)
	require.Len(t, ress, 2)
	require.Equal(t, []bool{true}, ress[1].ReserveSuccess)
	require.Equal(t, int64(3), ress[0].Generation)
	require.Equal(t, int64(3), ress[1].Generation)
	require.Equal(t, int64(3), c.Generation)
	require.Equal(t, "new", c.Slots[0].SlotId)
}

func TestUpdateFnConfigDecreaseIncrementsGeneration(t *testing.T) {
	now := time.Now().UTC()
	c := newTestComponent(2)
	c.ConfigVersion = 1
	c.Generation = 3
	c.WakeAll = true
	c.Slots = []*fcpb.ConcurrencyState_Slot{{SlotId: "existing", Committed: true}}

	ress, err := updateFn(c, newTestMutableContext(now), chasmReq{items: []batchReq{{
		req: &fcpb.ConcurrencyBatchRequest{
			ConfigUpdate:        &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1},
			ConfigUpdateVersion: 2,
		},
	}}})
	require.NoError(t, err)
	require.Equal(t, int64(4), ress[0].Generation)
	require.Equal(t, int64(4), c.Generation)
	require.False(t, c.WakeAll)
}

func TestUpdateFnAvailabilityIncreaseStartsWake(t *testing.T) {
	now := time.Now().UTC()
	cctx := newTestMutableContext(now)
	c := newTestComponent(1)
	c.Generation = 3
	c.Slots = []*fcpb.ConcurrencyState_Slot{{SlotId: "existing", Committed: true}}
	wantTokens := int32(0)

	ress, err := updateFn(c, cctx, chasmReq{
		items: []batchReq{{req: &fcpb.ConcurrencyBatchRequest{ReleaseSlots: []string{"existing"}}}},
		getWakeTime: func(tokens int32) (int64, bool) {
			wantTokens = tokens
			return 100, false
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), ress[0].Generation)
	require.Equal(t, int32(1), wantTokens)
	require.Equal(t, int64(100), c.WakeUpTo)
	require.False(t, c.WakeAll)
	require.Len(t, cctx.Tasks, 1)
	require.Equal(t, now.Add(stagedWakeInterval), cctx.Tasks[0].Attributes.ScheduledTime)
	require.IsType(t, &stagedWake{}, cctx.Tasks[0].Payload)
}

func TestUpdateFnNoAvailabilityIncreaseDoesNotRestartWake(t *testing.T) {
	now := time.Now().UTC()
	cctx := newTestMutableContext(now)
	c := newTestComponent(2)
	c.Generation = 3
	c.WakeUpTo = 100
	c.Slots = []*fcpb.ConcurrencyState_Slot{{SlotId: "existing", Committed: true}}

	ress, err := updateFn(c, cctx, chasmReq{
		items: []batchReq{{req: &fcpb.ConcurrencyBatchRequest{ReleaseSlots: []string{"missing"}}}},
		getWakeTime: func(int32) (int64, bool) {
			t.Fatal("getWakeTime called without an availability increase")
			return 0, false
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), ress[0].Generation)
	require.Equal(t, int64(100), c.WakeUpTo)
	require.Empty(t, cctx.Tasks)
}

func TestUpdateFnExpirationStartsWake(t *testing.T) {
	now := time.Now().UTC()
	cctx := newTestMutableContext(now)
	c := newTestComponent(1)
	c.Generation = 3
	c.Slots = []*fcpb.ConcurrencyState_Slot{{
		SlotId:  "expired",
		Expires: timestamppb.New(now.Add(-time.Second)),
	}}
	wantTokens := int32(0)

	ress, err := updateFn(c, cctx, chasmReq{
		items: []batchReq{{req: &fcpb.ConcurrencyBatchRequest{}}},
		getWakeTime: func(tokens int32) (int64, bool) {
			wantTokens = tokens
			return 0, true
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), ress[0].Generation)
	require.Equal(t, int32(1), wantTokens)
	require.Empty(t, c.Slots)
	require.True(t, c.WakeAll)
	require.Empty(t, cctx.Tasks)
}

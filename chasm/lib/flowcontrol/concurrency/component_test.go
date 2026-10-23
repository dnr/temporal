package concurrency

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/testing/protorequire"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultReserveTimeout = 30 * time.Second

func newTestComponent(limit int32) *Component {
	return &Component{
		ConcurrencyState: &fcpb.ConcurrencyState{
			Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: limit},
		},
	}
}

func TestSlotLifecycle(t *testing.T) {
	t.Parallel()
	now := time.Now()
	limiter := newTestComponent(1)

	require.True(t, limiter.reserve("slot-1", now, defaultReserveTimeout))
	require.True(t, limiter.reserve("slot-1", now, defaultReserveTimeout))
	require.Len(t, limiter.Slots, 1)
	require.False(t, limiter.reserve("slot-2", now, defaultReserveTimeout))
	require.False(t, limiter.commit("slot-2"))

	require.True(t, limiter.commit("slot-1"))
	require.True(t, limiter.commit("slot-1"))
	require.True(t, limiter.reserve("slot-1", now, defaultReserveTimeout))
	require.Nil(t, limiter.Slots[0].Expires)
	limiter.cancelReservation("slot-1")
	limiter.expire(now.Add(time.Hour))
	require.Len(t, limiter.Slots, 1)

	limiter.release("slot-2")
	require.Len(t, limiter.Slots, 1)
	limiter.release("slot-1")
	require.Empty(t, limiter.Slots)
	limiter.release("slot-1")
	require.Empty(t, limiter.Slots)
}

func TestCancelReservationFreesCapacity(t *testing.T) {
	t.Parallel()
	now := time.Now()
	limiter := newTestComponent(1)
	require.True(t, limiter.reserve("slot-1", now, defaultReserveTimeout))

	// Release only affects committed slots.
	limiter.release("slot-1")
	require.False(t, limiter.reserve("slot-2", now, defaultReserveTimeout))
	limiter.cancelReservation("slot-1")
	limiter.cancelReservation("slot-1")
	require.False(t, limiter.commit("slot-1"))
	require.True(t, limiter.reserve("slot-2", now, defaultReserveTimeout))
}

func TestExpiredSlotIDCanBeReplaced(t *testing.T) {
	t.Parallel()
	now := time.Now()
	limiter := newTestComponent(1)

	require.True(t, limiter.reserve("expired-slot", now, defaultReserveTimeout))
	limiter.expire(now.Add(defaultReserveTimeout - 2*time.Second))
	require.False(t, limiter.reserve("new-slot", now, defaultReserveTimeout))
	limiter.expire(now.Add(defaultReserveTimeout + 2*time.Second))
	require.False(t, limiter.commit("expired-slot"))
	require.True(t, limiter.reserve("new-slot", now.Add(defaultReserveTimeout), defaultReserveTimeout))
	require.Len(t, limiter.Slots, 1)
	require.Equal(t, "new-slot", limiter.Slots[0].GetSlotId())
}

func TestPoll(t *testing.T) {
	t.Parallel()
	now := time.Now()
	type result struct {
		generation int64
		tokens     int32
		ready      bool
	}
	for _, tc := range []struct {
		name              string
		state             *fcpb.ConcurrencyState
		requestGeneration int64
		requestTokens     int32
		want              result
	}{
		{
			name:              "old generation retries with current generation",
			state:             &fcpb.ConcurrencyState{Config: &taskqueuepb.ConcurrencyLimit{}, Generation: 2},
			requestGeneration: 1, requestTokens: 1,
			want: result{generation: 2, ready: true},
		},
		{
			name:              "future generation waits",
			state:             &fcpb.ConcurrencyState{Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1}, Generation: 2},
			requestGeneration: 3, requestTokens: 1,
		},
		{
			name: "wake all returns available capacity",
			state: &fcpb.ConcurrencyState{
				Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 3}, Generation: 2, WakeAll: true,
				Slots: []*fcpb.ConcurrencyState_Slot{{SlotId: "committed", Committed: true}},
			},
			requestGeneration: 2, requestTokens: 3,
			want: result{generation: 2, tokens: 2, ready: true},
		},
		{
			name: "wake tokens are capped by the request",
			state: &fcpb.ConcurrencyState{
				Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 3}, Generation: 2, WakeAll: true,
			},
			requestGeneration: 2, requestTokens: 1,
			want: result{generation: 2, tokens: 1, ready: true},
		},
		{
			name:              "waits until wake state advances",
			state:             &fcpb.ConcurrencyState{Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1}, Generation: 2},
			requestGeneration: 2, requestTokens: 1,
		},
		{
			name: "expired reservation wakes without a transition",
			state: &fcpb.ConcurrencyState{
				Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1}, Generation: 2,
				Slots: []*fcpb.ConcurrencyState_Slot{{SlotId: "expired", Expires: timestamppb.New(now.Add(-time.Second))}},
			},
			requestGeneration: 2, requestTokens: 1,
			want: result{generation: 2, tokens: 1, ready: true},
		},
		{
			name: "unexpired reservation keeps waiter blocked",
			state: &fcpb.ConcurrencyState{
				Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1}, Generation: 2,
				Slots: []*fcpb.ConcurrencyState_Slot{{SlotId: "active", Expires: timestamppb.New(now.Add(time.Minute))}},
			},
			requestGeneration: 2, requestTokens: 1,
		},
		{
			name: "woken generation stays ready after capacity is consumed",
			state: &fcpb.ConcurrencyState{
				Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1}, Generation: 2, WakeAll: true,
				Slots: []*fcpb.ConcurrencyState_Slot{{SlotId: "committed", Committed: true}},
			},
			requestGeneration: 2, requestTokens: 1,
			want: result{generation: 2, ready: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			limiter := &Component{ConcurrencyState: tc.state}
			before := common.CloneProto(tc.state)

			generation, tokens, ready := limiter.poll(now, tc.requestGeneration, tc.requestTokens)

			require.Equal(t, tc.want, result{generation, tokens, ready})
			protorequire.ProtoEqual(t, before, limiter.ConcurrencyState)
		})
	}
}

func TestIncrementGenerationResetsWakeState(t *testing.T) {
	t.Parallel()
	c := newTestComponent(1)
	c.Generation = 3
	c.WakeAll = true

	c.incrementGeneration()

	require.Equal(t, int64(4), c.Generation)
	require.False(t, c.WakeAll)
}

func TestUpdateConfigRequiresNewerVersion(t *testing.T) {
	t.Parallel()
	c := newTestComponent(1)
	c.ConfigVersion = 2

	c.updateConfig(&taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 2}, 2)
	c.updateConfig(nil, 4)
	require.Equal(t, int32(1), c.Config.ConcurrentTasks)
	require.Equal(t, int64(2), c.ConfigVersion)

	c.updateConfig(&taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 3}, 3)
	require.Equal(t, int32(3), c.Config.ConcurrentTasks)
	require.Equal(t, int64(3), c.ConfigVersion)
}

func TestLimitDecreaseRetainsCommittedSlots(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c := newTestComponent(2)
	for _, id := range []string{"first", "second"} {
		require.True(t, c.reserve(id, now, defaultReserveTimeout))
		require.True(t, c.commit(id))
	}

	c.updateConfig(&taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1}, 1)

	require.Len(t, c.Slots, 2)
	require.Zero(t, c.availableSlots())
	require.False(t, c.reserve("third", now, defaultReserveTimeout))
	c.release("first")
	require.Zero(t, c.availableSlots())
	c.release("second")
	require.Equal(t, int32(1), c.availableSlots())
	require.True(t, c.reserve("third", now, defaultReserveTimeout))
}

package flowcontrol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
)

func TestConcurrencySlotLifecycle(t *testing.T) {
	now := time.Now().UTC()
	limiter := &concurrency{
		ConcurrencyState: &fcpb.ConcurrencyState{
			Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1},
		},
	}

	require.Equal(t, int32(1), limiter.reserve("slot-1", now))
	require.Equal(t, int32(1), limiter.reserve("slot-1", now))
	require.Len(t, limiter.Slots, 1)
	require.Equal(t, int32(0), limiter.reserve("slot-2", now))
	require.Error(t, limiter.commit("slot-2", now))

	require.NoError(t, limiter.commit("slot-1", now))
	limiter.cancelReservation("slot-1", now)
	require.Len(t, limiter.Slots, 1)

	limiter.release("slot-2", now)
	require.Len(t, limiter.Slots, 1)
	limiter.release("slot-1", now)
	require.Empty(t, limiter.Slots)
	limiter.release("slot-1", now)
	require.Empty(t, limiter.Slots)
}

func TestConcurrencyExpiredSlotIDCanBeReplaced(t *testing.T) {
	now := time.Now().UTC()
	limiter := &concurrency{
		ConcurrencyState: &fcpb.ConcurrencyState{
			Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1},
		},
	}

	require.Equal(t, int32(1), limiter.reserve("expired-slot", now))
	require.Equal(t, int32(1), limiter.reserve("new-slot", now.Add(reserveTimeout+time.Second)))
	require.Len(t, limiter.Slots, 1)
	require.Equal(t, "new-slot", limiter.Slots[0].GetSlotId())
}

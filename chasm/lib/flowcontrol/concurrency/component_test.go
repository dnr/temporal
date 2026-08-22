package concurrency

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
)

func TestConcurrencySlotLifecycle(t *testing.T) {
	now := time.Now().UTC()
	limiter := &Component{
		ConcurrencyState: &fcpb.ConcurrencyState{
			Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1},
		},
	}

	require.True(t, limiter.reserve("slot-1", now))
	require.True(t, limiter.reserve("slot-1", now))
	require.Len(t, limiter.Slots, 1)
	require.False(t, limiter.reserve("slot-2", now))
	require.False(t, limiter.commit("slot-2"))

	require.True(t, limiter.commit("slot-1"))
	require.False(t, limiter.released([]string{"slot-1"}))
	limiter.cancelReservation("slot-1")
	require.Len(t, limiter.Slots, 1)

	limiter.release("slot-2")
	require.Len(t, limiter.Slots, 1)
	limiter.release("slot-1")
	require.Empty(t, limiter.Slots)
	require.True(t, limiter.released([]string{"slot-1"}))
	limiter.release("slot-1")
	require.Empty(t, limiter.Slots)
	require.Equal(t, int64(3), limiter.ReleaseCount)
	require.True(t, limiter.reserve("reserved", now))
	limiter.release("reserved")
	require.Empty(t, limiter.Slots)
}

func TestConcurrencyExpiredSlotIDCanBeReplaced(t *testing.T) {
	now := time.Now().UTC()
	limiter := &Component{
		ConcurrencyState: &fcpb.ConcurrencyState{
			Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1},
		},
	}

	require.True(t, limiter.reserve("expired-slot", now))
	limiter.expire(now.Add(reserveTimeout + time.Second))
	require.True(t, limiter.reserve("new-slot", now.Add(reserveTimeout+time.Second)))
	require.Len(t, limiter.Slots, 1)
	require.Equal(t, "new-slot", limiter.Slots[0].GetSlotId())
}

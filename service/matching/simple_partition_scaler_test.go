package matching

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/dynamicconfig"
	"google.golang.org/protobuf/types/known/anypb"
)

func newTestSimplePartitionScaler(settings *dynamicconfig.SimplePartitionScalerSettings) (*simplePartitionScaler, *clock.EventTimeSource) {
	ts := clock.NewEventTimeSource()
	ts.Update(time.Now())
	s := newSimplePartitionScaler(
		func() dynamicconfig.SimplePartitionScalerSettings { return *settings },
		ts,
	)
	// Pre-create trackers at t=0 so the "full interval" boundary aligns with
	// the number of ticks the test runs. (Production creates them lazily on
	// first OnTasks; this just shifts the start by one tick.)
	for _, d := range settings.Downs {
		_ = s.getTracker(d.Window)
	}
	for _, u := range settings.Ups {
		_ = s.getTracker(u.Window)
	}
	return s, ts
}

// runTicks advances the clock and calls OnTasks once per step at a constant
// tasksPerStep rate, threading state and target between calls (the same way
// scale_manager does). Returns the last decision plus the carried state and
// target.
func runTicks(
	s *simplePartitionScaler,
	ts *clock.EventTimeSource,
	step time.Duration,
	steps int,
	tasksPerStep int,
	target int,
	state *anypb.Any,
) (PartitionScalerDecision, *anypb.Any, int) {
	var dec PartitionScalerDecision
	for range steps {
		ts.Advance(step)
		dec = s.OnTasks(PartitionScalerInput{
			NumTasks:      tasksPerStep,
			CurrentTarget: target,
			PrivateState:  state,
		})
		if !dec.NoChange {
			state = dec.PrivateState
			target = dec.NewTarget
		}
	}
	return dec, state, target
}

func TestSimplePartitionScaler_Disabled(t *testing.T) {
	t.Parallel()
	s, _ := newTestSimplePartitionScaler(&dynamicconfig.SimplePartitionScalerSettings{
		Enabled: false,
	})
	dec := s.OnTasks(PartitionScalerInput{NumTasks: 100, CurrentTarget: 5})
	require.False(t, dec.NoChange)
	require.Equal(t, 0, dec.NewTarget)
}

func TestSimplePartitionScaler_Fixed(t *testing.T) {
	t.Parallel()
	// Fixed overrides everything else (Ups, Min, Max).
	s, _ := newTestSimplePartitionScaler(&dynamicconfig.SimplePartitionScalerSettings{
		Enabled: true,
		Fixed:   4,
		Ups:     []dynamicconfig.SimplePartitionScalerThreshold{{Window: 10 * time.Second, TargetRate: 10}},
		Min:     9,
		Max:     1,
	})
	dec := s.OnTasks(PartitionScalerInput{NumTasks: 100, CurrentTarget: 2})
	require.False(t, dec.NoChange)
	require.Equal(t, 4, dec.NewTarget)
}

func TestSimplePartitionScaler_UpsOnly(t *testing.T) {
	t.Parallel()
	// Production-like config: Ups but no Downs. Target can scale up with load
	// and never scales back down.
	s, ts := newTestSimplePartitionScaler(&dynamicconfig.SimplePartitionScalerSettings{
		Enabled: true,
		Ups:     []dynamicconfig.SimplePartitionScalerThreshold{{Window: 10 * time.Second, TargetRate: 10}},
	})

	// 30 tasks/s for a full window → rate ≈ 30 → target = 3.
	dec, state, target := runTicks(s, ts, time.Second, 10, 30, 1, nil)
	require.False(t, dec.NoChange)
	require.Equal(t, 3, dec.NewTarget)

	// Now drop to zero load for a full window. Without Downs the target must
	// not decrease.
	_, _, target = runTicks(s, ts, time.Second, 10, 0, target, state)
	require.Equal(t, 3, target)
}

func TestSimplePartitionScaler_NoChangeBeforeFullInterval(t *testing.T) {
	t.Parallel()
	s, ts := newTestSimplePartitionScaler(&dynamicconfig.SimplePartitionScalerSettings{
		Enabled: true,
		Ups:     []dynamicconfig.SimplePartitionScalerThreshold{{Window: 10 * time.Second, TargetRate: 10}},
	})

	// Nine 1s ticks: only 9s of data, less than the 10s window.
	dec, state, _ := runTicks(s, ts, time.Second, 9, 100, 0, nil)
	require.True(t, dec.NoChange)
	require.Nil(t, dec.PrivateState)

	// One more tick crosses 10s: now the window is full and a decision lands.
	dec, _, _ = runTicks(s, ts, time.Second, 1, 100, 0, state)
	require.False(t, dec.NoChange)
}

func TestSimplePartitionScaler_Hysteresis(t *testing.T) {
	t.Parallel()
	// Downs target rate (5) is below Ups target rate (10), creating a deadband:
	// once scaled up, a moderate drop in load (still above the down threshold
	// per partition) must not scale back down.
	s, ts := newTestSimplePartitionScaler(&dynamicconfig.SimplePartitionScalerSettings{
		Enabled: true,
		Ups:     []dynamicconfig.SimplePartitionScalerThreshold{{Window: 10 * time.Second, TargetRate: 10}},
		Downs:   []dynamicconfig.SimplePartitionScalerThreshold{{Window: 10 * time.Second, TargetRate: 5}},
	})

	// Phase 1: rate ≈ 30/s → target = 3.
	dec, state, target := runTicks(s, ts, time.Second, 10, 30, 1, nil)
	require.False(t, dec.NoChange)
	require.Equal(t, 3, dec.NewTarget)

	// Phase 2: rate drops to ≈ 20/s. Per-partition that's ~6.7, below the up
	// threshold (10) but above the down threshold (5). Target stays at 3.
	_, _, target = runTicks(s, ts, time.Second, 10, 20, target, state)
	require.Equal(t, 3, target)
}

func TestSimplePartitionScaler_Min(t *testing.T) {
	t.Parallel()
	// With no traffic the rate-based target would be 1; Min lifts it to 2.
	s, ts := newTestSimplePartitionScaler(&dynamicconfig.SimplePartitionScalerSettings{
		Enabled: true,
		Min:     2,
		Ups:     []dynamicconfig.SimplePartitionScalerThreshold{{Window: 10 * time.Second, TargetRate: 10}},
	})

	dec, _, _ := runTicks(s, ts, time.Second, 10, 0, 0, nil)
	require.False(t, dec.NoChange)
	require.Equal(t, 2, dec.NewTarget)
}

func TestSimplePartitionScaler_Max(t *testing.T) {
	t.Parallel()
	// 50 tasks/s would imply target=5; Max caps at 2.
	s, ts := newTestSimplePartitionScaler(&dynamicconfig.SimplePartitionScalerSettings{
		Enabled: true,
		Max:     2,
		Ups:     []dynamicconfig.SimplePartitionScalerThreshold{{Window: 10 * time.Second, TargetRate: 10}},
	})

	dec, _, _ := runTicks(s, ts, time.Second, 10, 50, 0, nil)
	require.False(t, dec.NoChange)
	require.Equal(t, 2, dec.NewTarget)
}

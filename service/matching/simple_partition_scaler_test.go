package matching

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/dynamicconfig"
)

func TestSimplePartitionScaler_Disabled(t *testing.T) {
	t.Parallel()
	ts := clock.NewEventTimeSource()
	ts.Update(time.Now())
	cfg := func() dynamicconfig.SimplePartitionScalerSettings {
		return dynamicconfig.SimplePartitionScalerSettings{Enabled: false}
	}
	s := newSimplePartitionScaler(cfg, ts)

	// When disabled and currentTarget != 0, should call setTarget(0)
	var called int
	s.OnTasks(10, 4, func(t int) { called = t })
	assert.Equal(t, 0, called)

	// When disabled and currentTarget == 0, should not call setTarget
	called = -1
	s.OnTasks(10, 0, func(t int) { called = t })
	assert.Equal(t, -1, called)
}

func TestSimplePartitionScaler_Fixed(t *testing.T) {
	t.Parallel()
	ts := clock.NewEventTimeSource()
	ts.Update(time.Now())
	cfg := func() dynamicconfig.SimplePartitionScalerSettings {
		return dynamicconfig.SimplePartitionScalerSettings{Enabled: true, Fixed: 5}
	}
	s := newSimplePartitionScaler(cfg, ts)

	// Should call setTarget(5) when currentTarget != 5
	var called int
	s.OnTasks(10, 1, func(t int) { called = t })
	assert.Equal(t, 5, called)

	// Should not call setTarget when currentTarget == 5
	called = -1
	s.OnTasks(10, 5, func(t int) { called = t })
	assert.Equal(t, -1, called)
}

func TestSimplePartitionScaler_ScaleUp(t *testing.T) {
	t.Parallel()
	ts := clock.NewEventTimeSource()
	ts.Update(time.Now())
	cfg := func() dynamicconfig.SimplePartitionScalerSettings {
		return dynamicconfig.SimplePartitionScalerSettings{
			Enabled: true,
			Ups: []dynamicconfig.SimplePartitionScalerThreshold{
				{Window: 10 * time.Second, TargetRate: 100},
			},
		}
	}
	s := newSimplePartitionScaler(cfg, ts)

	// Add 500 tasks/s over 10 seconds
	for i := 0; i < 100; i++ {
		s.OnTasks(50, 1, func(int) {})
		ts.Advance(100 * time.Millisecond)
	}

	// Rate should be ~500/s, target should be ceil(500/100) = 5
	var newTarget int
	s.OnTasks(1, 1, func(t int) { newTarget = t })
	assert.Equal(t, 5, newTarget)
}

func TestSimplePartitionScaler_ScaleDown(t *testing.T) {
	t.Parallel()
	ts := clock.NewEventTimeSource()
	ts.Update(time.Now())
	cfg := func() dynamicconfig.SimplePartitionScalerSettings {
		return dynamicconfig.SimplePartitionScalerSettings{
			Enabled: true,
			Downs: []dynamicconfig.SimplePartitionScalerThreshold{
				{Window: 10 * time.Second, TargetRate: 100},
			},
		}
	}
	s := newSimplePartitionScaler(cfg, ts)

	// Add 200 tasks/s over 10 seconds (starting from currentTarget=8)
	for i := 0; i < 100; i++ {
		s.OnTasks(20, 8, func(int) {})
		ts.Advance(100 * time.Millisecond)
	}

	// Rate ~200/s, at 100/s per partition → target = 2
	var newTarget int
	s.OnTasks(1, 8, func(t int) { newTarget = t })
	assert.Equal(t, 2, newTarget)
}

func TestSimplePartitionScaler_UpsOverrideDowns(t *testing.T) {
	t.Parallel()
	ts := clock.NewEventTimeSource()
	ts.Update(time.Now())
	cfg := func() dynamicconfig.SimplePartitionScalerSettings {
		return dynamicconfig.SimplePartitionScalerSettings{
			Enabled: true,
			Downs: []dynamicconfig.SimplePartitionScalerThreshold{
				// Downs wants to bring target down (target rate is high)
				{Window: 10 * time.Second, TargetRate: 200},
			},
			Ups: []dynamicconfig.SimplePartitionScalerThreshold{
				// Ups wants to bring target up (target rate is low)
				{Window: 10 * time.Second, TargetRate: 50},
			},
		}
	}
	s := newSimplePartitionScaler(cfg, ts)

	// Add 300 tasks/s over 10 seconds
	for i := 0; i < 100; i++ {
		s.OnTasks(30, 4, func(int) {})
		ts.Advance(100 * time.Millisecond)
	}

	// Downs: 300/200 ~ 2 → newTarget = min(4, 2) = 2
	// Ups: 300/50 = 6 → newTarget = max(2, 6) = 6
	// Ups wins
	var newTarget int
	s.OnTasks(1, 4, func(t int) { newTarget = t })
	assert.Equal(t, 6, newTarget)
}

func TestSimplePartitionScaler_NoChange(t *testing.T) {
	t.Parallel()
	ts := clock.NewEventTimeSource()
	ts.Update(time.Now())
	cfg := func() dynamicconfig.SimplePartitionScalerSettings {
		return dynamicconfig.SimplePartitionScalerSettings{
			Enabled: true,
			Downs: []dynamicconfig.SimplePartitionScalerThreshold{
				{Window: 10 * time.Second, TargetRate: 200},
			},
			Ups: []dynamicconfig.SimplePartitionScalerThreshold{
				{Window: 10 * time.Second, TargetRate: 100},
			},
		}
	}
	s := newSimplePartitionScaler(cfg, ts)

	// Add 400 tasks/s over 10 seconds, starting at target=4
	for i := 0; i < 100; i++ {
		s.OnTasks(40, 4, func(int) {})
		ts.Advance(100 * time.Millisecond)
	}

	// Downs: 400/200 = 2 → min(4, 2) = 2
	// Ups: 400/100 = 4 → max(2, 4) = 4
	// newTarget == currentTarget == 4, setTarget should not be called
	called := false
	s.OnTasks(1, 4, func(int) { called = true })
	assert.False(t, called)
}

func TestSimplePartitionScaler_MultipleWindows(t *testing.T) {
	t.Parallel()
	ts := clock.NewEventTimeSource()
	ts.Update(time.Now())
	cfg := func() dynamicconfig.SimplePartitionScalerSettings {
		return dynamicconfig.SimplePartitionScalerSettings{
			Enabled: true,
			Ups: []dynamicconfig.SimplePartitionScalerThreshold{
				// Long window with high target rate — by itself would suggest fewer partitions
				{Window: 10 * time.Second, TargetRate: 500},
				// Short window with low target rate — by itself would suggest more partitions
				{Window: 1 * time.Second, TargetRate: 50},
			},
		}
	}
	s := newSimplePartitionScaler(cfg, ts)

	// Sustained traffic: 500 tasks/s over 10 seconds
	for range 100 {
		s.OnTasks(50, 1, func(int) {})
		ts.Advance(100 * time.Millisecond)
	}

	// 10s window: ~500/s at 500/s per partition → target = 1
	// 1s window: ~500/s at 50/s per partition → target = 10
	// Ups takes the max, so 10 wins
	var newTarget int
	s.OnTasks(1, 1, func(t int) { newTarget = t })
	assert.GreaterOrEqual(t, newTarget, 5)
}

func TestSimplePartitionScaler_InvalidThresholds(t *testing.T) {
	t.Parallel()
	ts := clock.NewEventTimeSource()
	ts.Update(time.Now())
	cfg := func() dynamicconfig.SimplePartitionScalerSettings {
		return dynamicconfig.SimplePartitionScalerSettings{
			Enabled: true,
			Ups: []dynamicconfig.SimplePartitionScalerThreshold{
				{Window: 50 * time.Millisecond, TargetRate: 100}, // Window too small
				{Window: 10 * time.Second, TargetRate: 0},        // TargetRate too small
			},
		}
	}
	s := newSimplePartitionScaler(cfg, ts)

	// Add lots of tasks — but invalid thresholds should be skipped
	for i := 0; i < 100; i++ {
		s.OnTasks(100, 1, func(int) {})
		ts.Advance(100 * time.Millisecond)
	}

	// No valid thresholds → no change
	called := false
	s.OnTasks(1, 1, func(int) { called = true })
	assert.False(t, called)
}

func TestSimplePartitionScaler_MinTargetIsOne(t *testing.T) {
	t.Parallel()
	ts := clock.NewEventTimeSource()
	ts.Update(time.Now())
	cfg := func() dynamicconfig.SimplePartitionScalerSettings {
		return dynamicconfig.SimplePartitionScalerSettings{
			Enabled: true,
			Downs: []dynamicconfig.SimplePartitionScalerThreshold{
				{Window: 10 * time.Second, TargetRate: 1000},
			},
		}
	}
	s := newSimplePartitionScaler(cfg, ts)

	// Add 1 task/s — much lower than target rate
	for i := 0; i < 100; i++ {
		s.OnTasks(1, 8, func(int) {})
		ts.Advance(100 * time.Millisecond)
	}

	// Should scale down to 1, not 0
	var newTarget int
	s.OnTasks(1, 8, func(t int) { newTarget = t })
	assert.Equal(t, 1, newTarget)
}

func TestValidateSimplePartitionScalerThreshold(t *testing.T) {
	t.Parallel()
	assert.True(t, validateSimplePartitionScalerThreshold(dynamicconfig.SimplePartitionScalerThreshold{
		Window: 100 * time.Millisecond, TargetRate: 1,
	}))
	assert.False(t, validateSimplePartitionScalerThreshold(dynamicconfig.SimplePartitionScalerThreshold{
		Window: 99 * time.Millisecond, TargetRate: 1,
	}))
	assert.False(t, validateSimplePartitionScalerThreshold(dynamicconfig.SimplePartitionScalerThreshold{
		Window: 100 * time.Millisecond, TargetRate: 0,
	}))
}

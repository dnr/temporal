package matching

import (
	"sync"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/namespace"
)

type scalerFactoryCfg = dynamicconfig.TypedPropertyFnWithTaskQueueFilter[dynamicconfig.SimplePartitionScalerSettings]
type scalerCfg = dynamicconfig.TypedPropertyFn[dynamicconfig.SimplePartitionScalerSettings]

// simplePartitionScalerFactory creates simplePartitionScalers.
type simplePartitionScalerFactory struct {
	cfg scalerFactoryCfg
}

func newSimplePartitionScalerFactory(cfg scalerFactoryCfg) *simplePartitionScalerFactory {
	return &simplePartitionScalerFactory{cfg: cfg}
}

func (s *simplePartitionScalerFactory) New(
	nsName namespace.Name, tqName string, tqType enumspb.TaskQueueType,
) PartitionScaler {
	cfg := func() dynamicconfig.SimplePartitionScalerSettings { return s.cfg(nsName.String(), tqName, tqType) }
	return newSimplePartitionScaler(cfg, clock.NewRealTimeSource())
}

// simplePartitionScaler uses task add rates to scale partitions.
type simplePartitionScaler struct {
	cfg      scalerCfg
	ts       clock.TimeSource
	trackers sync.Map // time.Duration -> *taskTracker
}

func newSimplePartitionScaler(cfg scalerCfg, ts clock.TimeSource) *simplePartitionScaler {
	return &simplePartitionScaler{
		cfg: cfg,
		ts:  ts,
	}
}

func (s *simplePartitionScaler) getTracker(interval time.Duration) *taskTracker {
	if t, _ := s.trackers.Load(interval); t != nil {
		return t.(*taskTracker) //nolint:revive
	}
	newT := newTaskTracker(s.ts, interval/10, interval)
	t, _ := s.trackers.LoadOrStore(interval, newT)
	return t.(*taskTracker)
}

func (s *simplePartitionScaler) OnTasks(num, currentTarget int, setTarget func(newTarget int)) {
	cfg := s.cfg()

	if !cfg.Enabled {
		if currentTarget != 0 {
			setTarget(0)
		}
		return
	} else if cfg.Fixed > 0 {
		if currentTarget != cfg.Fixed {
			setTarget(cfg.Fixed)
		}
		return
	}

	// TODO: optimization: use one tracker and query it for different intervals.
	// TODO: clean up trackers that are unused after config change.
	s.trackers.Range(func(_, t any) bool {
		t.(*taskTracker).inc(num)
		return true
	})

	newTarget := currentTarget

	for _, down := range cfg.Downs {
		if !validateSimplePartitionScalerThreshold(down) {
			continue
		}
		rate := s.getTracker(down.Window).rate()
		// decrease target so that each partition is ~= target rate
		newTarget = max(1, min(
			newTarget,
			int(rate/float32(down.TargetRate)+0.5),
		))
	}

	for _, up := range cfg.Ups {
		if !validateSimplePartitionScalerThreshold(up) {
			continue
		}
		rate := s.getTracker(up.Window).rate()
		// increase target so that each partition is ~= target rate
		newTarget = max(
			newTarget,
			int(rate/float32(up.TargetRate)+0.5),
			1,
		)
	}

	if cfg.Min > 0 {
		newTarget = max(newTarget, cfg.Min)
	}
	if cfg.Max > 0 {
		newTarget = min(newTarget, cfg.Max)
	}

	if newTarget != currentTarget {
		setTarget(newTarget)
	}
}

func (s *simplePartitionScaler) Stop() {
}

func validateSimplePartitionScalerThreshold(t dynamicconfig.SimplePartitionScalerThreshold) bool {
	return t.Window >= 100*time.Millisecond && t.TargetRate >= 1
}

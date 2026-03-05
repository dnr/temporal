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
	return newSimplePartitionScaler(cfg)
}

// simplePartitionScaler uses task add rates to scale partitions.
type simplePartitionScaler struct {
	cfg scalerCfg

	lock     sync.Mutex
	trackers map[time.Duration]*taskTracker
}

func newSimplePartitionScaler(cfg scalerCfg) *simplePartitionScaler {
	return &simplePartitionScaler{
		cfg:      cfg,
		trackers: make(map[time.Duration]*taskTracker),
	}
}

func (s *simplePartitionScaler) getTrackerLocked(interval time.Duration) *taskTracker {
	if t := s.trackers[interval]; t != nil {
		return t
	}
	t := newTaskTracker(
		clock.NewRealTimeSource(),
		interval/20,
		interval,
	)
	s.trackers[interval] = t
	return t
}

func (s *simplePartitionScaler) OnTasks(num, currentTarget int, setTarget func(newTarget int)) {
	cfg := s.cfg()

	if !cfg.Enabled {
		if currentTarget != 0 {
			setTarget(0)
		}
		return
	}
	s.lock.Lock()

	// TODO: optimization: use one tracker and query it for different intervals.
	// TODO: clean up trackers that are unused after config change.
	for _, t := range s.trackers {
		t.inc(num)
	}

	// FIXME: do some rate limiting here

	newTarget := currentTarget

	for _, up := range cfg.Ups {
		rate := s.getTrackerLocked(up.Interval).rate()
		// increase target so that each partition is ~= threshold
		newTarget = max(
			newTarget,
			int(rate/float32(up.Threshold)+0.5),
		)
	}

	for _, down := range cfg.Downs {
		rate := s.getTrackerLocked(down.Interval).rate()
		// decrease target so that each partition is ~= threshold
		newTarget = min(
			newTarget,
			int(rate/float32(down.Threshold)+0.5),
		)
	}

	// unlock before setTarget
	s.lock.Unlock()

	if newTarget != currentTarget {
		setTarget(newTarget)
	}
}

func (s *simplePartitionScaler) Stop() {
}

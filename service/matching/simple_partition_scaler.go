package matching

import (
	enumspb "go.temporal.io/api/enums/v1"
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

func (s *simplePartitionScalerFactory) New(nsName namespace.Name, tqName string, tqType enumspb.TaskQueueType) PartitionScaler {
	cfg := func() dynamicconfig.SimplePartitionScalerSettings { return s.cfg(nsName.String(), tqName, tqType) }
	return newSimplePartitionScaler(cfg)
}

// simplePartitionScaler uses task add rates to scale partitions.
type simplePartitionScaler struct {
	cfg scalerCfg
}

func newSimplePartitionScaler(cfg scalerCfg) *simplePartitionScaler {
	return &simplePartitionScaler{cfg: cfg}
}

func (s *simplePartitionScaler) OnTask(currentTarget int, setTarget func(newTarget int)) {
	panic("not implemented") // FIXME: Implement
}

func (s *simplePartitionScaler) Stop() {
}

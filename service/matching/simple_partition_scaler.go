package matching

// simplePartitionScalerFactory creates simplePartitionScalers.
type simplePartitionScalerFactory struct {
}

func newSimplePartitionScalerFactory() *simplePartitionScalerFactory {
	return &simplePartitionScalerFactory{}
}

func (s *simplePartitionScalerFactory) New() PartitionScaler {
	return newSimplePartitionScaler()
}

// simplePartitionScaler uses task add rates to scale partitions.
type simplePartitionScaler struct {
}

func newSimplePartitionScaler() *simplePartitionScaler {
	return &simplePartitionScaler{}
}

func (s *simplePartitionScaler) OnTask(currentTarget int, setTarget func(newTarget int)) {
	panic("not implemented") // FIXME: Implement
}

func (s *simplePartitionScaler) Stop() {
}

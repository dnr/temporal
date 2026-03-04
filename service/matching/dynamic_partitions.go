package matching

import (
	"math/bits"
)

type PartitionScalerFactory interface {
	// New will be called for a new partition. It should return a new PartitionScaler
	// (or nil to disable).
	New() PartitionScaler
}

type PartitionScaler interface {
	// OnTask will be called once per task added, either sync match or async.
	// It will also be given the current partition count target. If it wants to change the
	// target, it should call setTarget with the new target. Changes may be rejected if called
	// too often or the changes are too large.
	OnTask(currentTarget int, setTarget func(newTarget int))
	// Stop will be called when unloading the partition.
	Stop()
}

type simplePartitionScalerFactory struct {
}

type simplePartitionScaler struct {
}

func newSimplePartitionScalerFactory() *simplePartitionScalerFactory {
	return &simplePartitionScalerFactory{}
}

func (s *simplePartitionScalerFactory) New() PartitionScaler {
	return newSimplePartitionScaler()
}

func newSimplePartitionScaler() *simplePartitionScaler {
	return &simplePartitionScaler{}
}

func (s *simplePartitionScaler) OnTask(currentTarget int, setTarget func(newTarget int)) {
	panic("not implemented") // TODO: Implement
}

func (s *simplePartitionScaler) Stop() {
}

func readPartitionsFromBacklogState(state []uint64) int32 {
	i := len(state) - 1
	if i < 0 {
		return 0
	}
	return int32(bits.Len64(state[i]) - 1 + i*64)
}

package matching

import (
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/server/common/namespace"
)

// PartitionScalerFactory is a pluggable interface to control partition scaling.
type PartitionScalerFactory interface {
	// New will be called for a new root partition. It should return a new PartitionScaler
	// (or nil to disable).
	New(nsName namespace.Name, tqName string, tqType enumspb.TaskQueueType) PartitionScaler
}

// PartitionScaler is an instance of a scaler for one task queue.
type PartitionScaler interface {
	// OnTask will be called once per task added, either sync match or async.
	// It will also be given the current partition count target. If it wants to change the
	// target, it should call setTarget with the new target. Changes may be rejected if called
	// too often or the changes are too large.
	// Setting target to zero will disable dynamic partition scaling.
	OnTask(currentTarget int, setTarget func(newTarget int))
	// Stop will be called when unloading the partition.
	Stop()
}

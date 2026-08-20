package flowcontrol

import (
	"slices"
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/chasm"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FIXME: move to dynamic config
const reserveTimeout = 30 * time.Second

type concurrency struct {
	chasm.UnimplementedComponent

	*fcpb.ConcurrencyState
}

func (c *concurrency) LifecycleState(_ chasm.Context) chasm.LifecycleState {
	// TODO(fc): we should be able to clean these up if they've been idle a while
	return chasm.LifecycleStateRunning
}

func (c *concurrency) Terminate(_ chasm.MutableContext, _ chasm.TerminateComponentRequest) (chasm.TerminateComponentResponse, error) {
	// TODO(fc): can we block this if there are any committed slots?
	return chasm.TerminateComponentResponse{}, nil
}

func (c *concurrency) ContextMetadata(_ chasm.Context) map[string]string {
	return nil
}

func (c *concurrency) expire(now time.Time) {
	nowSec := now.Unix() + 1
	c.Slots = slices.DeleteFunc(c.Slots, func(slot *fcpb.ConcurrencySlot) bool {
		return !slot.Committed && slot.Expires != nil && slot.Expires.Seconds < nowSec
	})
}

func (c *concurrency) find(taskUUID string) int {
	return slices.IndexFunc(c.Slots, func(slot *fcpb.ConcurrencySlot) bool {
		return slot.TaskUuid == taskUUID
	})
}

func (c *concurrency) availableSlots() int32 {
	return max(0, c.Limit-int32(len(c.Slots)))
}

func (c *concurrency) maintainGeneration() func() {
	oldPred := c.availableSlots() > 0
	return func() {
		newPred := c.availableSlots() > 0
		if oldPred && !newPred {
			// Increment Generation on any transition where Wait would have returned
			// immediately before but would not now.
			c.Generation++
		}
	}
}

func (c *concurrency) reserve(taskUUID string, now time.Time) int32 {
	defer c.maintainGeneration()()
	c.expire(now)

	if c.find(taskUUID) >= 0 {
		return 1 // already reserved or committed, accept
	}
	if int32(len(c.Slots)) >= c.Limit {
		return 0
	}
	c.Slots = append(c.Slots, &fcpb.ConcurrencySlot{
		TaskUuid:  taskUUID,
		Committed: false,
		// Truncate is okay because expire adds a second anyway.
		Expires: timestamppb.New(now.Add(reserveTimeout).Truncate(time.Second)),
	})
	return 1
}

func (c *concurrency) cancelReservation(taskUUID string, now time.Time) {
	defer c.maintainGeneration()()
	c.expire(now)

	c.Slots = slices.DeleteFunc(c.Slots, func(slot *fcpb.ConcurrencySlot) bool {
		return !slot.Committed && slot.TaskUuid == taskUUID
	})
}

func (c *concurrency) commit(taskUUID string, now time.Time) error {
	defer c.maintainGeneration()()
	c.expire(now)

	if idx := c.find(taskUUID); idx >= 0 {
		// note this works even if it was already committed
		c.Slots[idx].Committed = true
		c.Slots[idx].Expires = nil
		return nil
	}

	// TODO(fc): consider fast-recover if free slots
	return serviceerror.NewFailedPrecondition("no reservation found")
}

func (c *concurrency) release(taskUUID string, now time.Time) {
	defer c.maintainGeneration()()
	c.expire(now)

	c.Slots = slices.DeleteFunc(c.Slots, func(slot *fcpb.ConcurrencySlot) bool {
		return slot.Committed && slot.TaskUuid == taskUUID
	})
}

func (c *concurrency) notifyWaiters(now time.Time) int32 {
	defer c.maintainGeneration()()
	c.expire(now)

	// FIXME: do slow release of notifications
	return c.availableSlots()
}

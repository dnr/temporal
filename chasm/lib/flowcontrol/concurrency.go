package flowcontrol

import (
	"slices"
	"time"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/server/chasm"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FIXME: move to dynamic config
const reserveTimeout = 30 * time.Second

const initialConcurrencyLimit = int32(1_000_000)

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

func expiredReservation(slot *fcpb.ConcurrencyState_Slot, nowSec int64) bool {
	return !slot.Committed && slot.Expires != nil && slot.Expires.Seconds < nowSec
}

func (c *concurrency) expire(now time.Time) {
	nowSec := now.Unix() + 1
	c.Slots = slices.DeleteFunc(c.Slots, func(slot *fcpb.ConcurrencyState_Slot) bool {
		return expiredReservation(slot, nowSec)
	})
}

func (c *concurrency) find(slotID string) int {
	return slices.IndexFunc(c.Slots, func(slot *fcpb.ConcurrencyState_Slot) bool {
		return slot.SlotId == slotID
	})
}

func (c *concurrency) availableSlots() int32 {
	return max(0, c.Config.ConcurrentTasks-int32(len(c.Slots)))
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

func (c *concurrency) updateConfig(config *taskqueuepb.ConcurrencyLimit, version int64) {
	if config != nil && version > c.ConfigVersion {
		c.Config = config
	}
}

func (c *concurrency) reserve(slotID string, now time.Time) bool {
	// reserve is the only method that can increment generation, when it takes the last slot
	defer c.maintainGeneration()()

	if c.find(slotID) >= 0 {
		return true // already reserved or committed, accept
	}
	if int32(len(c.Slots)) >= c.Config.ConcurrentTasks {
		return false
	}
	c.Slots = append(c.Slots, &fcpb.ConcurrencyState_Slot{
		SlotId:    slotID,
		Committed: false,
		// Truncate is okay because expire adds a second anyway.
		Expires: timestamppb.New(now.Add(reserveTimeout).Truncate(time.Second)),
	})
	return true
}

func (c *concurrency) cancelReservation(slotID string) {
	c.Slots = slices.DeleteFunc(c.Slots, func(slot *fcpb.ConcurrencyState_Slot) bool {
		return !slot.Committed && slot.SlotId == slotID
	})
}

func (c *concurrency) commit(slotID string) bool {
	if idx := c.find(slotID); idx >= 0 {
		// note this works even if it was already committed
		c.Slots[idx].Committed = true
		c.Slots[idx].Expires = nil
		return true
	}

	// TODO(fc): consider fast-recover if free slots
	return false
}

func (c *concurrency) release(slotID string) {
	c.Slots = slices.DeleteFunc(c.Slots, func(slot *fcpb.ConcurrencyState_Slot) bool {
		return slot.Committed && slot.SlotId == slotID
	})
}

// notifyWaiters is called from PollComponent and should not modify the state.
func (c *concurrency) notifyWaiters(now time.Time) int32 {
	nowSec := now.Unix() + 1
	usedSlots := int32(0)
	for _, slot := range c.Slots {
		if !expiredReservation(slot, nowSec) {
			usedSlots++
		}
	}
	// FIXME: do slow release of notifications
	return max(0, c.Config.ConcurrentTasks-usedSlots)
}

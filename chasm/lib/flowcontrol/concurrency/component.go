package concurrency

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

const initialLimit = int32(1_000_000)

type Component struct {
	chasm.UnimplementedComponent

	*fcpb.ConcurrencyState
}

func (c *Component) LifecycleState(_ chasm.Context) chasm.LifecycleState {
	// TODO(fc): we should be able to clean these up if they've been idle a while
	return chasm.LifecycleStateRunning
}

func (c *Component) Terminate(_ chasm.MutableContext, _ chasm.TerminateComponentRequest) (chasm.TerminateComponentResponse, error) {
	// TODO(fc): can we block this if there are any committed slots?
	return chasm.TerminateComponentResponse{}, nil
}

func (c *Component) ContextMetadata(_ chasm.Context) map[string]string {
	return nil
}

func expiredReservation(slot *fcpb.ConcurrencyState_Slot, nowSec int64) bool {
	return !slot.Committed && slot.Expires != nil && slot.Expires.Seconds < nowSec+1
}

func (c *Component) expire(now time.Time) {
	nowSec := now.Unix()
	c.Slots = slices.DeleteFunc(c.Slots, func(slot *fcpb.ConcurrencyState_Slot) bool {
		return expiredReservation(slot, nowSec)
	})
}

func (c *Component) find(slotID string) int {
	return slices.IndexFunc(c.Slots, func(slot *fcpb.ConcurrencyState_Slot) bool {
		return slot.SlotId == slotID
	})
}

func (c *Component) availableSlots() int32 {
	return max(0, c.Config.ConcurrentTasks-int32(len(c.Slots)))
}

func (c *Component) maintainGeneration() func() {
	oldPred := c.availableSlots() > 0
	return func() {
		newPred := c.availableSlots() > 0
		if oldPred && !newPred {
			// Increment Generation on any transition where Wait would have returned
			// immediately before but would not now.
			c.Generation++
			c.WakeUpTo = 0
			c.WakeAll = false
		}
	}
}

func (c *Component) updateConfig(config *taskqueuepb.ConcurrencyLimit, version int64) {
	if config != nil && version > c.ConfigVersion {
		c.Config = config
	}
}

func (c *Component) reserve(slotID string, now time.Time) bool {
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

func (c *Component) cancelReservation(slotID string) {
	c.Slots = slices.DeleteFunc(c.Slots, func(slot *fcpb.ConcurrencyState_Slot) bool {
		return !slot.Committed && slot.SlotId == slotID
	})
}

func (c *Component) commit(slotID string) bool {
	if idx := c.find(slotID); idx >= 0 {
		// note this works even if it was already committed
		c.Slots[idx].Committed = true
		c.Slots[idx].Expires = nil
		return true
	}

	// TODO(fc): consider fast-recover if free slots
	return false
}

func (c *Component) release(slotID string) {
	c.Slots = slices.DeleteFunc(c.Slots, func(slot *fcpb.ConcurrencyState_Slot) bool {
		return slot.Committed && slot.SlotId == slotID
	})
}

// pollFreeSlots is called from PollComponent and should not modify the state.
func (c *Component) pollFreeSlots(now time.Time) int32 {
	nowSec := now.Unix()
	usedSlots := int32(0)
	for _, slot := range c.Slots {
		if !expiredReservation(slot, nowSec) {
			usedSlots++
		}
	}
	return max(0, c.Config.ConcurrentTasks-usedSlots)
}

// poll is called from PollComponent and should not modify the state.
func (c *Component) poll(now time.Time, reqGeneration int64, reqStartTime int64, reqTokens int32) (int64, int32, bool) {
	if reqGeneration > c.Generation || // too new generation
		reqGeneration == c.Generation && // waiting on this generation
			!c.WakeAll && // doing staged wake
			reqStartTime > c.WakeUpTo { // this one isn't ready yet
		return 0, 0, false // wait for another transition
	}
	// allow as many as we have available
	tokens := min(c.pollFreeSlots(now), reqTokens)
	return c.Generation, tokens, true
}

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

func (c *concurrency) full() bool {
	return len(c.Slots) >= int(c.Limit)
}

func (c *concurrency) reserve(taskUUID string, now time.Time) error {
	c.expire(now)

	if c.find(taskUUID) >= 0 {
		return nil // already reserved or committed, accept
	}
	if c.full() {
		return serviceerror.NewFailedPreconditionf("limit of %d slots reached", c.Limit)
	}
	c.Slots = append(c.Slots, &fcpb.ConcurrencySlot{
		TaskUuid:  taskUUID,
		Committed: false,
		Expires:   timestamppb.New(now.Add(reserveTimeout)),
	})
	return nil
}

func (c *concurrency) cancelReservation(taskUUID string, now time.Time) error {
	c.expire(now)

	c.Slots = slices.DeleteFunc(c.Slots, func(slot *fcpb.ConcurrencySlot) bool {
		return !slot.Committed && slot.TaskUuid == taskUUID
	})
	return nil
}

func (c *concurrency) commit(taskUUID string, now time.Time) error {
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

func (c *concurrency) release(taskUUID string, now time.Time) error {
	c.expire(now)

	c.Slots = slices.DeleteFunc(c.Slots, func(slot *fcpb.ConcurrencySlot) bool {
		return slot.Committed && slot.TaskUuid == taskUUID
	})
	// FIXME: wake subscribers
	return nil
}

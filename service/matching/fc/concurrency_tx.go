package fc

import (
	"context"
	"time"

	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/namespace"
	serviceerrors "go.temporal.io/server/common/serviceerror"
)

var ErrConcurrencyBlocked = serviceerror.NewFailedPrecondition("blocked by concurrency limit")

type concurrencyTx struct {
	client fcpb.ConcurrencyServiceClient
	r      *Readiness
	nsID   namespace.ID
	slotID string
	lim    Limiter
	pri    int32
	age    time.Time
}

func newConcurrencyTx(
	client fcpb.ConcurrencyServiceClient,
	r *Readiness, // FIXME: whole thing?
	nsID namespace.ID,
	slotID string,
	lim Limiter,
	pri int32,
	age time.Time,
) *concurrencyTx {
	return &concurrencyTx{
		client: client,
		r:      r,
		nsID:   nsID,
		slotID: slotID,
		lim:    lim,
		pri:    pri,
		age:    age,
	}
}

func (c *concurrencyTx) check(cb ReadinessCallback) error {
	n := c.r.getNS(c.nsID)

	n.lock.Lock()
	defer n.lock.Unlock()

	cs, ok := n.concurrencyLimiters[c.lim.Key]
	if !ok {
		// if missing from cache, pass check. matching will probably try to Reserve and
		// based on result, call either reportConcurrencyReady or reportConcurrencyBlocked.
		return nil
	}

	blocked := cs.tokens == 0
	if cb != nil {
		// add callback if blocked, remove if unblocked
		if blocked {
			cs.waiters.add(cb, c.pri, c.age)
		} else {
			cs.waiters.remove(cb)
		}
		cs.syncGoroLocked(n, c.lim.Key)
	}

	if blocked {
		return ErrConcurrencyBlocked
	}
	return nil
}

func (c *concurrencyTx) cancelCheck(cb ReadinessCallback) {
	// FIXME
	return
}

func (c *concurrencyTx) reserve(ctx context.Context) error {
	// if config is missing or wrong type, just leave it out
	configUpdate, _ := c.lim.Config.(*taskqueuepb.ConcurrencyLimit)

	res, err := c.client.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId:         c.nsID.String(),
		Key:                 c.lim.Key,
		ReserveSlots:        []string{c.slotID},
		ConfigUpdate:        configUpdate,
		ConfigUpdateVersion: c.lim.ConfigVersion,
	})
	if err != nil {
		return err // don't update cache on rpc error
	}
	if !res.ReserveSuccess[0] {
		c.r.reportConcurrencyBlocked(c.nsID, c.lim.Key, res.Generation)
		return serviceerrors.NewFlowControlBlocked()
	}
	// TODO(fc): we could include a hint for how many slots are _remaining_, and if zero, mark
	// this limiter as blocked in the cache. but we don't want to immediately Wait on it since
	// we might not have another waiter yet.
	c.r.reportConcurrencyReady(c.nsID, c.lim.Key, res.Generation, 0) // FIXME: 0?
	return nil
}

func (c *concurrencyTx) commit(ctx context.Context) error {
	res, err := c.client.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId: c.nsID.String(),
		Key:         c.lim.Key,
		CommitSlots: []string{c.slotID},
	})
	if err != nil {
		return err
	} else if !res.CommitSuccess[0] {
		return errCommitFailure
	}
	return err
}

func (c *concurrencyTx) cancelReserve(ctx context.Context) {
	// call in new goroutine, don't block here, we don't care about the result
	go c.client.Batch(ctx, &fcpb.ConcurrencyBatchRequest{
		NamespaceId:            c.nsID.String(),
		Key:                    c.lim.Key,
		CancelReservationSlots: []string{c.slotID},
	})
}

package fc

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"go.temporal.io/api/serviceerror"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/namespace"
)

var errCommitFailure = serviceerror.NewFailedPrecondition("commit failed")

// A client of the flow control system (e.g. matchingEngine) should call NewTx when it wants to
// perform the flow control commit protocol. It should then call:
// - tx.Reserve() -> on error call tx.CancelReservations() and retry the task
// - tx.LimiterRefs() to get refs to pass to history (for releasing later)
// - history.RecordTaskStarted(refs) -> on error call tx.CancelReservations()
// - tx.Commit() -> on error call tx.CancelReservations() and DROP the task
// Methods on Tx must not be called concurrently.
func (r *Readiness) NewTx(ctx context.Context, nsID namespace.ID, task fcTask) (*Tx, error) {
	lims := canonicalLimiters(task)
	if len(lims) == 0 {
		return nil, nil
	}

	committers := make([]committer, len(lims))
	var refs []*taskqueuespb.LimiterRef
	for i, lim := range lims {
		switch lim.Type {
		case enumsspb.LIMITER_TYPE_CONCURRENCY:
			// TODO(fc): consider deriving from task to fix some nongraceful failover situations?
			slotID := uuid.NewString()
			committers[i] = newConcurrencyCommitter(ctx, r.concurrencyServiceClient, r, nsID, slotID, lim)
			refs = append(refs, &taskqueuespb.LimiterRef{LimiterType: lim.Type, Key: lim.Key, SlotId: slotID})
		case enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT:
			committers[i] = newLocalLimiterCommitter(r, nsID, lim)
		default:
			return nil, errors.New("invalid limiter type")
		}
	}

	return &Tx{
		readiness:  r,
		committers: committers,
		refs:       refs,
	}, nil
}

// Holds state for an invocation of the flow control commit protocol. See Readiness.NewTx.
type Tx struct {
	readiness  *Readiness
	committers []committer
	refs       []*taskqueuespb.LimiterRef
	state      [MaxLimiters]txState
}

type committer interface {
	reserve() error
	commit() error
	cancelReservations()
}

type txState int8 // just so we can pack these in an array

const (
	txStateInit txState = iota
	txStateReserved
	txStateCommitted
	txStateCommitFailed
	txStateCanceled
)

// LimiterRefs returns references to limiters that should be passed to history and stored with
// activity/workflow task state.
func (tx *Tx) LimiterRefs() []*taskqueuespb.LimiterRef {
	if tx == nil {
		return nil
	}
	return tx.refs
}

// Reserve checks that all limiters can be satisfied now.
func (tx *Tx) Reserve() error {
	if tx == nil {
		return nil // no limiters
	}
	// reserve must be sequential
	for i, com := range tx.committers {
		if err := com.reserve(); err != nil {
			return err
		}
		tx.state[i] = txStateReserved
	}
	return nil
}

// Commit turns reservations into committed slots.
func (tx *Tx) Commit() error {
	if tx == nil {
		return nil // no limiters
	}

	n := len(tx.committers)

	errC := make(chan error, n)
	commit := func(i int) {
		err := tx.committers[i].commit()
		if err != nil {
			tx.state[i] = txStateCommitFailed
		} else {
			tx.state[i] = txStateCommitted
		}
		errC <- err
	}

	// commit all concurrently and record success/failures
	for i := range n - 1 {
		go commit(i + 1)
	}
	commit(0)

	errs := make([]error, n)
	for i := range n {
		errs[i] = <-errC
	}
	return errors.Join(errs...)
}

// CancelReservations cancels reservations on any limiters that have been made and not
// committed so far.
func (tx *Tx) CancelReservations() {
	// this is best-effort, reservations have timeouts so it's okay if we fail to cancel
	if tx == nil {
		return // no limiters
	}
	for i, com := range tx.committers {
		if tx.state[i] == txStateReserved {
			com.cancelReservations()
			tx.state[i] = txStateCanceled
		}
	}
}

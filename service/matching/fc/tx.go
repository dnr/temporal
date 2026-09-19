package fc

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	"go.temporal.io/api/serviceerror"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/namespace"
)

var errCommitFailure = serviceerror.NewFailedPrecondition("commit failed")
var errInvalidTxState = serviceerror.NewInternal("invalid fc tx state")

// A client of the flow control system should call NewTx when it it's ready to match a task, to
// perform the flow control commit protocol. It should then call:
//   - tx.Check(cb) -> checks readiness, if not ready atomically registers cb to be called when
//     possibly ready
//   - tx.Reserve(ctx) -> on error retry the task
//   - tx.LimiterRefs() to get refs to pass to history (for releasing later)
//   - history.RecordTaskStarted(refs) -> on error call tx.Rollback()
//   - tx.Commit(ctx) -> on error DROP the task
//
// Tx is not safe for concurrent use.
func (r *Readiness) NewTx(nsID namespace.ID, task fcTask, cb ReadinessCallback) *Tx {
	lims := canonicalLimiters(task)
	if len(lims) == 0 {
		return nil
	}

	limiterTxs := make([]limiterTx, len(lims))
	var refs []*taskqueuespb.LimiterRef
	for i, lim := range lims {
		switch lim.Type {
		case enumsspb.LIMITER_TYPE_CONCURRENCY:
			// TODO(fc): consider deriving from task to fix some nongraceful failover situations
			slotID := uuid.NewString()
			pri, age := task.PriorityAndAge()
			limiterTxs[i] = newConcurrencyTx(r.concurrencyServiceClient, r, nsID, slotID, lim, pri, age)
			refs = append(refs, &taskqueuespb.LimiterRef{LimiterType: lim.Type, Key: lim.Key, SlotId: slotID})
		case enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT:
			limiterTxs[i] = newLocalLimiterTx(r, nsID, lim)
		default:
			// TODO(fc): log or notify here
			limiterTxs[i] = noopLimiterTx{}
		}
	}

	return &Tx{
		readiness: r,
		limiters:  limiterTxs,
		refs:      refs,
		cb:        cb,
	}
}

// Holds state for an invocation of the flow control commit protocol. See Readiness.NewTx.
type Tx struct {
	readiness *Readiness
	limiters  []limiterTx
	refs      []*taskqueuespb.LimiterRef
	cb        ReadinessCallback
	state     [MaxLimiters]txState
}

type limiterTx interface {
	check(ReadinessCallback) error
	cancelCheck(ReadinessCallback)
	reserve(context.Context) error
	commit(context.Context) error
	cancelReserve(context.Context)
}

type txState int8 // just so we can pack these in an array

const (
	txStateInit txState = iota
	txStateChecked
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

// Check checks and consumes local tokens for each limiter.
// // ReadinessState gets the readiness state of a limiter. If it's blocked and cb is not nil,
// // cb.OnReady will be called once when the state of the limiter transitions to ready. If it is
// // ready, the callback will be removed from the limiter
// FIXME: comment more here
func (tx *Tx) Check() (retErr error) {
	if tx == nil {
		return nil // no limiters
	}
	defer func() {
		if retErr != nil {
			tx.Rollback(nil)
		}
	}()

	for i, lim := range tx.limiters {
		if tx.state[i] != txStateInit {
			return errInvalidTxState
		}
		if err := lim.check(tx.cb); err != nil {
			return err
		}
		tx.state[i] = txStateChecked
	}
	return nil
}

// Reserve checks that all limiters can be satisfied now.
// Reserve may make RPC calls.
func (tx *Tx) Reserve(ctx context.Context) (retErr error) {
	if tx == nil {
		return nil // no limiters
	}
	defer func() {
		if retErr != nil {
			tx.Rollback(ctx)
		}
	}()

	// reserve must be sequential
	for i, lim := range tx.limiters {
		if tx.state[i] != txStateChecked {
			return errInvalidTxState
		}
		if err := lim.reserve(ctx); err != nil {
			return err
		}
		tx.state[i] = txStateReserved
	}
	return nil
}

// Commit turns reservations into committed slots.
// Reserve may make RPC calls.
func (tx *Tx) Commit(ctx context.Context) (retErr error) {
	if tx == nil {
		return nil // no limiters
	}
	defer func() {
		if retErr != nil {
			tx.Rollback(ctx)
		}
	}()

	// commit all concurrently
	var wg sync.WaitGroup
	errs := make([]error, len(tx.limiters))
	for i, lim := range tx.limiters {
		if tx.state[i] != txStateReserved {
			return errInvalidTxState
		}
		if i == len(tx.limiters)-1 {
			errs[i] = lim.commit(ctx)
		} else {
			wg.Go(func() { errs[i] = lim.commit(ctx) })
		}
	}
	wg.Wait()

	return errors.Join(errs...)
}

// Rollback cancels checks and reservations on any limiters that have been made and not
// committed so far. ctx may be nil only if Reserve has not been called yet.
func (tx *Tx) Rollback(ctx context.Context) {
	if tx == nil {
		return // no limiters
	}
	for i, lim := range tx.limiters {
		switch tx.state[i] {
		case txStateReserved:
			// this is best-effort, reservations have timeouts so it's okay if we fail to cancel
			lim.cancelReserve(ctx)
			fallthrough
		case txStateChecked:
			lim.cancelCheck(tx.cb)
			tx.state[i] = txStateCanceled
		}
	}
}

type noopLimiterTx struct{}

func (noopLimiterTx) check(cb ReadinessCallback) error { return nil }
func (noopLimiterTx) cancelCheck(cb ReadinessCallback) {}
func (noopLimiterTx) reserve(context.Context) error    { return nil }
func (noopLimiterTx) commit(context.Context) error     { return nil }
func (noopLimiterTx) cancelReserve(context.Context)    {}

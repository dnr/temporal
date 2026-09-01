package fc

import (
	"context"
	"errors"

	"github.com/google/uuid"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/namespace"
)

func (r *Readiness) NewTx(ctx context.Context, nsID namespace.ID, task fcTask) (*tx, error) {
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
			committers[i] = newLocalLimiterCommitter(lim)
		default:
			return nil, errors.New("invalid limiter type")
		}
	}

	return &tx{
		readiness:  r,
		committers: committers,
		refs:       refs,
	}, nil
}

type tx struct {
	readiness  *Readiness
	committers []committer
	refs       []*taskqueuespb.LimiterRef
	state      [MaxLimiters]txState
}

type txState int8 // just so we can pack these in an array

const (
	txStateInit txState = iota
	txStateReserved
	txStateCommitted
	txStateCommitFailed
	txStateCanceled
)

func (tx *tx) LimiterRefs() []*taskqueuespb.LimiterRef {
	if tx == nil {
		return nil
	}
	return tx.refs
}

func (tx *tx) Reserve() error {
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

func (tx *tx) Commit() error {
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

func (tx *tx) CancelReservations() {
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

package matching

import (
	"context"
	"errors"

	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/namespace"
)

type fcTxState int8

const (
	fcTxStateInit fcTxState = iota
	fcTxStateReserved
	fcTxStateCommitted
	fcTxStateCommitFailed
	fcTxStateCanceled
)

type fcReadiness struct {
	concurrencyServiceClient fcpb.ConcurrencyServiceClient
}

type fcTx struct {
	readiness  *fcReadiness
	committers []fcCommitter
	refs       []*taskqueuespb.LimiterRef
	state      [maxLimiters]fcTxState
}

func newFcReadiness(
	concurrencyServiceClient fcpb.ConcurrencyServiceClient,
) *fcReadiness {
	return &fcReadiness{
		concurrencyServiceClient: concurrencyServiceClient,
	}
}

func (r *fcReadiness) NewTx(ctx context.Context, nsID namespace.ID, task *internalTask) (*fcTx, error) {
	lims := canonicalLimiters(task)
	if len(lims) == 0 {
		return nil, nil
	}

	committers := make([]fcCommitter, len(lims))
	refs := make([]*taskqueuespb.LimiterRef, len(lims))
	for i, lim := range lims {
		switch lim.tp {
		case enumsspb.LIMITER_TYPE_CONCURRENCY:
			committers[i] = newFcConcurrencyCommitter(ctx, r.concurrencyServiceClient, nsID, task, lim.key)
			refs[i] = &taskqueuespb.LimiterRef{LimiterType: lim.tp, Key: lim.key}
		default:
			return nil, errors.New("invalid limiter type")
		}
	}

	return &fcTx{
		readiness:  r,
		committers: committers,
		refs:       refs,
	}, nil
}

func (tx *fcTx) LimiterRefs() []*taskqueuespb.LimiterRef {
	return tx.refs
}

func (tx *fcTx) Reserve() error {
	if tx == nil {
		return nil // no limiters
	}
	// reserve must be sequential
	for i, com := range tx.committers {
		if err := com.Reserve(); err != nil {
			return err
		}
		tx.state[i] = fcTxStateReserved
	}
	return nil
}

func (tx *fcTx) CancelReservations() {
	// this is best-effort, reservations have timeouts so it's okay if we fail to cancel
	if tx == nil {
		return // no limiters
	}
	for i, com := range tx.committers {
		if tx.state[i] == fcTxStateReserved {
			com.CancelReservations()
			tx.state[i] = fcTxStateCanceled
		}
	}
}

func (tx *fcTx) Commit() error {
	if tx == nil {
		return nil // no limiters
	}
	// TODO(fc): we can call Commit concurrently on all limiters
	// note: rate limiters don't have to be Committed
	var errs []error
	for i, com := range tx.committers {
		if err := com.Commit(); err != nil {
			errs = append(errs, err)
			tx.state[i] = fcTxStateCommitFailed
		} else {
			tx.state[i] = fcTxStateCommitted
		}
	}
	return errors.Join(errs...)
}

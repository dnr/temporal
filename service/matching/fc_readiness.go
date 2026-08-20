package matching

import (
	"context"
	"errors"
	"sync"

	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/cache"
	"go.temporal.io/server/common/goro"
	"go.temporal.io/server/common/namespace"
)

type fcReadinessState int32

const (
	fcReadinessUnknown fcReadinessState = iota
	fcReadinessBlocked
	fcReadinessReady
)

type fcReadinessCacheKey struct {
	nsID namespace.ID
	key  string
}

type fcReadinessCacheValue struct {
	generation int64
	state      fcReadinessState
	// TODO(fc): cache some limiter-specific state?
}

type fcReadiness struct {
	concurrencyServiceClient fcpb.ConcurrencyServiceClient
	// TODO(fc): consider namespace isolation in this cache, probably separate caches per ns
	cache cache.StoppableCache

	waitersLock sync.Mutex
	waiters     map[fcReadinessCacheKey]struct{}

	goros *goro.KeyedSet[fcReadinessCacheKey]

	cancelGoros context.CancelFunc
}

func newFcReadiness(
	concurrencyServiceClient fcpb.ConcurrencyServiceClient,
) *fcReadiness {
	return &fcReadiness{
		concurrencyServiceClient: concurrencyServiceClient,
		// FIXME: size from dc
		// FIXME: eviction handler
		// FIXME: metrics
		cache: cache.New(10000, &cache.Options{}),
	}
}

func (r *fcReadiness) ReadyState(nsID namespace.ID, key string) fcReadinessState {
	v := r.cache.Get(fcReadinessCacheKey{nsID: nsID, key: key})
	if v == nil {
		return fcReadinessUnknown
	}
	readiness := v.(fcReadinessCacheValue) // nolint:revive
	return readiness.state
}

func (r *fcReadiness) ReportReady(nsID namespace.ID, key string, gen int64) {
	r.cache.Put(
		fcReadinessCacheKey{nsID: nsID, key: key},
		fcReadinessCacheValue{
			generation: gen,
			state:      fcReadinessReady,
		},
	)
}

func (r *fcReadiness) ReportBlocked(nsID namespace.ID, key string, gen int64) {
	r.cache.Put(
		fcReadinessCacheKey{nsID: nsID, key: key},
		fcReadinessCacheValue{
			generation: gen,
			state:      fcReadinessBlocked,
		},
	)
	// FIXME: start subscription here! if we don't have one
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
			committers[i] = newFcConcurrencyCommitter(ctx, r.concurrencyServiceClient, r, nsID, task, lim.key)
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

type fcTx struct {
	readiness  *fcReadiness
	committers []fcCommitter
	refs       []*taskqueuespb.LimiterRef
	state      [maxLimiters]fcTxState
}

type fcTxState int8 // just so we can pack these in an array

const (
	fcTxStateInit fcTxState = iota
	fcTxStateReserved
	fcTxStateCommitted
	fcTxStateCommitFailed
	fcTxStateCanceled
)

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

func (tx *fcTx) CancelReservations() {
	// this is best-effort, reservations have timeouts so it's okay if we fail to cancel
	if tx == nil {
		return // no limiters
	}
	for i, com := range tx.committers {
		if tx.state[i] == fcTxStateReserved {
			// call in new goroutine, don't block here, we don't care about the result
			go com.CancelReservations()
			tx.state[i] = fcTxStateCanceled
		}
	}
}

package matching

import (
	"context"
	"errors"
	"sync"
	"time"

	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/util"
)

type fcReadinessState int32

const (
	fcReadinessUnknown fcReadinessState = iota
	fcReadinessBlocked
	fcReadinessReady
)

func (s fcReadinessState) Likely() bool {
	return s == fcReadinessUnknown || s == fcReadinessReady
}

type fcReadinessKey struct {
	tp  enumsspb.LimiterType
	key string
}

type fcReadinessValue struct {
	generation int64
	state      fcReadinessState
	// invariant: len(waiters) > 0 == Wait goroutine is running == goroCancel != nil
	// (for now, until we add eviction)
	waiters    map[fcReadinessCallback]struct{}
	goroCancel context.CancelFunc
	// TODO(fc): cache some limiter-specific state?
}

type fcReadinessNS struct {
	r    *fcReadiness
	nsID namespace.ID

	lock  sync.Mutex
	cache map[fcReadinessKey]*fcReadinessValue
}

type fcReadinessCallback interface {
	OnReady()
}

type fcReadiness struct {
	concurrencyServiceClient fcpb.ConcurrencyServiceClient

	caches sync.Map // namespaceID -> *fcReadinessNS
}

func newFcReadiness(
	concurrencyServiceClient fcpb.ConcurrencyServiceClient,
) *fcReadiness {
	return &fcReadiness{
		concurrencyServiceClient: concurrencyServiceClient,
	}
}

func (r *fcReadiness) Stop() {
	r.caches.Range(func(k, v any) bool {
		v.(*fcReadinessNS).Stop() // nolint:revive
		return true
	})
}

func (rn *fcReadinessNS) Stop() {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	for rkey, v := range rn.cache {
		v.waiters = nil
		v.syncGoroLocked(rn, rkey)
	}
}

func (r *fcReadiness) getNS(nsID namespace.ID) *fcReadinessNS {
	v, ok := r.caches.Load(nsID)
	if ok {
		return v.(*fcReadinessNS) // nolint:revive
	}
	newFcrn := &fcReadinessNS{
		r:     r,
		nsID:  nsID,
		cache: make(map[fcReadinessKey]*fcReadinessValue),
	}
	fcrn, _ := r.caches.LoadOrStore(nsID, newFcrn)
	return fcrn.(*fcReadinessNS) // nolint:revive
}

func (rn *fcReadinessNS) getValueLocked(rkey fcReadinessKey) *fcReadinessValue {
	if v, ok := rn.cache[rkey]; ok {
		return v
	}
	v := &fcReadinessValue{
		waiters: make(map[fcReadinessCallback]struct{}),
	}
	rn.cache[rkey] = v
	return v
}

// ReadinessState gets the readiness state of a limiter. If it's blocked and cb is not nil,
// cb.OnReady will be called once when the state of the limiter transitions to ready. If it is
// ready, the callback will be removed from the limiter
//
// If we have too many subscriptions, we may drop some. In that case, we'll set a timer to call
// cb.OnReady with some backoff.
func (r *fcReadiness) ReadinessState(nsID namespace.ID, tp enumsspb.LimiterType, key string, cb fcReadinessCallback) fcReadinessState {
	return r.getNS(nsID).ReadinessState(fcReadinessKey{tp: tp, key: key}, cb)
}

func (rn *fcReadinessNS) ReadinessState(rkey fcReadinessKey, cb fcReadinessCallback) fcReadinessState {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	v, ok := rn.cache[rkey]
	if !ok {
		return fcReadinessUnknown
	}

	if cb != nil {
		// add callback if blocked, remove if unblocked
		if v.state.Likely() {
			delete(v.waiters, cb)
		} else {
			v.waiters[cb] = struct{}{}
		}
		v.syncGoroLocked(rn, rkey)
	}

	return v.state
}

// CancelCallback cancels any future calls to cb.OnReady.
func (r *fcReadiness) CancelCallback(nsID namespace.ID, tp enumsspb.LimiterType, key string, cb fcReadinessCallback) {
	r.getNS(nsID).CancelCallback(fcReadinessKey{tp: tp, key: key}, cb)
}

func (rn *fcReadinessNS) CancelCallback(rkey fcReadinessKey, cb fcReadinessCallback) {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	if v, ok := rn.cache[rkey]; ok {
		delete(v.waiters, cb)
		v.syncGoroLocked(rn, rkey)
	}
}

func (r *fcReadiness) ReportReady(nsID namespace.ID, tp enumsspb.LimiterType, key string, gen int64) {
	r.getNS(nsID).ReportReady(fcReadinessKey{tp: tp, key: key}, gen)
}

// ReportReady is called when a Reserve or Wait call succeeds.
func (rn *fcReadinessNS) ReportReady(rkey fcReadinessKey, gen int64) {
	rn.lock.Lock()

	v := rn.getValueLocked(rkey)
	v.generation = gen
	v.state = fcReadinessReady

	// TODO(fc): do staged wakeup similar to the distributed case
	waiters := v.waiters
	v.waiters = make(map[fcReadinessCallback]struct{})
	v.syncGoroLocked(rn, rkey)

	rn.lock.Unlock()

	for w := range waiters {
		w.OnReady()
	}
}

// ReportReady is called when a Reserve or Wait call fails.
func (r *fcReadiness) ReportBlocked(nsID namespace.ID, tp enumsspb.LimiterType, key string, gen int64) {
	r.getNS(nsID).ReportBlocked(fcReadinessKey{tp: tp, key: key}, gen)
}

func (rn *fcReadinessNS) ReportBlocked(rkey fcReadinessKey, gen int64) {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	v := rn.getValueLocked(rkey)
	v.generation = gen
	v.state = fcReadinessBlocked
}

func (v *fcReadinessValue) syncGoroLocked(rn *fcReadinessNS, rkey fcReadinessKey) {
	haveWaiters := len(v.waiters) > 0
	if (v.goroCancel != nil) == haveWaiters {
		return
	}

	if !haveWaiters {
		v.goroCancel()
		v.goroCancel = nil
		return
	}

	// TODO(fc): put some limit on these, maybe two-stage lru
	ctx := headers.SetCallerInfo(context.Background(), headers.NewCallerInfo(
		rn.nsID.String(), // TODO(fc): use namespace name instead of id
		headers.CallerTypeBackgroundHigh,
		"",
	))
	ctx, v.goroCancel = context.WithCancel(ctx)
	// Wait result will be reported back through ReportReady/Blocked
	go rn.r.callWait(ctx, rn, rkey, v)
}

func (r *fcReadiness) callWait(ctx context.Context, rn *fcReadinessNS, rkey fcReadinessKey, v *fcReadinessValue) {
	policy := backoff.NewExponentialRetryPolicy(time.Second).
		WithExpirationInterval(backoff.NoInterval).
		WithMaximumInterval(time.Minute)
	retrier := backoff.NewRetrier(policy, clock.NewRealTimeSource())

	for ctx.Err() == nil {
		switch rkey.tp {
		case enumsspb.LIMITER_TYPE_CONCURRENCY:
			rn.lock.Lock()
			gen := v.generation
			rn.lock.Unlock()

			res, err := r.concurrencyServiceClient.Wait(ctx, &fcpb.ConcurrencyWaitRequest{
				NamespaceId: rn.nsID.String(),
				Key:         rkey.key,
				Generation:  gen,
			})
			if err != nil {
				util.InterruptibleSleep(ctx, retrier.NextBackOff(err))
				continue
			}
			retrier.Reset()

			if res.WakeCount > 0 {
				rn.ReportReady(rkey, res.Generation)
			} else {
				rn.ReportBlocked(rkey, res.Generation)
			}

		default:
			// TODO: log error
			return
		}
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

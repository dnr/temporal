package fc

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

type ReadinessState int32

const (
	ReadinessUnknown ReadinessState = iota
	ReadinessBlocked
	ReadinessReady
)

func (s ReadinessState) Likely() bool {
	return s == ReadinessUnknown || s == ReadinessReady
}

type rcKey struct {
	tp  enumsspb.LimiterType
	key string
}

type rcValue struct {
	generation int64
	state      ReadinessState
	// invariant: len(waiters) > 0 == Wait goroutine is running == goroCancel != nil
	// (for now, until we add eviction)
	waiters    map[readinessCallback]struct{}
	goroCancel context.CancelFunc
	// TODO(fc): cache some limiter-specific state?
}

type readinessNS struct {
	r    *Readiness
	nsID namespace.ID

	lock  sync.Mutex
	cache map[rcKey]*rcValue
}

type Readiness struct {
	concurrencyServiceClient fcpb.ConcurrencyServiceClient

	caches sync.Map // namespaceID -> *readinessNS
}

func NewReadiness(
	concurrencyServiceClient fcpb.ConcurrencyServiceClient,
) *Readiness {
	return &Readiness{
		concurrencyServiceClient: concurrencyServiceClient,
	}
}

func (r *Readiness) Stop() {
	r.caches.Range(func(k, v any) bool {
		v.(*readinessNS).Stop() // nolint:revive
		return true
	})
}

func (rn *readinessNS) Stop() {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	for rkey, v := range rn.cache {
		v.waiters = nil
		v.syncGoroLocked(rn, rkey)
	}
}

func (r *Readiness) getNS(nsID namespace.ID) *readinessNS {
	v, ok := r.caches.Load(nsID)
	if ok {
		return v.(*readinessNS) // nolint:revive
	}
	newFcrn := &readinessNS{
		r:     r,
		nsID:  nsID,
		cache: make(map[rcKey]*rcValue),
	}
	fcrn, _ := r.caches.LoadOrStore(nsID, newFcrn)
	return fcrn.(*readinessNS) // nolint:revive
}

func (rn *readinessNS) getValueLocked(rkey rcKey) *rcValue {
	if v, ok := rn.cache[rkey]; ok {
		return v
	}
	v := &rcValue{
		waiters: make(map[readinessCallback]struct{}),
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
func (r *Readiness) ReadinessState(nsID namespace.ID, tp enumsspb.LimiterType, key string, cb readinessCallback) ReadinessState {
	return r.getNS(nsID).readinessState(rcKey{tp: tp, key: key}, cb)
}

func (rn *readinessNS) readinessState(rkey rcKey, cb readinessCallback) ReadinessState {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	v, ok := rn.cache[rkey]
	if !ok {
		return ReadinessUnknown
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
func (r *Readiness) CancelCallback(nsID namespace.ID, tp enumsspb.LimiterType, key string, cb readinessCallback) {
	r.getNS(nsID).cancelCallback(rcKey{tp: tp, key: key}, cb)
}

func (rn *readinessNS) cancelCallback(rkey rcKey, cb readinessCallback) {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	if v, ok := rn.cache[rkey]; ok {
		delete(v.waiters, cb)
		v.syncGoroLocked(rn, rkey)
	}
}

func (r *Readiness) reportReady(nsID namespace.ID, tp enumsspb.LimiterType, key string, gen int64) {
	r.getNS(nsID).reportReady(rcKey{tp: tp, key: key}, gen)
}

// reportReady is called when a Reserve or Wait call succeeds.
func (rn *readinessNS) reportReady(rkey rcKey, gen int64) {
	rn.lock.Lock()

	v := rn.getValueLocked(rkey)
	v.generation = gen
	v.state = ReadinessReady

	// TODO(fc): do staged wakeup similar to the distributed case
	waiters := v.waiters
	v.waiters = make(map[readinessCallback]struct{})
	v.syncGoroLocked(rn, rkey)

	rn.lock.Unlock()

	for w := range waiters {
		w.OnReady()
	}
}

// reportBlocked is called when a Reserve or Wait call fails.
func (r *Readiness) reportBlocked(nsID namespace.ID, tp enumsspb.LimiterType, key string, gen int64) {
	r.getNS(nsID).reportBlocked(rcKey{tp: tp, key: key}, gen)
}

func (rn *readinessNS) reportBlocked(rkey rcKey, gen int64) {
	rn.lock.Lock()
	defer rn.lock.Unlock()

	v := rn.getValueLocked(rkey)
	v.generation = gen
	v.state = ReadinessBlocked
}

func (v *rcValue) syncGoroLocked(rn *readinessNS, rkey rcKey) {
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

func (r *Readiness) callWait(ctx context.Context, rn *readinessNS, rkey rcKey, v *rcValue) {
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
				rn.reportReady(rkey, res.Generation)
			} else {
				rn.reportBlocked(rkey, res.Generation)
			}

		default:
			// TODO(fc): log error
			return
		}
	}
}

func (r *Readiness) NewTx(ctx context.Context, nsID namespace.ID, task fcTask) (*tx, error) {
	lims := canonicalLimiters(task)
	if len(lims) == 0 {
		return nil, nil
	}

	committers := make([]committer, len(lims))
	refs := make([]*taskqueuespb.LimiterRef, len(lims))
	for i, lim := range lims {
		switch lim.tp {
		case enumsspb.LIMITER_TYPE_CONCURRENCY:
			committers[i] = newConcurrencyCommitter(ctx, r.concurrencyServiceClient, r, nsID, task.TaskUUID(), lim)
			refs[i] = &taskqueuespb.LimiterRef{LimiterType: lim.tp, Key: lim.key}
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
	state      [maxLimiters]txState
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
	// TODO(fc): we can call Commit concurrently on all limiters
	// note: rate limiters don't have to be Committed
	var errs []error
	for i, com := range tx.committers {
		if err := com.commit(); err != nil {
			errs = append(errs, err)
			tx.state[i] = txStateCommitFailed
		} else {
			tx.state[i] = txStateCommitted
		}
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
			// call in new goroutine, don't block here, we don't care about the result
			go com.cancelReservations()
			tx.state[i] = txStateCanceled
		}
	}
}

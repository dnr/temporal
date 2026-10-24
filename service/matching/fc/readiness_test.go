package fc

import (
	"context"
	"math"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	enumsspb "go.temporal.io/server/api/enums/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/streambatcher"
	"go.temporal.io/server/common/testing/await"
	"go.temporal.io/server/service/matching/simplelimiter"
	"google.golang.org/grpc"
)

type testCallback struct{}

func (*testCallback) OnReady() {}

type testClient struct {
	fcpb.ConcurrencyServiceClient
	batch func(context.Context, *fcpb.ConcurrencyServiceBatchRequest) (*fcpb.ConcurrencyServiceBatchResponse, error)
	wait  func(context.Context, *fcpb.ConcurrencyServiceWaitRequest) (*fcpb.ConcurrencyServiceWaitResponse, error)
}

func (c *testClient) Batch(ctx context.Context, req *fcpb.ConcurrencyServiceBatchRequest, _ ...grpc.CallOption) (*fcpb.ConcurrencyServiceBatchResponse, error) {
	return c.batch(ctx, req)
}

func (c *testClient) Wait(ctx context.Context, req *fcpb.ConcurrencyServiceWaitRequest, _ ...grpc.CallOption) (*fcpb.ConcurrencyServiceWaitResponse, error) {
	return c.wait(ctx, req)
}

func newTestReadiness(t *testing.T, client fcpb.ConcurrencyServiceClient) (*Readiness, *clock.EventTimeSource) {
	t.Helper()
	ts := clock.NewEventTimeSource().Update(time.Now())
	ts.UseAsyncTimers(true)
	cfg := NewReadinessConfig(dynamicconfig.NewNoopCollection())
	// one item per batch, so batching doesn't wait on the fake clock
	cfg.clientBatcherOptions = func() streambatcher.BatcherOptions {
		return streambatcher.BatcherOptions{MaxItems: 1, IdleTime: time.Minute}
	}
	r := NewReadiness(ts, client, cfg)
	t.Cleanup(r.Stop)
	return r, ts
}

func requirePins(t *testing.T, lim limiterState, pins int) {
	t.Helper()
	b := lim.base()
	b.rs.lock.Lock()
	defer b.rs.lock.Unlock()
	require.Equal(t, int32(pins), b.pins)
}

func TestTransactionPins(t *testing.T) {
	for _, name := range []string{"finish", "rollback", "check only"} {
		t.Run(name, func(t *testing.T) {
			r, ts := newTestReadiness(t, nil)
			lim := r.getLocalLimiter(namespace.ID("ns"), "key")
			tx := &Tx{limiters: []txEntry{{limiterTx: newLocalLimiterTx(lim, simplelimiter.NoLimit(), 0)}}}
			requirePins(t, lim, 1)
			lim.rs.cleanup(1000, r.gcParameters())
			require.Len(t, lim.rs.localLimiters, 1)
			require.NoError(t, tx.Check())
			switch name {
			case "rollback":
				tx.Rollback(context.Background())
			case "finish":
				require.NoError(t, tx.Reserve(context.Background()))
				require.NoError(t, tx.Commit(context.Background()))
				tx.Close()
			default:
				require.Equal(t, "check only", name)
			}
			tx.Close()
			tx.Rollback(context.Background())
			requirePins(t, lim, 0)
			require.ErrorIs(t, tx.Check(), errInvalidTxState)
			ts.Advance(time.Nanosecond)
			lim.rs.cleanup(1000, r.gcParameters())
			require.Empty(t, lim.rs.localLimiters)
		})
	}
}

func TestFailedCheckReleasesAllTransactionPins(t *testing.T) {
	r, _ := newTestReadiness(t, nil)
	ns := namespace.ID("ns")
	first := r.getLocalLimiter(ns, "first")
	second := r.getLocalLimiter(ns, "second")
	cb := &testCallback{}
	tx := &Tx{cb: cb, limiters: []txEntry{
		{limiterTx: newLocalLimiterTx(first, simplelimiter.MakeParams(0, 0), 0)},
		{limiterTx: newLocalLimiterTx(second, simplelimiter.NoLimit(), 0)},
	}}
	require.ErrorIs(t, tx.Check(), ErrLocalLimiterBlocked)
	requirePins(t, first, 1) // timer
	requirePins(t, second, 0)
	r.CancelAllCallbacks(ns, cb)
	requirePins(t, first, 0)
}

func TestCanceledWaitRetainsPinUntilExit(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	exit := make(chan struct{})
	client := &testClient{wait: func(ctx context.Context, _ *fcpb.ConcurrencyServiceWaitRequest) (*fcpb.ConcurrencyServiceWaitResponse, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-exit
		return nil, ctx.Err()
	}}
	r, ts := newTestReadiness(t, client)
	ns := namespace.ID("ns")
	lim := r.getConcurrencyLimiter(ns, "key")
	lim.tokens = 0
	cb := &testCallback{}
	_, err := lim.check(cb, 0)
	require.ErrorIs(t, err, ErrConcurrencyBlocked)
	lim.unpin()
	<-started
	r.CancelAllCallbacks(ns, cb)
	<-canceled
	requirePins(t, lim, 1)
	lim.rs.cleanup(1000, r.gcParameters())
	require.Len(t, lim.rs.concurrencyLimiters, 1)
	close(exit)
	await.RequireTrue(t, func() bool {
		lim.rs.lock.Lock()
		defer lim.rs.lock.Unlock()
		return lim.pins == 0
	}, time.Second, time.Millisecond)
	ts.Advance(concurrencyBlockedRetention)
	lim.rs.cleanup(1000, r.gcParameters())
	require.Empty(t, lim.rs.concurrencyLimiters)
}

func TestCancelReservationOwnsPinAndDeadline(t *testing.T) {
	started := make(chan struct{})
	exit := make(chan struct{})
	client := &testClient{batch: func(ctx context.Context, _ *fcpb.ConcurrencyServiceBatchRequest) (*fcpb.ConcurrencyServiceBatchResponse, error) {
		_, hasDeadline := ctx.Deadline()
		require.True(t, hasDeadline)
		require.NoError(t, ctx.Err())
		close(started)
		<-exit
		return &fcpb.ConcurrencyServiceBatchResponse{}, nil
	}}
	r, _ := newTestReadiness(t, client)
	lim := r.getConcurrencyLimiter(namespace.ID("ns"), "key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lim.cancelReserve(ctx, "slot")
	lim.unpin()
	<-started
	requirePins(t, lim, 1)
	lim.rs.cleanup(1000, r.gcParameters())
	require.Len(t, lim.rs.concurrencyLimiters, 1)
	close(exit)
	await.RequireTrue(t, func() bool {
		lim.rs.lock.Lock()
		defer lim.rs.lock.Unlock()
		return lim.pins == 0
	}, time.Second, time.Millisecond)
}

func TestTimerPins(t *testing.T) {
	r, ts := newTestReadiness(t, nil)
	ns := namespace.ID("ns")
	lim := r.getLocalLimiter(ns, "key")
	cb := &testCallback{}
	params := simplelimiter.MakeParams(1, 0)
	require.NoError(t, lim.check(params, cb, 0))
	require.ErrorIs(t, lim.check(params, cb, 0), ErrLocalLimiterBlocked)
	requirePins(t, lim, 2)
	lim.unpin()
	ts.Advance(time.Second)
	await.RequireTrue(t, func() bool {
		lim.rs.lock.Lock()
		defer lim.rs.lock.Unlock()
		return lim.pins == 0
	}, time.Second, time.Millisecond)
}

func TestGCListMembership(t *testing.T) {
	r, _ := newTestReadiness(t, nil)
	ns := namespace.ID("ns")
	first := r.getLocalLimiter(ns, "first")
	second := r.getLocalLimiter(ns, "second")
	rs := first.rs
	require.EqualValues(t, 2, rs.created)
	require.Zero(t, rs.gcList.len)
	first.unpin()
	second.unpin()
	require.Equal(t, 2, rs.gcList.len)
	require.Same(t, first.base(), rs.gcList.head)
	require.Same(t, second.base(), rs.gcList.tail)
	first = r.getLocalLimiter(ns, "first")
	require.EqualValues(t, 2, rs.created)
	require.Same(t, second.base(), rs.gcList.head)
	cb := &testCallback{}
	rs.lock.Lock()
	rs.addEdgeLocked(first, cb, 0)
	rs.lock.Unlock()
	first.unpin()
	require.Equal(t, 1, rs.gcList.len)
	r.CancelAllCallbacks(ns, cb)
	require.Equal(t, 2, rs.gcList.len)
	require.Same(t, first.base(), rs.gcList.tail)
	second = r.getLocalLimiter(ns, "second")
	require.Same(t, first.base(), rs.gcList.head)
	require.Same(t, first.base(), rs.gcList.tail)
	second.unpin()
	r.Stop()
	require.Zero(t, rs.gcList.len)
}

func TestCleanupCandidates(t *testing.T) {
	r, ts := newTestReadiness(t, nil)
	ns := namespace.ID("ns")
	debt := r.getLocalLimiter(ns, "debt")
	free := r.getLocalLimiter(ns, "free")
	zero := r.getLocalLimiter(ns, "zero")
	pinned := r.getLocalLimiter(ns, "pinned")
	cb := &testCallback{}
	require.NoError(t, debt.check(simplelimiter.MakeParams(1, 0), cb, 0))
	require.ErrorIs(t, zero.check(simplelimiter.MakeParams(0, 0), cb, 0), ErrLocalLimiterBlocked)
	r.CancelAllCallbacks(ns, cb)
	debt.unpin()
	free.unpin()
	zero.unpin()
	rs := debt.rs
	rs.cleanup(1000, r.gcParameters())
	require.Len(t, rs.localLimiters, 2)
	require.Same(t, debt, rs.localLimiters[debt.mapKey])
	require.Same(t, pinned, rs.localLimiters[pinned.mapKey])
	require.Equal(t, 1, rs.gcList.len)
	ts.Advance(2 * time.Second)
	rs.cleanup(1000, r.gcParameters())
	require.Equal(t, 0, rs.gcList.len)
	require.Len(t, rs.localLimiters, 1)
	pinned.unpin()
	rs.cleanup(1000, r.gcParameters())
	require.Empty(t, rs.localLimiters)
}

func TestCleanupWorkerStops(t *testing.T) {
	r, _ := newTestReadiness(t, nil)
	r.Start()
	r.Stop()
	select {
	case <-r.cleanupHandle.Done():
	default:
		t.Fatal("cleanup worker has not exited")
	}
}

type testTask struct {
	limiters []Limiter
}

func (t *testTask) Limiters() []Limiter { return t.limiters }

func (*testTask) PriorityAndAge() (int32, time.Time) { return 1, time.Time{} }

func TestOptimisticConcurrencyReserve(t *testing.T) {
	for _, allow := range []bool{false, true} {
		name := "blocked"
		if allow {
			name = "allowed"
		}
		t.Run(name, func(t *testing.T) {
			batches := 0
			client := &testClient{batch: func(_ context.Context, req *fcpb.ConcurrencyServiceBatchRequest) (*fcpb.ConcurrencyServiceBatchResponse, error) {
				batches++
				res := &fcpb.ConcurrencyServiceBatchResponse{}
				if len(req.ReserveSlots) > 0 {
					res.ReserveSuccess = []bool{allow}
				}
				if len(req.CommitSlots) > 0 {
					res.CommitSuccess = []bool{true}
				}
				return res, nil
			}}
			r, _ := newTestReadiness(t, client)
			task := &testTask{limiters: []Limiter{{
				Type: enumsspb.LIMITER_TYPE_CONCURRENCY, Source: LimiterSourceConfig, Key: "key",
			}}}
			tx := r.NewTx(namespace.ID("ns"), task, &testCallback{})
			defer tx.Close()
			lim := tx.limiters[0].limiterTx.(*concurrencyTx).cs
			require.EqualValues(t, 1, lim.tokens)
			require.NoError(t, tx.Check())
			require.Zero(t, batches)
			err := tx.Reserve(context.Background())
			if allow {
				require.NoError(t, err)
				require.NoError(t, tx.Commit(context.Background()))
				require.Equal(t, 2, batches)
			} else {
				require.Error(t, err)
				require.Equal(t, 1, batches)
				require.Zero(t, lim.tokens)
			}
			tx.Close()
			requirePins(t, lim, 0)
		})
	}
}

func TestConcurrencyRunIDCaching(t *testing.T) {
	var gotRunIDs []string
	client := &testClient{batch: func(_ context.Context, req *fcpb.ConcurrencyServiceBatchRequest) (*fcpb.ConcurrencyServiceBatchResponse, error) {
		gotRunIDs = append(gotRunIDs, req.RunId)
		res := &fcpb.ConcurrencyServiceBatchResponse{RunId: "run-1", AvailableSlotsHint: 10}
		if len(req.ReserveSlots) > 0 {
			res.ReserveSuccess = []bool{true}
		}
		if len(req.CommitSlots) > 0 {
			res.CommitSuccess = []bool{true}
		}
		return res, nil
	}}
	r, _ := newTestReadiness(t, client)
	task := &testTask{limiters: []Limiter{{
		Type: enumsspb.LIMITER_TYPE_CONCURRENCY, Source: LimiterSourceConfig, Key: "key",
	}}}
	tx := r.NewTx(namespace.ID("ns"), task, &testCallback{})
	require.NoError(t, tx.Check())
	require.NoError(t, tx.Reserve(context.Background()))
	// learned the run id from the reserve response: pass it to history and on later calls
	refs := tx.LimiterRefs()
	require.Len(t, refs, 1)
	require.Equal(t, "run-1", refs[0].RunId)
	require.NoError(t, tx.Commit(context.Background()))
	tx.Close()
	require.Equal(t, []string{"", "run-1"}, gotRunIDs)
}

func TestBlockedConcurrencyRetention(t *testing.T) {
	r, ts := newTestReadiness(t, nil)
	ns := namespace.ID("ns")
	lim := r.getConcurrencyLimiter(ns, "key")
	lim.tokens = 0
	lim.unpin()
	lim.rs.cleanup(1000, r.gcParameters())
	require.Len(t, lim.rs.concurrencyLimiters, 1)
	ts.Advance(concurrencyBlockedRetention / 2)
	reused := r.getConcurrencyLimiter(ns, "key")
	require.Same(t, lim, reused)
	reused.unpin()
	ts.Advance(concurrencyBlockedRetention / 2)
	lim.rs.cleanup(1000, r.gcParameters())
	require.Len(t, lim.rs.concurrencyLimiters, 1)
	ts.Advance(concurrencyBlockedRetention / 2)
	lim.rs.cleanup(1000, r.gcParameters())
	require.Empty(t, lim.rs.concurrencyLimiters)

	recreated := r.getConcurrencyLimiter(ns, "key")
	require.NotSame(t, lim, recreated)
	require.EqualValues(t, 1, recreated.tokens)
	recreated.unpin()
	recreated.rs.cleanup(1000, r.gcParameters())
	require.Empty(t, recreated.rs.concurrencyLimiters)
}

type pendingTimerClock struct {
	clock.TimeSource
	timer *pendingTimer
}

type pendingTimer struct {
	active   bool
	callback func()
}

func (c *pendingTimerClock) AfterFunc(_ time.Duration, callback func()) clock.Timer {
	c.timer = &pendingTimer{active: true, callback: callback}
	return c.timer
}

func (t *pendingTimer) Reset(time.Duration) bool {
	active := t.active
	t.active = true
	return active
}

func (t *pendingTimer) Stop() bool {
	active := t.active
	t.active = false
	return active
}

func TestPendingTimerCallbackRetainsPin(t *testing.T) {
	ts := &pendingTimerClock{TimeSource: clock.NewEventTimeSource().Update(time.Now())}
	r := NewReadiness(ts, nil, NewReadinessConfig(dynamicconfig.NewNoopCollection()))
	t.Cleanup(r.Stop)
	ns := namespace.ID("ns")
	lim := r.getLocalLimiter(ns, "key")
	cb := &testCallback{}
	params := simplelimiter.MakeParams(1, 0)
	require.NoError(t, lim.check(params, cb, 0))
	require.ErrorIs(t, lim.check(params, cb, 0), ErrLocalLimiterBlocked)
	requirePins(t, lim, 2)
	// The timer has fired, but its callback has not acquired the shard lock yet.
	ts.timer.active = false
	require.ErrorIs(t, lim.check(params, cb, 0), ErrLocalLimiterBlocked)
	requirePins(t, lim, 3)
	r.CancelAllCallbacks(ns, cb)
	lim.unpin()
	requirePins(t, lim, 1)
	require.Zero(t, lim.rs.gcList.len)
	ts.timer.callback()
	requirePins(t, lim, 0)
	require.Equal(t, 1, lim.rs.gcList.len)
}

func TestLocalLimiterDroppableFullBurst(t *testing.T) {
	r, ts := newTestReadiness(t, nil)
	lim := r.getLocalLimiter(namespace.ID("ns"), "key")
	params := simplelimiter.MakeParams(1, 2*time.Second)
	for range 3 {
		require.NoError(t, lim.check(params, &testCallback{}, 0))
	}
	lim.unpin()
	ts.Advance(time.Second)
	require.Zero(t, lim.lim.Delay(ts.Now().UnixNano()))
	require.False(t, lim.droppableLocked(r.gcParameters()))
	ts.Advance(2*time.Second - time.Nanosecond)
	require.False(t, lim.droppableLocked(r.gcParameters()))
	ts.Advance(time.Nanosecond)
	require.True(t, lim.droppableLocked(r.gcParameters()))
}

func TestLimiterIdentityAndGC(t *testing.T) {
	r, _ := newTestReadiness(t, nil)
	ns := namespace.ID("01234567-89ab-cdef-0123-456789abcdef")
	cs := r.getConcurrencyLimiter(ns, "same-key")
	lls := r.getLocalLimiter(ns, "same-key")
	require.Equal(t, ns, cs.namespaceID())
	require.Equal(t, "same-key", cs.limiterKey())
	require.Equal(t, cs.mapKey, lls.mapKey)
	cs.unpin()
	lls.unpin()
	require.Equal(t, 2, cs.rs.cleanup(1000, r.gcParameters()))
	require.Empty(t, cs.rs.concurrencyLimiters)
	require.Empty(t, cs.rs.localLimiters)
	t.Logf("concurrency state: %d bytes; local limiter state: %d bytes", unsafe.Sizeof(*cs), unsafe.Sizeof(*lls))
}

func TestConcurrencyRetentionSecondsWrap(t *testing.T) {
	r, ts := newTestReadiness(t, nil)
	ts.Advance(time.Duration(math.MaxInt32-20) * time.Second)
	lim := r.getConcurrencyLimiter(namespace.ID("ns"), "key")
	lim.tokens = 0
	lim.unpin()
	ts.Advance(concurrencyBlockedRetention - time.Second)
	require.Negative(t, r.elapsedSeconds(ts.Now()))
	require.False(t, lim.droppableLocked(r.gcParameters()))
	ts.Advance(time.Second)
	require.True(t, lim.droppableLocked(r.gcParameters()))
}

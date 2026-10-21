package fc

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/namespace"
	serviceerrors "go.temporal.io/server/common/serviceerror"
	"go.temporal.io/server/common/streambatcher"
	"go.temporal.io/server/service/matching/simplelimiter"
)

const (
	testNsID   = namespace.ID("nsid")
	testConKey = "conkey"
)

func newConcurrencyTestTask() *testTask {
	return &testTask{limiters: []Limiter{{
		Type:   enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:    testConKey,
		Source: LimiterSourceConfig,
	}}}
}

func newHintTestClient(batch func(*fcpb.ConcurrencyServiceBatchRequest) *fcpb.ConcurrencyServiceBatchResponse) *testClient {
	return &testClient{
		batch: func(_ context.Context, req *fcpb.ConcurrencyServiceBatchRequest) (*fcpb.ConcurrencyServiceBatchResponse, error) {
			return batch(req), nil
		},
		wait: func(ctx context.Context, _ *fcpb.ConcurrencyServiceWaitRequest) (*fcpb.ConcurrencyServiceWaitResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
}

// setTokens sets the cached tokens of the test concurrency limiter, as a Wait response would.
func setTokens(r *Readiness, gen int64, tokens int32) {
	cs := r.getConcurrencyLimiter(testNsID, testConKey)
	defer cs.unpin()
	cs.reportWaitResponse(gen, tokens, notifySync)
}

func getTokens(r *Readiness) int32 {
	rs := r.getShard(testNsID)
	rs.lock.Lock()
	defer rs.lock.Unlock()
	return rs.concurrencyLimiters[testNsID.String()+testConKey].tokens
}

func concurrencyHint(gen int64, slots int32) *taskqueuespb.LimiterHint {
	return &taskqueuespb.LimiterHint{
		LimiterType:        enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:                testConKey,
		Generation:         gen,
		AvailableSlotsHint: slots,
	}
}

func TestSkipCheck_DoesNotConsumeOrReturnTokens(t *testing.T) {
	r, _ := newTestReadiness(t, nil)
	setTokens(r, 1, 5)

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.SkipCheck())
	require.Equal(t, int32(5), getTokens(r))

	tx.Rollback(context.Background())
	require.Equal(t, int32(5), getTokens(r))
}

func TestSkipCheck_DoesNotBlock(t *testing.T) {
	r, _ := newTestReadiness(t, newHintTestClient(nil))
	setTokens(r, 1, 0)
	task := newConcurrencyTestTask()
	// a local rate limiter that's always blocked
	task.limiters = append(task.limiters, Limiter{
		Type:   enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
		Key:    "ratekey",
		Source: LimiterSourceConfig,
		Config: new(simplelimiter.MakeParams(0, 0)),
	})
	cb := &testCallback{}
	defer r.CancelAllCallbacks(testNsID, cb)

	// concurrency limiter has zero tokens, so a normal check blocks
	tx := r.NewTx(testNsID, task, cb)
	require.ErrorIs(t, tx.Check(), ErrConcurrencyBlocked)

	tx = r.NewTx(testNsID, task, cb)
	require.NoError(t, tx.SkipCheck())
	require.Equal(t, int32(0), getTokens(r))
	tx.Close()
}

func TestSkipCheck_InvalidState(t *testing.T) {
	r, _ := newTestReadiness(t, nil)
	setTokens(r, 1, 5)

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.Check())
	require.ErrorIs(t, tx.SkipCheck(), errInvalidTxState)
	tx.Rollback(context.Background())
	require.ErrorIs(t, tx.SkipCheck(), errInvalidTxState)
}

func TestHints_FromReserveAndCommit(t *testing.T) {
	r, _ := newTestReadiness(t, newHintTestClient(func(req *fcpb.ConcurrencyServiceBatchRequest) *fcpb.ConcurrencyServiceBatchResponse {
		if len(req.ReserveSlots) > 0 {
			return &fcpb.ConcurrencyServiceBatchResponse{Generation: 7, AvailableSlotsHint: 3, ReserveSuccess: []bool{true}}
		}
		return &fcpb.ConcurrencyServiceBatchResponse{Generation: 7, AvailableSlotsHint: 2, CommitSuccess: []bool{true}}
	}))

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.SkipCheck())
	require.NoError(t, tx.Reserve(context.Background()))
	require.Equal(t, int32(3), getTokens(r))
	require.NoError(t, tx.Commit(context.Background()))
	require.Nil(t, tx.Hints(), "hints are only available after Close")

	tx.Close()
	require.Equal(t, []*taskqueuespb.LimiterHint{concurrencyHint(7, 2)}, tx.Hints())
}

func TestHints_FromBlockedReserve(t *testing.T) {
	r, _ := newTestReadiness(t, newHintTestClient(func(*fcpb.ConcurrencyServiceBatchRequest) *fcpb.ConcurrencyServiceBatchResponse {
		return &fcpb.ConcurrencyServiceBatchResponse{Generation: 4, AvailableSlotsHint: 0, ReserveSuccess: []bool{false}}
	}))

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.SkipCheck())
	err := tx.Reserve(context.Background())
	var blocked *serviceerrors.FlowControlBlocked
	require.ErrorAs(t, err, &blocked)
	// Reserve's implicit Rollback closes the tx
	require.Equal(t, []*taskqueuespb.LimiterHint{concurrencyHint(4, 0)}, tx.Hints())
}

func TestHints_SharedByBatchedCallers(t *testing.T) {
	var calls atomic.Int32
	client := newHintTestClient(func(req *fcpb.ConcurrencyServiceBatchRequest) *fcpb.ConcurrencyServiceBatchResponse {
		calls.Add(1)
		return &fcpb.ConcurrencyServiceBatchResponse{Generation: 4, AvailableSlotsHint: 0, ReserveSuccess: []bool{true, false}}
	})
	cfg := NewReadinessConfig(dynamicconfig.NewNoopCollection())
	cfg.clientBatcherOptions = func() streambatcher.BatcherOptions {
		return streambatcher.BatcherOptions{MaxItems: 2, MinDelay: time.Minute, MaxDelay: time.Minute, IdleTime: time.Minute}
	}
	r := NewReadiness(clock.NewRealTimeSource(), client, cfg)
	defer r.Stop()

	// Both reserves go in one rpc, and both see the state from it.
	txs := []*Tx{
		r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{}),
		r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{}),
	}
	var wg sync.WaitGroup
	for _, tx := range txs {
		require.NoError(t, tx.SkipCheck())
		wg.Go(func() {
			_ = tx.Reserve(context.Background())
			tx.Close()
		})
	}
	wg.Wait()

	require.Equal(t, int32(1), calls.Load())
	for _, tx := range txs {
		require.Equal(t, []*taskqueuespb.LimiterHint{concurrencyHint(4, 0)}, tx.Hints())
	}
}

func TestHints_OnlyForForwardedTasks(t *testing.T) {
	r, _ := newTestReadiness(t, newHintTestClient(func(req *fcpb.ConcurrencyServiceBatchRequest) *fcpb.ConcurrencyServiceBatchResponse {
		if len(req.ReserveSlots) > 0 {
			return &fcpb.ConcurrencyServiceBatchResponse{Generation: 7, AvailableSlotsHint: 3, ReserveSuccess: []bool{true}}
		}
		return &fcpb.ConcurrencyServiceBatchResponse{Generation: 7, AvailableSlotsHint: 2, CommitSuccess: []bool{true}}
	}))

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.Check())
	require.NoError(t, tx.Reserve(context.Background()))
	require.NoError(t, tx.Commit(context.Background()))
	tx.Close()
	require.Nil(t, tx.Hints())
}

func TestHints_ExcludeWaitResults(t *testing.T) {
	r, _ := newTestReadiness(t, nil)
	// wake tokens from Wait are for this node only
	setTokens(r, 3, 1)

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.SkipCheck())
	tx.Close()
	require.Nil(t, tx.Hints())
}

func TestApplyHints_SupersedesCheckToken(t *testing.T) {
	r, _ := newTestReadiness(t, nil)
	setTokens(r, 1, 5)

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.Check())
	require.Equal(t, int32(4), getTokens(r))

	// forwarded, parent found no slots
	tx.ApplyHints([]*taskqueuespb.LimiterHint{concurrencyHint(2, 0)})
	require.Equal(t, int32(0), getTokens(r))

	// rollback after a failed forward must not return the check token
	tx.Rollback(context.Background())
	require.Equal(t, int32(0), getTokens(r))
}

func TestApplyHints_PassedOnByMiddleHop(t *testing.T) {
	r, _ := newTestReadiness(t, nil)

	// task forwarded to us, and we forwarded it further
	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.SkipCheck())
	tx.ApplyHints([]*taskqueuespb.LimiterHint{concurrencyHint(2, 0)})
	require.Equal(t, int32(0), getTokens(r))
	tx.Rollback(context.Background())

	require.Equal(t, []*taskqueuespb.LimiterHint{concurrencyHint(2, 0)}, tx.Hints())
}

func TestApplyHints_StaleGenerationIgnored(t *testing.T) {
	r, _ := newTestReadiness(t, nil)
	setTokens(r, 3, 5)

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.Check())
	require.Equal(t, int32(4), getTokens(r))

	tx.ApplyHints([]*taskqueuespb.LimiterHint{concurrencyHint(2, 0)})
	require.Equal(t, int32(4), getTokens(r))

	tx.Rollback(context.Background())
	require.Equal(t, int32(5), getTokens(r))
}

func TestApplyHints_UnknownOrInvalidIgnored(t *testing.T) {
	r, _ := newTestReadiness(t, nil)
	setTokens(r, 1, 5)

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.Check())

	otherKey := concurrencyHint(2, 0)
	otherKey.Key = "otherkey"
	otherType := concurrencyHint(2, 0)
	otherType.LimiterType = enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT
	negative := concurrencyHint(2, -1)
	tx.ApplyHints([]*taskqueuespb.LimiterHint{otherKey, otherType, negative})

	require.Equal(t, int32(4), getTokens(r))
	require.Empty(t, tx.Hints())
	tx.Rollback(context.Background())
	require.Equal(t, int32(5), getTokens(r))
}

func TestNilTx(t *testing.T) {
	var tx *Tx
	require.NoError(t, tx.SkipCheck())
	require.Nil(t, tx.Hints())
	tx.ApplyHints([]*taskqueuespb.LimiterHint{concurrencyHint(1, 1)})
}

func TestCheckWithoutCallbackRollsBackWithoutRegisteringWaiters(t *testing.T) {
	t.Parallel()
	r, _ := newTestReadiness(t, nil)
	task := &testTask{limiters: []Limiter{
		{Type: enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT, Key: "first", Source: LimiterSourceConfig, Config: new(simplelimiter.MakeParams(1, 0))},
		{Type: enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT, Key: "second", Source: LimiterSourceConfig, Config: new(simplelimiter.MakeParams(0, 0))},
	}}
	tx := r.NewTx(testNsID, task, nil)
	require.ErrorIs(t, tx.Check(), ErrLocalLimiterBlocked)
	require.True(t, tx.closed)
	rs := r.getShard(testNsID)
	func() {
		rs.lock.Lock()
		defer rs.lock.Unlock()
		require.Empty(t, rs.fwd)
		require.Empty(t, rs.rev)
		for _, state := range rs.localLimiters {
			require.Zero(t, state.pins)
		}
	}()

	// The first limiter's token must have been returned when the second blocked.
	task.limiters = task.limiters[:1]
	tx = r.NewTx(testNsID, task, nil)
	require.NoError(t, tx.Check())
	tx.Close()
}

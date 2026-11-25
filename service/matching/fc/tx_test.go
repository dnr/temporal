package fc

import (
	"context"
	"errors"
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
	"go.temporal.io/server/common/testing/parallelsuite"
	"go.temporal.io/server/common/testing/testcontext"
	"go.temporal.io/server/service/matching/simplelimiter"
	"go.uber.org/mock/gomock"
)

type txSuite struct {
	parallelsuite.Suite[*txSuite]
}

func TestTx(t *testing.T) {
	parallelsuite.Run(t, &txSuite{})
}

func newMockTx(ctrl *gomock.Controller, count int) (*Tx, []*MocklimiterTx) {
	tx := &Tx{limiters: make([]txEntry, count)}
	limiters := make([]*MocklimiterTx, count)
	for i := range limiters {
		limiters[i] = NewMocklimiterTx(ctrl)
		limiters[i].EXPECT().hint().Return(nil).AnyTimes()
		tx.limiters[i] = txEntry{limiterTx: limiters[i]}
	}
	return tx, limiters
}

func (s *txSuite) newTx(count int) (*Tx, []*MocklimiterTx) {
	s.T().Helper()
	return newMockTx(gomock.NewController(s.T()), count)
}

func expectChecks(callback any, limiters []*MocklimiterTx) {
	calls := make([]any, 0, len(limiters))
	for _, limiter := range limiters {
		calls = append(calls, limiter.EXPECT().check(callback).Return(nil))
	}
	gomock.InOrder(calls...)
}

func expectReserves(ctx context.Context, limiters []*MocklimiterTx) {
	calls := make([]any, 0, len(limiters))
	for _, limiter := range limiters {
		calls = append(calls, limiter.EXPECT().reserve(ctx).Return(nil))
	}
	gomock.InOrder(calls...)
}

func TestNilTx(t *testing.T) {
	t.Parallel()
	var tx *Tx
	require.NoError(t, tx.Check())
	require.NoError(t, tx.SkipCheck())
	require.NoError(t, tx.Reserve(t.Context()))
	require.NoError(t, tx.Commit(t.Context()))
	tx.Rollback(t.Context())
	tx.Close()
	require.Nil(t, tx.Hints())
	tx.ApplyHints([]*taskqueuespb.LimiterHint{concurrencyHint(1, 1)})
}

func (s *txSuite) TestSuccessfulTransaction() {
	// Use more entries than MaxLimiters to cover every entry in a larger transaction.
	ctx := s.Context()
	tx, limiters := s.newTx(MaxLimiters + 1)
	cb := &testCallback{}
	tx.cb = cb

	expectChecks(gomock.Cond(func(got ReadinessCallback) bool { return got == cb }), limiters)
	s.NoError(tx.Check())
	expectReserves(ctx, limiters)
	s.NoError(tx.Reserve(ctx))
	for _, limiter := range limiters {
		limiter.EXPECT().commit(ctx).Return(nil)
	}
	s.NoError(tx.Commit(ctx))

	for _, limiter := range limiters {
		limiter.EXPECT().close()
	}
	tx.Close()
	tx.Close()
	tx.Rollback(ctx)
}

func (s *txSuite) TestCheckFailureRefundsOnlyEarlierChecks() {
	for failAt, name := range []string{"first", "middle", "last"} {
		s.Run(name, func(s *txSuite) {
			tx, limiters := s.newTx(3)
			checkErr := errors.New("check blocked")
			checks := make([]any, 0, failAt+1)
			for _, limiter := range limiters[:failAt] {
				checks = append(checks, limiter.EXPECT().check(nil).Return(nil))
			}
			failed := limiters[failAt].EXPECT().check(nil).Return(checkErr)
			gomock.InOrder(append(checks, failed)...)
			for i, limiter := range limiters {
				cleanup := failed
				if i < failAt {
					cleanup = limiter.EXPECT().cancelCheck().After(failed)
				}
				limiter.EXPECT().close().After(cleanup)
			}

			s.ErrorIs(tx.Check(), checkErr)
		})
	}
}

func (s *txSuite) TestReserveFailureCancelsOnlyEarlierReservations() {
	for failAt, name := range []string{"first", "middle", "last"} {
		s.Run(name, func(s *txSuite) {
			ctx := s.Context()
			tx, limiters := s.newTx(3)
			expectChecks(nil, limiters)
			s.NoError(tx.Check())

			reserveErr := errors.New("reserve blocked")
			reserves := make([]any, 0, failAt+1)
			for _, limiter := range limiters[:failAt] {
				reserves = append(reserves, limiter.EXPECT().reserve(ctx).Return(nil))
			}
			failed := limiters[failAt].EXPECT().reserve(ctx).Return(reserveErr)
			gomock.InOrder(append(reserves, failed)...)
			for i, limiter := range limiters {
				cleanup := failed
				if i < failAt {
					cleanup = limiter.EXPECT().cancelReserve(ctx).After(failed)
				}
				checkCanceled := limiter.EXPECT().cancelCheck().After(cleanup)
				limiter.EXPECT().close().After(checkCanceled)
			}

			s.ErrorIs(tx.Reserve(ctx), reserveErr)
		})
	}
}

func (s *txSuite) TestCommitRunsConcurrentlyAndJoinsErrors() {
	ctx, cancel := context.WithCancel(testcontext.For(s.T(), testcontext.WithTimeout(5*time.Second)))
	defer cancel()
	ctrl, ctx := gomock.WithContext(ctx, s.T())
	tx, limiters := newMockTx(ctrl, 3)
	firstErr := errors.New("commit rejected")
	lastErr := errors.New("commit response lost")
	entered := make(chan struct{}, len(limiters))
	release := make(chan struct{})

	expectChecks(nil, limiters)
	s.NoError(tx.Check())
	expectReserves(ctx, limiters)
	s.NoError(tx.Reserve(ctx))
	for i, err := range []error{firstErr, nil, lastErr} {
		limiter := limiters[i]
		commit := limiter.EXPECT().commit(ctx).DoAndReturn(func(ctx context.Context) error {
			entered <- struct{}{}
			select {
			case <-release:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		// A failed RPC can leave a committed slot. None of these entries is safe to refund.
		limiter.EXPECT().close().After(commit)
	}

	result := make(chan error, 1)
	go func() { result <- tx.Commit(ctx) }()
	// Each commit must enter while the others are still waiting at the gate.
	for range limiters {
		s.Rcv(entered)
	}
	close(release)
	err := s.Rcv(result)

	s.ErrorIs(err, firstErr)
	s.ErrorIs(err, lastErr)
}

func (s *txSuite) TestRollbackAfterCheck() {
	tx, limiters := s.newTx(2)
	expectChecks(nil, limiters)
	s.NoError(tx.Check())

	for _, limiter := range limiters {
		canceled := limiter.EXPECT().cancelCheck()
		limiter.EXPECT().close().After(canceled)
	}
	tx.Rollback(s.Context())
	tx.Rollback(s.Context())
	tx.Close()
}

func (s *txSuite) TestRollbackAfterReserve() {
	ctx := s.Context()
	tx, limiters := s.newTx(2)
	expectChecks(nil, limiters)
	s.NoError(tx.Check())
	expectReserves(ctx, limiters)
	s.NoError(tx.Reserve(ctx))

	for _, limiter := range limiters {
		gomock.InOrder(limiter.EXPECT().cancelReserve(ctx), limiter.EXPECT().cancelCheck(), limiter.EXPECT().close())
	}
	tx.Rollback(ctx)
	tx.Rollback(ctx)
}

func (s *txSuite) TestCloseAfterCheckKeepsConsumptionAndRejectsMoreWork() {
	tx, limiters := s.newTx(1)
	l := limiters[0]
	l.EXPECT().check(nil).Return(nil)
	s.NoError(tx.Check())

	l.EXPECT().close()
	tx.Close()
	s.ErrorIs(tx.Check(), errInvalidTxState)
	s.ErrorIs(tx.SkipCheck(), errInvalidTxState)
	s.ErrorIs(tx.Reserve(s.Context()), errInvalidTxState)
	s.ErrorIs(tx.Commit(s.Context()), errInvalidTxState)
	tx.Rollback(s.Context())
}

func (s *txSuite) TestReserveBeforeCheck() {
	tx, limiters := s.newTx(2)
	for _, limiter := range limiters {
		limiter.EXPECT().close()
	}

	s.ErrorIs(tx.Reserve(s.Context()), errInvalidTxState)
}

func (s *txSuite) TestCommitValidatesAllEntriesBeforeStarting() {
	ctx := s.Context()
	tx, limiters := s.newTx(2)
	expectChecks(nil, limiters)
	s.NoError(tx.Check())
	// Only the first entry has a reservation. Commit must validate both before making any RPC.
	tx.limiters[0].state = txStateReserved

	first, second := limiters[0], limiters[1]
	gomock.InOrder(first.EXPECT().cancelReserve(ctx), first.EXPECT().cancelCheck(), first.EXPECT().close())
	gomock.InOrder(second.EXPECT().cancelCheck(), second.EXPECT().close())

	s.ErrorIs(tx.Commit(ctx), errInvalidTxState)
}

func (s *txSuite) TestCheckTwiceRollsBack() {
	tx, limiters := s.newTx(1)
	l := limiters[0]
	l.EXPECT().check(nil).Return(nil)
	s.NoError(tx.Check())
	gomock.InOrder(l.EXPECT().cancelCheck(), l.EXPECT().close())

	s.ErrorIs(tx.Check(), errInvalidTxState)
}

func (s *txSuite) TestReserveTwiceRollsBack() {
	ctx := s.Context()
	tx, limiters := s.newTx(1)
	l := limiters[0]
	l.EXPECT().check(nil).Return(nil)
	s.NoError(tx.Check())
	l.EXPECT().reserve(ctx).Return(nil)
	s.NoError(tx.Reserve(ctx))
	gomock.InOrder(l.EXPECT().cancelReserve(ctx), l.EXPECT().cancelCheck(), l.EXPECT().close())

	s.ErrorIs(tx.Reserve(ctx), errInvalidTxState)
}

func (s *txSuite) TestSkipCheckCanReserveAndCommit() {
	ctx := s.Context()
	tx, limiters := s.newTx(1)
	l := limiters[0]
	s.NoError(tx.SkipCheck())

	gomock.InOrder(l.EXPECT().reserve(ctx).Return(nil), l.EXPECT().commit(ctx).Return(nil), l.EXPECT().close())
	s.NoError(tx.Reserve(ctx))
	s.NoError(tx.Commit(ctx))
	tx.Close()
}

func (s *txSuite) TestRollbackAfterSkipCheckCancelsOnlyReservations() {
	ctx := s.Context()
	tx, limiters := s.newTx(2)
	s.NoError(tx.SkipCheck())
	expectReserves(ctx, limiters)
	s.NoError(tx.Reserve(ctx))

	for _, limiter := range limiters {
		gomock.InOrder(limiter.EXPECT().cancelReserve(ctx), limiter.EXPECT().close())
	}
	tx.Rollback(ctx)
}

func (s *txSuite) TestInvalidSkipCheckDoesNotInventConsumedTokens() {
	tx, limiters := s.newTx(2)
	// If SkipCheck updates entries while validating them, rollback will refund a's nonexistent token.
	tx.limiters[1].state = txStateChecked
	limiters[0].EXPECT().close()
	gomock.InOrder(limiters[1].EXPECT().cancelCheck(), limiters[1].EXPECT().close())

	s.ErrorIs(tx.SkipCheck(), errInvalidTxState)
}

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

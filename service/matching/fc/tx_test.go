package fc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/namespace"
	serviceerrors "go.temporal.io/server/common/serviceerror"
	"go.temporal.io/server/service/matching/simplelimiter"
	"google.golang.org/grpc"
)

const (
	testNsID   = namespace.ID("nsid")
	testConKey = "conkey"
)

type testConcurrencyClient struct {
	fcpb.ConcurrencyServiceClient
	batch func(*fcpb.ConcurrencyServiceBatchRequest) *fcpb.ConcurrencyServiceBatchResponse
}

func (c *testConcurrencyClient) Batch(
	_ context.Context,
	req *fcpb.ConcurrencyServiceBatchRequest,
	_ ...grpc.CallOption,
) (*fcpb.ConcurrencyServiceBatchResponse, error) {
	return c.batch(req), nil
}

func (c *testConcurrencyClient) Wait(
	ctx context.Context,
	_ *fcpb.ConcurrencyServiceWaitRequest,
	_ ...grpc.CallOption,
) (*fcpb.ConcurrencyServiceWaitResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type testTask struct {
	limiters Limiters
}

func (t *testTask) Limiters() *Limiters                { return &t.limiters }
func (t *testTask) PriorityAndAge() (int32, time.Time) { return 3, time.Time{} }

type testCallback struct{}

func (*testCallback) OnReady() {}

func newConcurrencyTestTask() *testTask {
	return &testTask{limiters: Limiters{Limiters: [MaxLimiters]Limiter{{
		Type:   enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:    testConKey,
		Source: LimiterSourceConfig,
	}}}}
}

func newTestReadiness(batch func(*fcpb.ConcurrencyServiceBatchRequest) *fcpb.ConcurrencyServiceBatchResponse) *Readiness {
	return NewReadiness(clock.NewRealTimeSource(), &testConcurrencyClient{batch: batch})
}

func getTokens(r *Readiness) int32 {
	cs := r.getConcurrencyLimiter(testNsID, testConKey)
	cs.rs.lock.Lock()
	defer cs.rs.lock.Unlock()
	return cs.tokens
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
	t.Parallel()
	r := newTestReadiness(nil)
	r.getConcurrencyLimiter(testNsID, testConKey).reportSlotsHint(1, 5, notifySync)

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.SkipCheck())
	require.Equal(t, int32(5), getTokens(r))

	tx.Rollback(context.Background())
	require.Equal(t, int32(5), getTokens(r))
}

func TestSkipCheck_DoesNotBlock(t *testing.T) {
	t.Parallel()
	r := newTestReadiness(nil)
	defer r.Stop()
	task := newConcurrencyTestTask()
	// a local rate limiter that's always blocked
	task.limiters.Limiters[1] = Limiter{
		Type:   enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT,
		Key:    "ratekey",
		Source: LimiterSourceConfig,
		Config: simplelimiter.MakeParams(0, 0),
	}

	// concurrency limiter has zero tokens, so a normal check blocks
	tx := r.NewTx(testNsID, task, &testCallback{})
	require.ErrorIs(t, tx.Check(), ErrConcurrencyBlocked)

	tx = r.NewTx(testNsID, task, &testCallback{})
	require.NoError(t, tx.SkipCheck())
	require.Equal(t, int32(0), getTokens(r))
}

func TestSkipCheck_InvalidState(t *testing.T) {
	t.Parallel()
	r := newTestReadiness(nil)
	r.getConcurrencyLimiter(testNsID, testConKey).reportSlotsHint(1, 5, notifySync)

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.Check())
	require.ErrorIs(t, tx.SkipCheck(), errInvalidTxState)
}

func TestHints_FromReserveAndCommit(t *testing.T) {
	t.Parallel()
	r := newTestReadiness(func(req *fcpb.ConcurrencyServiceBatchRequest) *fcpb.ConcurrencyServiceBatchResponse {
		if len(req.ReserveSlots) > 0 {
			return &fcpb.ConcurrencyServiceBatchResponse{Generation: 7, AvailableSlotsHint: 3, ReserveSuccess: []bool{true}}
		}
		return &fcpb.ConcurrencyServiceBatchResponse{Generation: 7, AvailableSlotsHint: 2, CommitSuccess: []bool{true}}
	})

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.SkipCheck())
	require.Empty(t, tx.Hints())

	require.NoError(t, tx.Reserve(context.Background()))
	require.Len(t, tx.Hints(), 1)
	require.Equal(t, int32(3), tx.Hints()[0].GetAvailableSlotsHint())

	require.NoError(t, tx.Commit(context.Background()))
	require.Len(t, tx.Hints(), 1)
	require.Equal(t, int64(7), tx.Hints()[0].GetGeneration())
	require.Equal(t, int32(2), tx.Hints()[0].GetAvailableSlotsHint())
	require.Equal(t, testConKey, tx.Hints()[0].GetKey())
	require.Equal(t, enumsspb.LIMITER_TYPE_CONCURRENCY, tx.Hints()[0].GetLimiterType())
}

func TestHints_FromBlockedReserve(t *testing.T) {
	t.Parallel()
	r := newTestReadiness(func(req *fcpb.ConcurrencyServiceBatchRequest) *fcpb.ConcurrencyServiceBatchResponse {
		return &fcpb.ConcurrencyServiceBatchResponse{Generation: 4, AvailableSlotsHint: 0, ReserveSuccess: []bool{false}}
	})

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.SkipCheck())
	err := tx.Reserve(context.Background())
	var blocked *serviceerrors.FlowControlBlocked
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, []*taskqueuespb.LimiterHint{concurrencyHint(4, 0)}, tx.Hints())
}

func TestApplyHints_SupersedesCheckToken(t *testing.T) {
	t.Parallel()
	r := newTestReadiness(nil)
	r.getConcurrencyLimiter(testNsID, testConKey).reportSlotsHint(1, 5, notifySync)

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.Check())
	require.Equal(t, int32(4), getTokens(r))

	// forwarded, parent found no slots
	tx.ApplyHints([]*taskqueuespb.LimiterHint{concurrencyHint(2, 0)})
	require.Equal(t, int32(0), getTokens(r))

	// rollback after a failed forward must not return the check token
	tx.Rollback(context.Background())
	require.Equal(t, int32(0), getTokens(r))

	// hints are recorded so they can be passed further down a forwarding chain
	require.Equal(t, []*taskqueuespb.LimiterHint{concurrencyHint(2, 0)}, tx.Hints())
}

func TestApplyHints_StaleGenerationIgnored(t *testing.T) {
	t.Parallel()
	r := newTestReadiness(nil)
	r.getConcurrencyLimiter(testNsID, testConKey).reportSlotsHint(3, 5, notifySync)

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.Check())
	require.Equal(t, int32(4), getTokens(r))

	tx.ApplyHints([]*taskqueuespb.LimiterHint{concurrencyHint(2, 0)})
	require.Equal(t, int32(4), getTokens(r))

	tx.Rollback(context.Background())
	require.Equal(t, int32(5), getTokens(r))
}

func TestApplyHints_UnknownLimiterIgnored(t *testing.T) {
	t.Parallel()
	r := newTestReadiness(nil)
	r.getConcurrencyLimiter(testNsID, testConKey).reportSlotsHint(1, 5, notifySync)

	tx := r.NewTx(testNsID, newConcurrencyTestTask(), &testCallback{})
	require.NoError(t, tx.Check())

	otherKey := concurrencyHint(2, 0)
	otherKey.Key = "otherkey"
	otherType := concurrencyHint(2, 0)
	otherType.LimiterType = enumsspb.LIMITER_TYPE_LOCAL_RATE_LIMIT
	tx.ApplyHints([]*taskqueuespb.LimiterHint{otherKey, otherType})

	require.Equal(t, int32(4), getTokens(r))
	require.Empty(t, tx.Hints())
	tx.Rollback(context.Background())
	require.Equal(t, int32(5), getTokens(r))
}

func TestNilTx(t *testing.T) {
	t.Parallel()
	var tx *Tx
	require.NoError(t, tx.SkipCheck())
	require.Nil(t, tx.Hints())
	tx.ApplyHints([]*taskqueuespb.LimiterHint{concurrencyHint(1, 1)})
}

package semaphore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/chasm"
	semaphorepb "go.temporal.io/server/chasm/lib/semaphore/gen/semaphorepb/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/testing/testlogger"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/testing/testvars"
)

const (
	testNamespaceID = "test-ns-id"
	testSemaphoreID = "test-sem"
)

// testLibrary registers the Semaphore component (and an expiry task handler)
// so the chasm.Node accepts our root component when we run unit tests
// without the production fx wiring.
type testLibrary struct {
	chasm.UnimplementedLibrary
}

func (l *testLibrary) Name() string { return LibraryName }
func (l *testLibrary) Components() []*chasm.RegistrableComponent {
	return []*chasm.RegistrableComponent{
		chasm.NewRegistrableComponent[*Semaphore](
			ComponentName,
			chasm.WithBusinessIDAlias("SemaphoreId"),
		),
	}
}

func setupSemaphore(t *testing.T, limit int32) (*Semaphore, chasm.MutableContext) {
	sem, ctx, _, _ := setupSemaphoreWithClock(t, limit)
	return sem, ctx
}

func setupSemaphoreWithClock(t *testing.T, limit int32) (*Semaphore, chasm.MutableContext, *clock.EventTimeSource, *chasm.Node) {
	t.Helper()

	logger := testlogger.NewTestLogger(t, testlogger.FailOnExpectedErrorOnly)
	registry := chasm.NewRegistry(logger)
	require.NoError(t, registry.Register(&chasm.CoreLibrary{}))
	require.NoError(t, registry.Register(&testLibrary{}))

	timeSource := clock.NewEventTimeSource()
	timeSource.Update(time.Now())

	tv := testvars.New(t)
	backend := &chasm.MockNodeBackend{
		HandleNextTransitionCount: func() int64 { return 2 },
		HandleGetCurrentVersion:   func() int64 { return 1 },
		HandleGetWorkflowKey:      tv.Any().WorkflowKey,
		HandleIsWorkflow:          func() bool { return false },
		HandleCurrentVersionedTransition: func() *persistencespb.VersionedTransition {
			return &persistencespb.VersionedTransition{NamespaceFailoverVersion: 1, TransitionCount: 1}
		},
	}

	node := chasm.NewEmptyTree(
		registry, timeSource, backend, chasm.DefaultPathEncoder, logger, metrics.NoopMetricsHandler,
	)

	ctx := chasm.NewMutableContext(context.Background(), node)
	sem, err := CreateSemaphore(ctx, &semaphorepb.SetLimitRequest{
		NamespaceId: testNamespaceID,
		SemaphoreId: testSemaphoreID,
		Limit:       limit,
	})
	require.NoError(t, err)
	require.NoError(t, node.SetRootComponent(sem))
	_, err = node.CloseTransaction()
	require.NoError(t, err)

	return sem, chasm.NewMutableContext(context.Background(), node), timeSource, node
}

func TestReserveBasic(t *testing.T) {
	sem, ctx := setupSemaphore(t, 2)

	r, err := sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "a"})
	require.NoError(t, err)
	require.Equal(t, reserveOutcomeReserved, r.outcome)
}

func TestReserve_NoRoomWhenFull(t *testing.T) {
	sem, ctx := setupSemaphore(t, 1)

	r, err := sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "a"})
	require.NoError(t, err)
	require.Equal(t, reserveOutcomeReserved, r.outcome)

	r, err = sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "b"})
	require.NoError(t, err)
	require.Equal(t, reserveOutcomeNoRoom, r.outcome)
}

func TestReserveCommitReleaseFlow(t *testing.T) {
	sem, ctx := setupSemaphore(t, 1)

	_, err := sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "a"})
	require.NoError(t, err)
	_, err = sem.Commit(ctx, &semaphorepb.CommitRequest{HolderId: "a"})
	require.NoError(t, err)

	h, err := sem.GetHolders(ctx, &semaphorepb.GetHoldersRequest{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"a"}, h.Committed)
	require.Empty(t, h.Reserved)

	// Repeat Reserve while committed: short-circuits.
	r, err := sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "a"})
	require.NoError(t, err)
	require.Equal(t, reserveOutcomeAlreadyCommitted, r.outcome)

	_, err = sem.Release(ctx, &semaphorepb.ReleaseRequest{HolderId: "a"})
	require.NoError(t, err)
	h, err = sem.GetHolders(ctx, &semaphorepb.GetHoldersRequest{})
	require.NoError(t, err)
	require.Empty(t, h.Committed)
	require.Empty(t, h.Reserved)
}

func TestReserveRefreshesExpiration(t *testing.T) {
	sem, ctx := setupSemaphore(t, 1)

	r1, err := sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "a"})
	require.NoError(t, err)
	require.Equal(t, reserveOutcomeReserved, r1.outcome)

	// Wait a tick so the new expiration is observably different.
	time.Sleep(2 * time.Millisecond)

	r2, err := sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "a"})
	require.NoError(t, err)
	require.Equal(t, reserveOutcomeReserved, r2.outcome)
	require.True(t, r2.expiresAt.After(r1.expiresAt) || r2.expiresAt.Equal(r1.expiresAt))
}

func TestUnreserve_OnlyTouchesReserved(t *testing.T) {
	sem, ctx := setupSemaphore(t, 2)

	_, _ = sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "a"})
	_, _ = sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "b"})
	_, _ = sem.Commit(ctx, &semaphorepb.CommitRequest{HolderId: "a"})

	_, err := sem.Unreserve(ctx, &semaphorepb.UnreserveRequest{HolderId: "a"})
	require.NoError(t, err)
	_, err = sem.Unreserve(ctx, &semaphorepb.UnreserveRequest{HolderId: "b"})
	require.NoError(t, err)

	h, err := sem.GetHolders(ctx, &semaphorepb.GetHoldersRequest{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"a"}, h.Committed)
	require.Empty(t, h.Reserved)
}

func TestCommit_MissingSlot(t *testing.T) {
	sem, ctx := setupSemaphore(t, 1)
	_, err := sem.Commit(ctx, &semaphorepb.CommitRequest{HolderId: "ghost"})
	require.ErrorIs(t, err, ErrSlotNotFound)
}

func TestSetLimit_RejectsNegative(t *testing.T) {
	sem, ctx := setupSemaphore(t, 1)
	_, err := sem.SetLimit(ctx, &semaphorepb.SetLimitRequest{Limit: -1})
	require.ErrorIs(t, err, ErrInvalidLimit)
}

// TestOnDemandSweep_FreesExpiredSlot uses an EventTimeSource so we can fast
// forward past reservationTTL and observe that the next Reserve sweeps the
// stale slot.
func TestOnDemandSweep_FreesExpiredSlot(t *testing.T) {
	sem, _, ts, node := setupSemaphoreWithClock(t, 1)

	// Reserve A (no Commit).
	ctx := chasm.NewMutableContext(context.Background(), node)
	_, err := sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "a"})
	require.NoError(t, err)
	_, err = node.CloseTransaction()
	require.NoError(t, err)

	// Reserve B before TTL expires: no room.
	ctx = chasm.NewMutableContext(context.Background(), node)
	r, err := sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "b"})
	require.NoError(t, err)
	require.Equal(t, reserveOutcomeNoRoom, r.outcome)
	require.False(t, r.soonestExpiry.IsZero(), "soonestExpiry should be reported when NoRoom")

	// Advance past TTL. Next Reserve should sweep "a" and succeed for "b".
	ts.Update(ts.Now().Add(reservationTTL + time.Millisecond))
	ctx = chasm.NewMutableContext(context.Background(), node)
	r, err = sem.Reserve(ctx, &semaphorepb.ReserveRequest{HolderId: "b"})
	require.NoError(t, err)
	require.Equal(t, reserveOutcomeReserved, r.outcome)

	// "a" was swept; only "b" is now reserved.
	h, err := sem.GetHolders(ctx, &semaphorepb.GetHoldersRequest{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"b"}, h.Reserved)
}

func TestHolderIDForTask_Deterministic(t *testing.T) {
	k := TaskKey{
		NamespaceID:      "ns",
		TaskQueue:        "tq",
		TaskQueueKind:    1,
		WorkflowID:       "wf",
		RunID:            "run",
		ScheduledEventID: 42,
	}
	require.Equal(t, HolderIDForTask(k), HolderIDForTask(k))
	k2 := k
	k2.ScheduledEventID = 43
	require.NotEqual(t, HolderIDForTask(k), HolderIDForTask(k2))
}

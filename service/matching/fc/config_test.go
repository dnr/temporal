package fc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/testing/await"
)

func TestReadinessDynamicConfig(t *testing.T) {
	client := dynamicconfig.NewMemoryClient()
	dc := dynamicconfig.NewCollection(client, log.NewNoopLogger())
	opts := defaultCleanupOptions
	opts.BatchSize = 1
	t.Cleanup(client.OverrideSetting(ReadinessCleanupOptions, opts))
	t.Cleanup(client.OverrideSetting(ReadinessConcurrencyBlockedRetention, 10*time.Second))
	ts := clock.NewEventTimeSource().Update(time.Now())
	r := readinessProvider(ts, nil, dc)
	t.Cleanup(r.Stop)
	require.Equal(t, opts, r.config.cleanup())

	ns := namespace.ID("ns")
	lim := r.getConcurrencyLimiter(ns, "key")
	lim.tokens = 0
	lim.unpin()
	ts.Advance(9 * time.Second)
	lim.rs.cleanup(1000, r.gcParameters())
	require.Len(t, lim.rs.concurrencyLimiters, 1)
	t.Cleanup(client.OverrideSetting(ReadinessConcurrencyBlockedRetention, time.Second))
	lim.rs.cleanup(1000, r.gcParameters())
	require.Empty(t, lim.rs.concurrencyLimiters)

	t.Cleanup(client.OverrideSetting(ReadinessCleanupOptions, CleanupOptions{}))
	require.Equal(t, defaultCleanupOptions, r.config.cleanup())
}

func TestCancellationTimeoutReleasesPin(t *testing.T) {
	closed := make(chan error, 1)
	client := &testClient{batch: func(ctx context.Context, _ *fcpb.ConcurrencyServiceBatchRequest) (*fcpb.ConcurrencyServiceBatchResponse, error) {
		<-ctx.Done()
		closed <- ctx.Err()
		return nil, ctx.Err()
	}}
	r, _ := newTestReadiness(t, client)
	dc := dynamicconfig.NewCollection(dynamicconfig.StaticClient{
		ReadinessCancelReservationTimeout.Key(): time.Millisecond,
	}, log.NewNoopLogger())
	r.config = NewReadinessConfig(dc)
	lim := r.getConcurrencyLimiter(namespace.ID("ns"), "key")
	lim.cancelReserve(context.Background(), "slot")
	lim.unpin()
	require.ErrorIs(t, <-closed, context.DeadlineExceeded)
	await.RequireTrue(t, func() bool {
		lim.rs.lock.Lock()
		defer lim.rs.lock.Unlock()
		return lim.pins == 0
	}, time.Second, time.Millisecond)
}

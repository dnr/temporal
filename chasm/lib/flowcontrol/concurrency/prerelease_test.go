package concurrency

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"google.golang.org/grpc"
)

func TestPreReleaser(t *testing.T) {
	var mu sync.Mutex
	var released []string
	calls := make(chan struct{}, 10)
	client := &testClient{batch: func(
		_ context.Context,
		req *fcpb.ConcurrencyServiceBatchRequest,
		_ ...grpc.CallOption,
	) (*fcpb.ConcurrencyServiceBatchResponse, error) {
		defer func() { calls <- struct{}{} }()
		require.Equal(t, "ns", req.GetNamespaceId())
		if req.GetKey() == "bad" {
			return nil, errors.New("release failed")
		}
		mu.Lock()
		defer mu.Unlock()
		released = append(released, req.GetRunId()+"/"+req.GetReleaseSlots()[0])
		return &fcpb.ConcurrencyServiceBatchResponse{}, nil
	}}
	opts := PreReleaseOptions{Timeout: time.Second}
	p := &PreReleaser{
		client: client,
		opts:   func() PreReleaseOptions { return opts },
	}
	good := &taskqueuespb.LimiterRef{LimiterType: enumsspb.LIMITER_TYPE_CONCURRENCY, Key: "good", SlotId: "slot-1", RunId: "run-1"}
	bad := &taskqueuespb.LimiterRef{LimiterType: enumsspb.LIMITER_TYPE_CONCURRENCY, Key: "bad", SlotId: "slot-2"}
	other := &taskqueuespb.LimiterRef{Key: "other", SlotId: "slot-3"}
	refs := []*taskqueuespb.LimiterRef{good, bad, other}

	var nilP *PreReleaser
	require.Nil(t, nilP.Release(context.Background(), "ns", refs))

	opts.Mode = PreReleaseModeOff
	require.Nil(t, p.Release(context.Background(), "ns", refs))
	require.Empty(t, calls)

	opts.Mode = PreReleaseModeSync
	require.Equal(t, []*taskqueuespb.LimiterRef{good}, p.Release(context.Background(), "ns", refs))
	require.Len(t, calls, 2) // the non-concurrency ref is skipped
	<-calls
	<-calls
	require.Equal(t, []string{"run-1/slot-1"}, released)

	opts.Mode = PreReleaseModeAsync
	require.Nil(t, p.Release(context.Background(), "ns", []*taskqueuespb.LimiterRef{good}))
	<-calls
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"run-1/slot-1", "run-1/slot-1"}, released)
}

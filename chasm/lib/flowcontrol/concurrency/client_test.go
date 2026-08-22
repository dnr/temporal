package concurrency

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/stream_batcher"
	"google.golang.org/grpc"
)

type testClient struct {
	fcpb.ConcurrencyServiceClient
	batch func(context.Context, *fcpb.ConcurrencyBatchRequest, ...grpc.CallOption) (*fcpb.ConcurrencyBatchResponse, error)
}

func (c *testClient) Batch(
	ctx context.Context,
	req *fcpb.ConcurrencyBatchRequest,
	opts ...grpc.CallOption,
) (*fcpb.ConcurrencyBatchResponse, error) {
	return c.batch(ctx, req, opts...)
}

func TestBatchingClient(t *testing.T) {
	callCount := 0
	batchedRequests := make(chan *fcpb.ConcurrencyBatchRequest, 1)
	client := NewBatchingClient(
		&testClient{batch: func(
			_ context.Context,
			req *fcpb.ConcurrencyBatchRequest,
			_ ...grpc.CallOption,
		) (*fcpb.ConcurrencyBatchResponse, error) {
			callCount++
			batchedRequests <- req
			return &fcpb.ConcurrencyBatchResponse{
				ReserveSuccess: []bool{true, false},
				CommitSuccess:  []bool{true},
				Generation:     42,
			}, nil
		}},
		stream_batcher.BatcherOptions{
			MaxItems: 2,
			MinDelay: time.Second,
			MaxDelay: time.Second,
			IdleTime: time.Minute,
		},
		clock.NewRealTimeSource(),
	)

	type result struct {
		res *fcpb.ConcurrencyBatchResponse
		err error
	}
	results := make(chan result, 2)
	requests := []*fcpb.ConcurrencyBatchRequest{
		{
			NamespaceId:            "namespace-id",
			Key:                    "limiter-key",
			ReserveSlots:           []string{"reserve-1"},
			CancelReservationSlots: []string{"cancel-1"},
			ConfigUpdate:           &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 10},
			ConfigUpdateVersion:    1,
		},
		{
			NamespaceId:         "namespace-id",
			Key:                 "limiter-key",
			ReserveSlots:        []string{"reserve-2"},
			CommitSlots:         []string{"commit-1"},
			ReleaseSlots:        []string{"release-1"},
			ConfigUpdate:        &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 20},
			ConfigUpdateVersion: 2,
		},
	}

	var wg sync.WaitGroup
	for _, req := range requests {
		wg.Go(func() {
			res, err := client.Batch(t.Context(), req)
			results <- result{res: res, err: err}
		})
	}
	wg.Wait()
	close(results)

	require.Equal(t, 1, callCount)
	batchedRequest := <-batchedRequests
	require.Equal(t, "namespace-id", batchedRequest.GetNamespaceId())
	require.Equal(t, "limiter-key", batchedRequest.GetKey())
	require.ElementsMatch(t, []string{"reserve-1", "reserve-2"}, batchedRequest.GetReserveSlots())
	require.Equal(t, []string{"cancel-1"}, batchedRequest.GetCancelReservationSlots())
	require.Equal(t, []string{"commit-1"}, batchedRequest.GetCommitSlots())
	require.Equal(t, []string{"release-1"}, batchedRequest.GetReleaseSlots())
	require.Equal(t, int64(2), batchedRequest.GetConfigUpdateVersion())
	require.Equal(t, int32(20), batchedRequest.GetConfigUpdate().GetConcurrentTasks())
	var reserveResults []bool
	var commitResults []bool
	for result := range results {
		require.NoError(t, result.err)
		require.Equal(t, int64(42), result.res.GetGeneration())
		reserveResults = append(reserveResults, result.res.GetReserveSuccess()...)
		commitResults = append(commitResults, result.res.GetCommitSuccess()...)
	}
	require.ElementsMatch(t, []bool{true, false}, reserveResults)
	require.Equal(t, []bool{true}, commitResults)
}

package activity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/chasm"
	"go.temporal.io/server/chasm/lib/activity/gen/activitypb/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/streambatcher"
	"google.golang.org/grpc"
)

type testConcurrencyServiceClient struct {
	fcpb.ConcurrencyServiceClient
	batch func(context.Context, *fcpb.ConcurrencyServiceBatchRequest, ...grpc.CallOption) (*fcpb.ConcurrencyServiceBatchResponse, error)
}

func (c *testConcurrencyServiceClient) Batch(
	ctx context.Context,
	request *fcpb.ConcurrencyServiceBatchRequest,
	opts ...grpc.CallOption,
) (*fcpb.ConcurrencyServiceBatchResponse, error) {
	return c.batch(ctx, request, opts...)
}

func TestReleaseLimiterTaskExecute(t *testing.T) {
	releaseErr := errors.New("release failed")
	var requests []*fcpb.ConcurrencyServiceBatchRequest
	handler := newReleaseLimiterTaskHandler(&testConcurrencyServiceClient{
		batch: func(
			_ context.Context,
			request *fcpb.ConcurrencyServiceBatchRequest,
			_ ...grpc.CallOption,
		) (*fcpb.ConcurrencyServiceBatchResponse, error) {
			// client id is generated per batching client, ignore it
			request.ClientId = ""
			requests = append(requests, request)
			if request.GetReleaseSlots()[0] == "bad-slot" {
				return nil, releaseErr
			}
			return &fcpb.ConcurrencyServiceBatchResponse{}, nil
		},
	}, &Config{FlowControlClientBatcherOptions: dynamicconfig.GetTypedPropertyFn(streambatcher.BatcherOptions{
		MaxItems: 1,
		IdleTime: time.Minute,
	})})
	task := &activitypb.ReleaseLimiterTask{Limiters: []*taskqueuespb.LimiterRef{
		{LimiterType: enumsspb.LIMITER_TYPE_CONCURRENCY, Key: "first-key", SlotId: "bad-slot"},
		{LimiterType: enumsspb.LIMITER_TYPE_UNSPECIFIED, Key: "ignored-key", SlotId: "ignored-slot"},
		{LimiterType: enumsspb.LIMITER_TYPE_CONCURRENCY, Key: "second-key", SlotId: "good-slot"},
	}}

	err := handler.Execute(t.Context(), chasm.ComponentRef{ExecutionKey: chasm.ExecutionKey{
		NamespaceID: "namespace-id",
	}}, chasm.TaskAttributes{}, task)

	require.ErrorIs(t, err, releaseErr)
	require.Equal(t, []*fcpb.ConcurrencyServiceBatchRequest{
		{NamespaceId: "namespace-id", Key: "first-key", ReleaseSlots: []string{"bad-slot"}},
		{NamespaceId: "namespace-id", Key: "second-key", ReleaseSlots: []string{"good-slot"}},
	}, requests)
}

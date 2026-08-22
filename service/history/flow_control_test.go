package history

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/serviceerror"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"google.golang.org/grpc"
)

type verifyReleaseClient struct {
	fcpb.ConcurrencyServiceClient
	response *fcpb.VerifyReleaseResponse
	err      error
	request  *fcpb.VerifyReleaseRequest
}

func (c *verifyReleaseClient) VerifyRelease(
	_ context.Context,
	request *fcpb.VerifyReleaseRequest,
	_ ...grpc.CallOption,
) (*fcpb.VerifyReleaseResponse, error) {
	c.request = request
	return c.response, c.err
}

func TestVerifyLimiterReleases(t *testing.T) {
	release := &taskqueuespb.LimiterRelease{
		Limiter: &taskqueuespb.LimiterRef{
			LimiterType: enumsspb.LIMITER_TYPE_CONCURRENCY,
			Key:         "limiter-key",
			SlotId:      "slot-id",
		},
		ComponentRef: []byte("component-ref"),
	}
	client := &verifyReleaseClient{response: &fcpb.VerifyReleaseResponse{Released: true}}

	released, err := verifyLimiterReleases(t.Context(), client, "namespace-id", []*taskqueuespb.LimiterRelease{release})

	require.NoError(t, err)
	require.True(t, released)
	require.Equal(t, &fcpb.VerifyReleaseRequest{
		NamespaceId:  "namespace-id",
		Key:          "limiter-key",
		Slots:        []string{"slot-id"},
		ComponentRef: []byte("component-ref"),
	}, client.request)

	client.err = serviceerror.NewUnavailable("not replicated")
	released, err = verifyLimiterReleases(t.Context(), client, "namespace-id", []*taskqueuespb.LimiterRelease{release})
	require.NoError(t, err)
	require.False(t, released)
}

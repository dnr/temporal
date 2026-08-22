package concurrency

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	fcpb "go.temporal.io/server/chasm/lib/flowcontrol/gen/flowcontrolpb/v1"
	"google.golang.org/grpc"
)

type fakeConcurrencyClient struct {
	fcpb.ConcurrencyServiceClient
	batchReqs  []*fcpb.ConcurrencyBatchRequest
	batchErr   func(*fcpb.ConcurrencyBatchRequest) error
	verifyReqs []*fcpb.ConcurrencyVerifyRequest
	verify     func(*fcpb.ConcurrencyVerifyRequest) (*fcpb.ConcurrencyVerifyResponse, error)
}

func (c *fakeConcurrencyClient) Batch(
	_ context.Context,
	req *fcpb.ConcurrencyBatchRequest,
	_ ...grpc.CallOption,
) (*fcpb.ConcurrencyBatchResponse, error) {
	c.batchReqs = append(c.batchReqs, req)
	if c.batchErr != nil {
		if err := c.batchErr(req); err != nil {
			return nil, err
		}
	}
	return &fcpb.ConcurrencyBatchResponse{}, nil
}

func (c *fakeConcurrencyClient) Verify(
	_ context.Context,
	req *fcpb.ConcurrencyVerifyRequest,
	_ ...grpc.CallOption,
) (*fcpb.ConcurrencyVerifyResponse, error) {
	c.verifyReqs = append(c.verifyReqs, req)
	return c.verify(req)
}

func concurrencyRef(key, slotID string) *taskqueuespb.LimiterRef {
	return &taskqueuespb.LimiterRef{
		LimiterType: enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:         key,
		SlotId:      slotID,
	}
}

func TestReleaseSlotsGroupsByLimiter(t *testing.T) {
	client := &fakeConcurrencyClient{}
	refs := []*taskqueuespb.LimiterRef{
		concurrencyRef("limiter-1", "slot-a"),
		concurrencyRef("limiter-2", "slot-b"),
		concurrencyRef("limiter-1", "slot-c"),
		{LimiterType: enumsspb.LIMITER_TYPE_UNSPECIFIED, Key: "limiter-3", SlotId: "slot-d"},
	}

	require.NoError(t, ReleaseSlots(context.Background(), client, "ns", refs))
	require.Len(t, client.batchReqs, 2)
	require.Equal(t, "limiter-1", client.batchReqs[0].GetKey())
	require.Equal(t, []string{"slot-a", "slot-c"}, client.batchReqs[0].GetReleaseSlots())
	require.Equal(t, "limiter-2", client.batchReqs[1].GetKey())
	require.Equal(t, []string{"slot-b"}, client.batchReqs[1].GetReleaseSlots())
}

func TestReleaseSlotsAttemptsAllLimitersOnError(t *testing.T) {
	releaseErr := serviceerror.NewUnavailable("nope")
	client := &fakeConcurrencyClient{
		batchErr: func(req *fcpb.ConcurrencyBatchRequest) error {
			if req.GetKey() == "limiter-1" {
				return releaseErr
			}
			return nil
		},
	}
	refs := []*taskqueuespb.LimiterRef{
		concurrencyRef("limiter-1", "slot-a"),
		concurrencyRef("limiter-2", "slot-b"),
	}

	require.ErrorIs(t, ReleaseSlots(context.Background(), client, "ns", refs), releaseErr)
	require.Len(t, client.batchReqs, 2)
}

func TestSlotsReleased(t *testing.T) {
	refs := []*taskqueuespb.LimiterRef{
		concurrencyRef("limiter-1", "slot-a"),
		concurrencyRef("limiter-2", "slot-b"),
	}
	verifyErr := serviceerror.NewUnavailable("nope")

	for _, tc := range []struct {
		name     string
		verify   func(*fcpb.ConcurrencyVerifyRequest) (*fcpb.ConcurrencyVerifyResponse, error)
		released bool
		err      error
	}{{
		name: "all released",
		verify: func(*fcpb.ConcurrencyVerifyRequest) (*fcpb.ConcurrencyVerifyResponse, error) {
			return &fcpb.ConcurrencyVerifyResponse{Released: []bool{true}}, nil
		},
		released: true,
	}, {
		name: "one still held",
		verify: func(req *fcpb.ConcurrencyVerifyRequest) (*fcpb.ConcurrencyVerifyResponse, error) {
			return &fcpb.ConcurrencyVerifyResponse{Released: []bool{req.GetKey() == "limiter-1"}}, nil
		},
	}, {
		// A limiter we can't see says nothing about whether the release happened.
		name: "limiter missing",
		verify: func(*fcpb.ConcurrencyVerifyRequest) (*fcpb.ConcurrencyVerifyResponse, error) {
			return nil, serviceerror.NewNotFound("no limiter")
		},
	}, {
		name: "verify fails",
		verify: func(*fcpb.ConcurrencyVerifyRequest) (*fcpb.ConcurrencyVerifyResponse, error) {
			return nil, verifyErr
		},
		err: verifyErr,
	}, {
		name: "malformed response",
		verify: func(*fcpb.ConcurrencyVerifyRequest) (*fcpb.ConcurrencyVerifyResponse, error) {
			return &fcpb.ConcurrencyVerifyResponse{}, nil
		},
		err: serviceerror.NewInternal("invalid concurrency verify response"),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeConcurrencyClient{verify: tc.verify}
			released, err := SlotsReleased(context.Background(), client, "ns", refs)
			if tc.err != nil {
				require.Error(t, err)
				require.Equal(t, tc.err.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.released, released)
		})
	}
}

func TestVerifyFnReportsCommittedSlots(t *testing.T) {
	now := time.Now().UTC()
	limiter := &Component{
		ConcurrencyState: &fcpb.ConcurrencyState{
			Config: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 5},
		},
	}
	require.True(t, limiter.reserve("committed", now))
	require.True(t, limiter.commit("committed"))
	require.True(t, limiter.reserve("reserved", now))

	ress, err := verifyFn(limiter, nil, []verifyReq{{
		req: &fcpb.ConcurrencyVerifyRequest{SlotIds: []string{"committed", "reserved", "unknown"}},
	}})
	require.NoError(t, err)
	require.Len(t, ress, 1)
	// Only a committed slot is one that Release still has to free; a bare reservation expires
	// on its own.
	require.Equal(t, []bool{false, true, true}, ress[0].GetReleased())
}

package taskqueue

import (
	"testing"

	"github.com/stretchr/testify/require"
	enumsspb "go.temporal.io/server/api/enums/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
)

func TestNeedsRelease(t *testing.T) {
	testCases := []struct {
		name string
		ref  *taskqueuespb.LimiterRef
		want bool
	}{
		{
			name: "nil",
		},
		{
			name: "unspecified",
			ref:  &taskqueuespb.LimiterRef{},
		},
		{
			name: "concurrency",
			ref: &taskqueuespb.LimiterRef{
				LimiterType: enumsspb.LIMITER_TYPE_CONCURRENCY,
			},
			want: true,
		},
		{
			name: "unknown",
			ref: &taskqueuespb.LimiterRef{
				LimiterType: enumsspb.LimiterType(100),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(t, testCase.want, NeedsRelease(testCase.ref))
		})
	}
}

func TestContainsLimiterRef(t *testing.T) {
	ref := &taskqueuespb.LimiterRef{
		LimiterType: enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:         "key",
		SlotId:      "slot-id",
	}

	require.True(t, ContainsLimiterRef([]*taskqueuespb.LimiterRef{ref}, ref))
	require.False(t, ContainsLimiterRef([]*taskqueuespb.LimiterRef{ref}, &taskqueuespb.LimiterRef{
		LimiterType: enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:         "key",
		SlotId:      "other-slot-id",
	}))
}

func TestLimiterReleaseHelpers(t *testing.T) {
	ref := &taskqueuespb.LimiterRef{
		LimiterType: enumsspb.LIMITER_TYPE_CONCURRENCY,
		Key:         "key",
		SlotId:      "slot-id",
	}
	release := &taskqueuespb.LimiterRelease{Limiter: ref, ComponentRef: []byte("ref")}

	require.Same(t, release, FindLimiterRelease([]*taskqueuespb.LimiterRelease{release}, ref))
	require.Nil(t, FindLimiterRelease([]*taskqueuespb.LimiterRelease{release}, &taskqueuespb.LimiterRef{Key: "other"}))
	require.True(t, ReleaseRecorded([]*taskqueuespb.LimiterRelease{release}))
	require.False(t, ReleaseRecorded(nil))
	require.False(t, ReleaseRecorded([]*taskqueuespb.LimiterRelease{{Limiter: ref}}))
}

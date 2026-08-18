package matching

import (
	"testing"

	"github.com/stretchr/testify/require"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestBuildConcurrencyLimitConfig(t *testing.T) {
	updateTime := timestamppb.Now()

	t.Run("set", func(t *testing.T) {
		config := buildConcurrencyLimitConfig(
			&workflowservice.UpdateTaskQueueConfigRequest_ConcurrencyLimitUpdate{
				ConcurrencyLimit: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 42},
				Reason:           "protect downstream",
			},
			updateTime,
			"test-identity",
		)

		require.Equal(t, int32(42), config.GetConcurrencyLimit().GetConcurrentTasks())
		require.Equal(t, "protect downstream", config.GetMetadata().GetReason())
		require.Equal(t, "test-identity", config.GetMetadata().GetUpdateIdentity())
		require.Equal(t, updateTime, config.GetMetadata().GetUpdateTime())
	})

	t.Run("remove", func(t *testing.T) {
		config := buildConcurrencyLimitConfig(
			&workflowservice.UpdateTaskQueueConfigRequest_ConcurrencyLimitUpdate{Reason: "no longer needed"},
			updateTime,
			"test-identity",
		)

		require.Nil(t, config.GetConcurrencyLimit())
		require.Equal(t, "no longer needed", config.GetMetadata().GetReason())
		require.Equal(t, "test-identity", config.GetMetadata().GetUpdateIdentity())
		require.Equal(t, updateTime, config.GetMetadata().GetUpdateTime())
	})
}

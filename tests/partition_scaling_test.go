package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	enumspb "go.temporal.io/api/enums/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	taskqueuespb "go.temporal.io/server/api/taskqueue/v1"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/testing/taskpoller"
	"go.temporal.io/server/common/testing/testvars"
	"go.temporal.io/server/tests/testcore"
)

var scalerEnvOptions = []testcore.TestOption{
	testcore.WithDynamicConfig(dynamicconfig.MatchingUseNewMatcher, true),
	// default dynamic config to 1 to ensure we turn on managed scaling immediately
	testcore.WithDynamicConfig(dynamicconfig.MatchingNumTaskqueueReadPartitions, 1),
	testcore.WithDynamicConfig(dynamicconfig.MatchingNumTaskqueueWritePartitions, 1),
	testcore.WithDynamicConfig(dynamicconfig.MatchingPartitionScaleManager, dynamicconfig.PartitionScaleManagerSettings{
		MaxRate:      100,         // don't limit speed of changes
		BatchSize:    1,           // always go directly to scaler
		IdleInterval: time.Second, // ping scaler on idle
	}),
}

func TestPartitionScaling_Up(t *testing.T) {
	s := testcore.NewEnv(t, scalerEnvOptions...)

	s.T().Log("set to 2 partitions using scaler")
	s.OverrideDynamicConfig(dynamicconfig.MatchingSimplePartitionScaler, dynamicconfig.SimplePartitionScalerSettings{
		Enabled: true,
		Fixed:   2,
	})

	s.T().Log("start sending 10 tasks/s")
	stopTasks := scalerBackgroundTasks(s, s.Tv(), 10)
	defer stopTasks()

	s.T().Log("wait until partitions 0,1 have 5 tasks backlog")
	s.Eventually(scalerBacklogAtLeast(s, s.Tv(), 5, 0, 1), 15*time.Second, time.Second)

	s.T().Log("check that 2,3 have no tasks (leave 4,5 unloaded)")
	s.True(scalerBacklogEmpty(s, s.Tv(), 5, 2, 3)())

	s.T().Log("set to 6 partitions using scaler")
	s.OverrideDynamicConfig(dynamicconfig.MatchingSimplePartitionScaler, dynamicconfig.SimplePartitionScalerSettings{
		Enabled: true,
		Fixed:   6,
	})

	s.T().Log("wait until partitions 2,3,4,5 have 5 tasks backlog")
	s.Eventually(scalerBacklogAtLeast(s, s.Tv(), 5, 2, 3, 4, 5), 15*time.Second, time.Second)

	s.T().Log("stop sending tasks")
	stopTasks()

	s.T().Log("start background polls")
	stopPolls := scalerBackgroundPolls(s, s.Tv(), s.TaskPoller(), 3)
	defer stopPolls()

	s.T().Log("wait until all are drained")
	s.Eventually(scalerBacklogEmpty(s, s.Tv(), 5, 0, 1, 2, 3, 4, 5), 15*time.Second, time.Second)
}

func TestPartitionScaling_Down(t *testing.T) {
	s := testcore.NewEnv(t, scalerEnvOptions...)

	// test plan:
	// set to 6 partitions using scaler (fixed)
	// start tasks (10/s)
	// wait until all have 5 tasks (~3s)
	// set to 4 partitions using scaler (fixed)
	// wait until 1s has gone by with 4,5 getting no newly added tasks
	// stop tasks
	// start pollers
	// wait until

	s.T().Log("set to 6 partitions using scaler")
	s.OverrideDynamicConfig(dynamicconfig.MatchingSimplePartitionScaler, dynamicconfig.SimplePartitionScalerSettings{
		Enabled: true,
		Fixed:   6,
	})

	s.T().Log("start sending 10 tasks/s")
	stopTasks := scalerBackgroundTasks(s, s.Tv(), 10)
	defer stopTasks()

	s.T().Log("wait until partitions 0-5 have 5 tasks backlog")
	s.Eventually(scalerBacklogAtLeast(s, s.Tv(), 5, 0, 1, 2, 3, 4, 5), 15*time.Second, time.Second)

	s.T().Log("set to 4 partitions using scaler")
	s.OverrideDynamicConfig(dynamicconfig.MatchingSimplePartitionScaler, dynamicconfig.SimplePartitionScalerSettings{
		Enabled: true,
		Fixed:   4,
	})

	s.T().Log("wait until 4,5 see no new tasks over a 1s window")
	s.EventuallyWithT(func(c *assert.CollectT) {
		fourBacklog, err := scalerGetBacklog(s, s.Tv(), 4)
		require.NoError(c, err)
		fiveBacklog, err := scalerGetBacklog(s, s.Tv(), 5)
		require.NoError(c, err)

		time.Sleep(time.Second) //nolint:forbidigo // trying to test a negative

		fourBacklog2, err := scalerGetBacklog(s, s.Tv(), 4)
		require.NoError(c, err)
		fiveBacklog2, err := scalerGetBacklog(s, s.Tv(), 5)
		require.NoError(c, err)

		require.Equal(c, fourBacklog, fourBacklog2)
		require.Equal(c, fiveBacklog, fiveBacklog2)
	}, 15*time.Second, time.Millisecond)

	s.T().Log("stop sending tasks")
	stopTasks()

	s.T().Log("start background polls")
	stopPolls := scalerBackgroundPolls(s, s.Tv(), s.TaskPoller(), 3)
	defer stopPolls()

	s.T().Log("wait until all are drained")
	s.Eventually(scalerBacklogEmpty(s, s.Tv(), 5, 0, 1, 2, 3, 4, 5), 15*time.Second, time.Second)

	// Note that this test does not test the read count eventually drops!
	// That's in another test (TODO).
}

// test migration from old dc to scaler with > 4:
// set 4 partitions using old dynamic config
// start creating tasks in the background, 5/s
// no pollers yet
// wait until all 4 partitions have 5 tasks in backlog
// set scaler to 6
// wait until parts 4,5 have 5 tasks in backlog
// stop tasks
// start pollers
// wait until all have no backlog

// test migration from old dc to scaler with < 4:
// set 4 partitions using old dynamic config
// start creating tasks in the background, 5/s
// no pollers yet
// wait until all 4 partitions have 5 tasks in backlog
// set scaler to 2
// wait until partitions 0,1 have 10 tasks in backlog
// stop tasks
// start pollers
// wait until all have no backlog

func scalerBackgroundTasks(s testcore.Env, tv *testvars.TestVars, rate float32) func() {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		t := time.NewTicker(time.Duration(float32(time.Second) / rate))
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_, _ = s.FrontendClient().StartWorkflowExecution(ctx, &workflowservice.StartWorkflowExecutionRequest{
					Namespace:    s.Namespace().String(),
					WorkflowId:   uuid.NewString(),
					WorkflowType: tv.WorkflowType(),
					TaskQueue:    tv.TaskQueue(),
					Identity:     tv.ClientIdentity(),
					RequestId:    uuid.NewString(),
				})
			}
		}
	}()

	return cancel
}

func scalerBackgroundPolls(s testcore.Env, tv *testvars.TestVars, tp *taskpoller.TaskPoller, workers int) func() {
	ctx, cancel := context.WithCancel(context.Background())

	for range workers {
		go func() {
			for ctx.Err() == nil {
				_, _ = tp.PollAndHandleWorkflowTask(
					tv,
					taskpoller.CompleteWorkflowHandler,
					taskpoller.WithContext(ctx),
				)
			}
		}()
	}

	return cancel
}

func scalerGetBacklog(s testcore.Env, tv *testvars.TestVars, part int) (int, error) {
	ctx := testcore.NewContext()
	res, err := s.AdminClient().DescribeTaskQueuePartition(ctx, &adminservice.DescribeTaskQueuePartitionRequest{
		Namespace: s.Namespace().String(),
		TaskQueuePartition: &taskqueuespb.TaskQueuePartition{
			TaskQueue:     tv.TaskQueue().Name,
			TaskQueueType: enumspb.TASK_QUEUE_TYPE_WORKFLOW,
			PartitionId:   &taskqueuespb.TaskQueuePartition_NormalPartitionId{NormalPartitionId: int32(part)},
		},
		BuildIds: &taskqueuepb.TaskQueueVersionSelection{Unversioned: true},
	})
	if err != nil {
		return 0, err
	}
	var count int
	for _, versionInfoInternal := range res.VersionsInfoInternal {
		for _, st := range versionInfoInternal.PhysicalTaskQueueInfo.InternalTaskQueueStatus {
			count += int(st.ApproximateBacklogCount)
		}
	}
	return count, nil
}

func scalerBacklogAtLeast(s testcore.Env, tv *testvars.TestVars, target int, parts ...int) func() bool {
	return func() bool {
		for _, part := range parts {
			count, err := scalerGetBacklog(s, tv, part)
			if err != nil || count < target {
				return false
			}
		}
		return true
	}
}

func scalerBacklogEmpty(s testcore.Env, tv *testvars.TestVars, parts ...int) func() bool {
	return func() bool {
		for _, part := range parts {
			count, err := scalerGetBacklog(s, tv, part)
			if err != nil || count > 0 {
				return false
			}
		}
		return true
	}
}

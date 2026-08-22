package xdc

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	enumspb "go.temporal.io/api/enums/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	sdkclient "go.temporal.io/sdk/client"
	sdkworker "go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.temporal.io/server/chasm"
	"go.temporal.io/server/chasm/lib/flowcontrol/concurrency"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/testing/await"
	"go.temporal.io/server/common/testing/testhooks"
	"go.temporal.io/server/service/history/tasks"
	"go.temporal.io/server/service/matching/fc"
	"go.temporal.io/server/tests/testcore"
)

type FlowControlSuite struct {
	xdcBaseSuite
}

func TestFlowControlSuite(t *testing.T) {
	t.Parallel()

	s := &FlowControlSuite{}
	s.enableTransitionHistory = true
	suite.Run(t, s)
}

func (s *FlowControlSuite) SetupSuite() {
	s.dynamicConfigOverrides = map[dynamicconfig.Key]any{
		// One partition keeps the limiter key and the dispatch path predictable.
		dynamicconfig.MatchingNumTaskqueueReadPartitions.Key():  1,
		dynamicconfig.MatchingNumTaskqueueWritePartitions.Key(): 1,
	}
	s.setupSuite()
}

func (s *FlowControlSuite) SetupTest() {
	s.setupTest()
}

func (s *FlowControlSuite) TearDownSuite() {
	s.tearDownSuite()
}

// TestLimiterSlotReleasedAfterFailover covers the case the release protocol exists for: the active
// cluster generates a release task for a committed flow control slot but fails over before running
// it. The new active cluster has to have generated its own copy of that task while it was standby,
// otherwise the slot stays committed forever and permanently shrinks the task queue's concurrency
// limit.
//
// The lost release is simulated with a task interceptor that drops ReleaseLimiterTask on the
// cluster that starts out active, which -- from the limiter's point of view -- is exactly what a
// failover between generating and running the task looks like.
func (s *FlowControlSuite) TestLimiterSlotReleasedAfterFailover() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := s.createGlobalNamespace()
	nsResp, err := s.clusters[0].FrontendClient().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: ns,
	})
	s.NoError(err)
	nsID := nsResp.GetNamespaceInfo().GetId()

	taskQueue := testcore.RandomizeStr("tq")

	// Drop every release on cluster 0, as if it had failed over before running the task.
	var droppedReleases atomic.Int32
	s.clusters[0].InjectHook(s.T(), testhooks.NewHook(
		testhooks.HistoryTransferTaskInterceptor,
		func(task tasks.Task, execute func()) {
			if _, ok := task.(*tasks.ReleaseLimiterTask); !ok {
				execute()
				return
			}
			droppedReleases.Add(1)
		},
	), namespace.ID(nsID))

	// Limit the activity task queue to a single concurrent task, so a leaked slot means the
	// queue can never dispatch an activity again.
	_, err = s.clusters[0].FrontendClient().UpdateTaskQueueConfig(ctx, &workflowservice.UpdateTaskQueueConfigRequest{
		Namespace:     ns,
		TaskQueue:     taskQueue,
		TaskQueueType: enumspb.TASK_QUEUE_TYPE_ACTIVITY,
		UpdateQueueConcurrencyLimit: &workflowservice.UpdateTaskQueueConfigRequest_ConcurrencyLimitUpdate{
			ConcurrencyLimit: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1},
			Reason:           "xdc functional test",
		},
	})
	s.NoError(err)
	await.Require(ctx, s.T(), func(t *await.T) {
		describeResp, err := s.clusters[0].FrontendClient().DescribeTaskQueue(ctx, &workflowservice.DescribeTaskQueueRequest{
			Namespace:     ns,
			TaskQueue:     &taskqueuepb.TaskQueue{Name: taskQueue},
			TaskQueueType: enumspb.TASK_QUEUE_TYPE_ACTIVITY,
			ReportConfig:  true,
		})
		require.NoError(t, err)
		require.NotNil(t, describeResp.GetConfig().GetQueueConcurrencyLimit().GetConcurrencyLimit())
	}, 10*time.Second, 100*time.Millisecond)

	activityStarted := make(chan struct{})
	releaseActivity := make(chan struct{})
	activityFn := func(ctx context.Context) error {
		close(activityStarted)
		select {
		case <-releaseActivity:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	workflowFn := func(ctx workflow.Context) error {
		ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 2 * time.Minute,
		})
		return workflow.ExecuteActivity(ctx, activityFn).Get(ctx, nil)
	}

	activeClient, err := sdkclient.Dial(sdkclient.Options{
		HostPort:  s.clusters[0].Host().FrontendGRPCAddress(),
		Namespace: ns,
		Logger:    log.NewSdkLogger(s.logger),
	})
	s.NoError(err)
	defer activeClient.Close()

	// Eager activity dispatch hands the activity straight back in the workflow task response,
	// which never goes through matching and so never takes a flow control slot.
	worker := sdkworker.New(activeClient, taskQueue, sdkworker.Options{
		DisableEagerActivities: true,
	})
	worker.RegisterWorkflow(workflowFn)
	worker.RegisterActivity(activityFn)
	s.NoError(worker.Start())
	defer worker.Stop()

	run, err := activeClient.ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
		ID:        testcore.RandomizeStr("wf-" + s.T().Name()),
		TaskQueue: taskQueue,
	}, workflowFn)
	s.NoError(err)

	select {
	case <-activityStarted:
	case <-ctx.Done():
		s.FailNow("activity never started")
	}

	// Dispatching the activity took the task queue's only slot.
	await.Require(ctx, s.T(), func(t *await.T) {
		slots, err := s.committedSlots(ctx, 0, nsID, taskQueue)
		require.NoError(t, err)
		require.Len(t, slots, 1)
	}, 10*time.Second, 200*time.Millisecond)

	close(releaseActivity)
	s.NoError(run.Get(ctx, nil))
	worker.Stop()

	// The activity is done and its release task was dropped, so both clusters should still see
	// the slot committed: cluster 0 because nothing released it, cluster 1 because that's what
	// replicated to it.
	await.Require(ctx, s.T(), func(t *await.T) {
		require.Positive(t, droppedReleases.Load())
		activeSlots, err := s.committedSlots(ctx, 0, nsID, taskQueue)
		require.NoError(t, err)
		require.Len(t, activeSlots, 1)
		standbySlots, err := s.committedSlots(ctx, 1, nsID, taskQueue)
		require.NoError(t, err)
		require.Equal(t, activeSlots, standbySlots)
	}, replicationWaitTime, replicationCheckInterval)

	s.failover(ns, 0, s.clusters[1].ClusterName(), 2)

	// Cluster 1 generated its own copy of the release task while it was standby, and now that
	// it is active it should run it and free the slot.
	await.Require(ctx, s.T(), func(t *await.T) {
		slots, err := s.committedSlots(ctx, 1, nsID, taskQueue)
		require.NoError(t, err)
		require.Empty(t, slots)
	}, 2*time.Minute, time.Second)
}

// committedSlots returns the slot ids that the given cluster's copy of the task queue's
// whole-queue activity limiter still holds.
func (s *FlowControlSuite) committedSlots(
	ctx context.Context,
	clusterIdx int,
	nsID string,
	taskQueue string,
) ([]string, error) {
	chasmCtx, err := s.clusters[clusterIdx].Host().ChasmContext(ctx)
	if err != nil {
		return nil, err
	}

	return chasm.ReadComponent(
		chasmCtx,
		chasm.NewComponentRef[*concurrency.Component](chasm.ExecutionKey{
			NamespaceID: nsID,
			BusinessID:  fc.WholeQueueLimiterName(taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY),
		}),
		func(c *concurrency.Component, _ chasm.Context, _ any) ([]string, error) {
			committed := []string{}
			for _, slot := range c.GetSlots() {
				if slot.GetCommitted() {
					committed = append(committed, slot.GetSlotId())
				}
			}
			return committed, nil
		},
		nil,
	)
}

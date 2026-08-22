package xdc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	matchingservice "go.temporal.io/server/api/matchingservice/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/chasm"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/testing/testhooks"
	historytasks "go.temporal.io/server/service/history/tasks"
	"google.golang.org/protobuf/types/known/durationpb"
)

type FlowControlFailoverSuite struct {
	xdcBaseSuite
}

func TestFlowControlFailoverSuite(t *testing.T) {
	t.Parallel()

	s := &FlowControlFailoverSuite{}
	s.enableTransitionHistory = true
	suite.Run(t, s)
}

func (s *FlowControlFailoverSuite) SetupSuite() {
	s.dynamicConfigOverrides = map[dynamicconfig.Key]any{
		dynamicconfig.MatchingNumTaskqueueReadPartitions.Key():  5,
		dynamicconfig.MatchingNumTaskqueueWritePartitions.Key(): 5,
	}
	s.setupSuite()
}

func (s *FlowControlFailoverSuite) SetupTest() {
	s.setupTest()
}

func (s *FlowControlFailoverSuite) TearDownSuite() {
	s.tearDownSuite()
}

func (s *FlowControlFailoverSuite) TestWorkflowTaskSlotReleasedAfterFailover() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ns := s.createGlobalNamespace()
	nsInfo := s.describeNamespace(s.T(), s.clusters[0], ns, true).GetNamespaceInfo()
	taskQueue := "flow-control-failover-" + uuid.NewString()
	taskQueueProto := &taskqueuepb.TaskQueue{Name: taskQueue, Kind: enumspb.TASK_QUEUE_KIND_NORMAL}
	s.setConcurrencyLimit(ctx, ns, nsInfo.GetId(), taskQueue, enumspb.TASK_QUEUE_TYPE_WORKFLOW)

	startWorkflow := func(workflowID string) {
		_, err := s.clusters[0].FrontendClient().StartWorkflowExecution(ctx, &workflowservice.StartWorkflowExecutionRequest{
			Namespace:           ns,
			WorkflowId:          workflowID,
			RequestId:           uuid.NewString(),
			WorkflowType:        &commonpb.WorkflowType{Name: "flow-control-failover"},
			TaskQueue:           taskQueueProto,
			WorkflowRunTimeout:  durationpb.New(time.Minute),
			WorkflowTaskTimeout: durationpb.New(time.Minute),
			Identity:            "flow-control-failover-test",
		})
		s.Require().NoError(err)
	}
	pollWorkflowTask := func(pollCtx context.Context, clusterIndex int) (*workflowservice.PollWorkflowTaskQueueResponse, error) {
		return s.clusters[clusterIndex].FrontendClient().PollWorkflowTaskQueue(pollCtx, &workflowservice.PollWorkflowTaskQueueRequest{
			Namespace: ns,
			TaskQueue: taskQueueProto,
			Identity:  "flow-control-failover-test",
		})
	}

	firstWorkflowID := "slot-holder-" + uuid.NewString()
	startWorkflow(firstWorkflowID)
	firstTask, err := pollWorkflowTask(ctx, 0)
	s.Require().NoError(err)
	s.Require().Equal(firstWorkflowID, firstTask.GetWorkflowExecution().GetWorkflowId())

	secondWorkflowID := "blocked-" + uuid.NewString()
	startWorkflow(secondWorkflowID)
	blockedPollCtx, blockedPollCancel := context.WithTimeout(ctx, 2*time.Second)
	blockedTask, _ := pollWorkflowTask(blockedPollCtx, 0)
	blockedPollCancel()
	s.Require().Empty(blockedTask.GetTaskToken())

	// Ensure the Commit has reached the standby before dropping only the later Release transition.
	s.waitForClusterSynced()

	limiterKey := fmt.Sprintf("/_sys/wholequeue/%s/%d/0", taskQueue, enumspb.TASK_QUEUE_TYPE_WORKFLOW)
	releaseReplicationDropped := make(chan struct{}, 1)
	cleanupReplicationHook := s.clusters[1].InjectHook(s.T(), testhooks.NewHook(
		testhooks.HistoryReplicationTaskInterceptor,
		func(task *replicationspb.ReplicationTask, execute func() error) error {
			if replicationTaskWorkflowID(task) != limiterKey {
				return execute()
			}
			select {
			case releaseReplicationDropped <- struct{}{}:
			default:
			}
			return nil
		},
	), testhooks.GlobalScope)
	defer cleanupReplicationHook()

	releaseTaskExecuted := make(chan struct{}, 1)
	cleanupHook := s.clusters[0].InjectHook(s.T(), testhooks.NewHook(
		testhooks.HistoryTransferTaskInterceptor,
		func(task historytasks.Task, execute func()) {
			if _, ok := task.(*historytasks.ReleaseLimiterTask); !ok {
				execute()
				return
			}
			execute()
			select {
			case releaseTaskExecuted <- struct{}{}:
			default:
			}
		},
	), namespace.ID(nsInfo.GetId()))
	defer cleanupHook()

	_, err = s.clusters[0].FrontendClient().TerminateWorkflowExecution(ctx, &workflowservice.TerminateWorkflowExecutionRequest{
		Namespace: ns,
		WorkflowExecution: &commonpb.WorkflowExecution{
			WorkflowId: firstWorkflowID,
		},
		Reason: "failover before limiter release",
	})
	s.Require().NoError(err)

	select {
	case <-releaseTaskExecuted:
	case <-ctx.Done():
		s.FailNow("timed out waiting for active ReleaseLimiterTask")
	}
	select {
	case <-releaseReplicationDropped:
	case <-ctx.Done():
		s.FailNow("timed out waiting for limiter Release replication")
	}

	s.failover(ns, 0, s.clusters[1].ClusterName(), 2)

	secondTask, err := pollWorkflowTask(ctx, 1)
	s.Require().NoError(err)
	s.Require().Equal(secondWorkflowID, secondTask.GetWorkflowExecution().GetWorkflowId())
}

func (s *FlowControlFailoverSuite) TestStandaloneActivitySlotReleasedAfterFailover() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ns := s.createGlobalNamespace()
	nsInfo := s.describeNamespace(s.T(), s.clusters[0], ns, true).GetNamespaceInfo()
	taskQueue := "activity-flow-control-failover-" + uuid.NewString()
	taskQueueProto := &taskqueuepb.TaskQueue{Name: taskQueue, Kind: enumspb.TASK_QUEUE_KIND_NORMAL}
	s.setConcurrencyLimit(ctx, ns, nsInfo.GetId(), taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY)

	startActivity := func(activityID string) {
		_, err := s.clusters[0].FrontendClient().StartActivityExecution(ctx, &workflowservice.StartActivityExecutionRequest{
			Namespace:           ns,
			ActivityId:          activityID,
			ActivityType:        &commonpb.ActivityType{Name: "flow-control-failover"},
			TaskQueue:           taskQueueProto,
			StartToCloseTimeout: durationpb.New(time.Minute),
			Identity:            "flow-control-failover-test",
		})
		s.Require().NoError(err)
	}
	pollActivityTask := func(pollCtx context.Context, clusterIndex int) (*workflowservice.PollActivityTaskQueueResponse, error) {
		return s.clusters[clusterIndex].FrontendClient().PollActivityTaskQueue(pollCtx, &workflowservice.PollActivityTaskQueueRequest{
			Namespace: ns,
			TaskQueue: taskQueueProto,
			Identity:  "flow-control-failover-test",
		})
	}

	firstActivityID := "slot-holder-" + uuid.NewString()
	startActivity(firstActivityID)
	firstTask, err := pollActivityTask(ctx, 0)
	s.Require().NoError(err)
	s.Require().Equal(firstActivityID, firstTask.GetActivityId())

	secondActivityID := "blocked-" + uuid.NewString()
	startActivity(secondActivityID)
	blockedPollCtx, blockedPollCancel := context.WithTimeout(ctx, 2*time.Second)
	blockedTask, _ := pollActivityTask(blockedPollCtx, 0)
	blockedPollCancel()
	s.Require().Empty(blockedTask.GetTaskToken())

	s.waitForClusterSynced()

	releaseLimiterTaskTypeID := chasm.GenerateTypeID(chasm.FullyQualifiedName("activity", "releaseLimiter"))
	releaseTaskSkipped := make(chan struct{}, 1)
	cleanupHook := s.clusters[0].InjectHook(s.T(), testhooks.NewHook(
		testhooks.HistoryTransferTaskInterceptor,
		func(task historytasks.Task, execute func()) {
			chasmTask, ok := task.(*historytasks.ChasmTask)
			if !ok || chasmTask.Info.GetTypeId() != releaseLimiterTaskTypeID {
				execute()
				return
			}
			select {
			case releaseTaskSkipped <- struct{}{}:
			default:
			}
		},
	), namespace.ID(nsInfo.GetId()))
	defer cleanupHook()

	_, err = s.clusters[0].FrontendClient().TerminateActivityExecution(ctx, &workflowservice.TerminateActivityExecutionRequest{
		Namespace:  ns,
		ActivityId: firstActivityID,
		Reason:     "failover before limiter release",
	})
	s.Require().NoError(err)

	select {
	case <-releaseTaskSkipped:
	case <-ctx.Done():
		s.FailNow("timed out waiting for active activity ReleaseLimiterTask")
	}

	s.failover(ns, 0, s.clusters[1].ClusterName(), 2)

	secondTask, err := pollActivityTask(ctx, 1)
	s.Require().NoError(err)
	s.Require().Equal(secondActivityID, secondTask.GetActivityId())
}

func (s *FlowControlFailoverSuite) setConcurrencyLimit(
	ctx context.Context,
	ns string,
	nsID string,
	taskQueue string,
	taskQueueType enumspb.TaskQueueType,
) {
	_, err := s.clusters[0].FrontendClient().UpdateTaskQueueConfig(ctx, &workflowservice.UpdateTaskQueueConfigRequest{
		Namespace:     ns,
		TaskQueue:     taskQueue,
		TaskQueueType: taskQueueType,
		UpdateQueueConcurrencyLimit: &workflowservice.UpdateTaskQueueConfigRequest_ConcurrencyLimitUpdate{
			ConcurrencyLimit: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: 1},
			Reason:           "failover test",
		},
	})
	s.Require().NoError(err)

	s.Require().Eventually(func() bool {
		response, err := s.clusters[1].MatchingClient().GetTaskQueueUserData(ctx, &matchingservice.GetTaskQueueUserDataRequest{
			NamespaceId:   nsID,
			TaskQueue:     taskQueue,
			TaskQueueType: taskQueueType,
		})
		return err == nil && response.GetUserData().GetData().GetPerType()[int32(taskQueueType)].
			GetConfig().GetQueueConcurrencyLimit().GetConcurrencyLimit().GetConcurrentTasks() == 1
	}, replicationWaitTime, replicationCheckInterval)
}

func replicationTaskWorkflowID(task *replicationspb.ReplicationTask) string {
	if attributes := task.GetSyncVersionedTransitionTaskAttributes(); attributes != nil {
		return attributes.GetWorkflowId()
	}
	if attributes := task.GetVerifyVersionedTransitionTaskAttributes(); attributes != nil {
		return attributes.GetWorkflowId()
	}
	if attributes := task.GetHistoryTaskAttributes(); attributes != nil {
		return attributes.GetWorkflowId()
	}
	return ""
}

package tests

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	commandpb "go.temporal.io/api/command/v1"
	enumspb "go.temporal.io/api/enums/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	sdkactivity "go.temporal.io/sdk/activity"
	sdkclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	chasmactivity "go.temporal.io/server/chasm/lib/activity"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/testing/parallelsuite"
	"go.temporal.io/server/common/testing/taskpoller"
	"go.temporal.io/server/common/testing/testvars"
	"go.temporal.io/server/tests/testcore"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	flowControlConcurrencyLimit = int32(5)
	flowControlTaskCount        = 50
	flowControlWorkerCount      = 10
)

type flowControlTestSuite struct {
	parallelsuite.Suite[*flowControlTestSuite]
}

func TestFlowControlTestSuite(t *testing.T) {
	parallelsuite.Run(t, &flowControlTestSuite{})
}

type concurrencyTracker struct {
	running atomic.Int32
	maximum atomic.Int32
	done    sync.WaitGroup
}

func newConcurrencyTracker(taskCount int) *concurrencyTracker {
	t := &concurrencyTracker{}
	t.done.Add(taskCount)
	return t
}

func (t *concurrencyTracker) execute() error {
	running := t.running.Add(1)
	// set t.maximum to max of running
	for maximum := t.maximum.Load(); running > maximum && !t.maximum.CompareAndSwap(maximum, running); maximum = t.maximum.Load() {
	}
	defer t.running.Add(-1)
	defer t.done.Done()

	// take some time to simulate an activity
	time.Sleep(time.Duration(200+rand.IntN(800)) * time.Millisecond) //nolint:forbidigo

	// return an error 20% of the time to check failure path
	if rand.IntN(5) == 0 {
		return errors.New("unlucky")
	}
	return nil
}

func (t *concurrencyTracker) wait(timeout time.Duration) bool {
	return common.AwaitWaitGroup(&t.done, timeout)
}

func (s *flowControlTestSuite) TestWorkflowTaskConcurrencyLimit() {
	env := testcore.NewEnv(s.T(),
		// force more partitions for better coverage
		testcore.WithDynamicConfig(dynamicconfig.MatchingNumTaskqueueReadPartitions, 5),
		testcore.WithDynamicConfig(dynamicconfig.MatchingNumTaskqueueWritePartitions, 5),
	)
	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_WORKFLOW)

	tracker := newConcurrencyTracker(flowControlTaskCount)
	workflowType := tv.WorkflowType().GetName()
	workflowFn := func(workflow.Context) error {
		if err := tracker.execute(); err != nil {
			panic(err) // simulate workflow task failure
		}
		return nil
	}

	for i := range flowControlTaskCount {
		_, err := env.SdkClient().ExecuteWorkflow(s.Context(), sdkclient.StartWorkflowOptions{
			ID:        fmt.Sprintf("%s-%d", tv.WorkflowID(), i),
			TaskQueue: taskQueue,
		}, workflowType)
		s.Require().NoError(err)
	}

	workers := make([]worker.Worker, 0, flowControlWorkerCount)
	for range flowControlWorkerCount {
		w := worker.New(env.SdkClient(), taskQueue, worker.Options{
			MaxConcurrentWorkflowTaskExecutionSize: 2,
		})
		w.RegisterWorkflowWithOptions(workflowFn, workflow.RegisterOptions{Name: workflowType})
		s.Require().NoError(w.Start())
		workers = append(workers, w)
	}
	defer stopWorkers(workers)

	s.waitAndVerifyConcurrency(tracker)
}

func (s *flowControlTestSuite) TestActivityTaskConcurrencyLimit() {
	env := testcore.NewEnv(s.T(),
		// force more partitions for better coverage
		testcore.WithDynamicConfig(dynamicconfig.MatchingNumTaskqueueReadPartitions, 5),
		testcore.WithDynamicConfig(dynamicconfig.MatchingNumTaskqueueWritePartitions, 5),
		// default pending activities limit is very low, raise it
		testcore.WithDynamicConfig(dynamicconfig.NumPendingActivitiesLimitError, flowControlTaskCount+1),
	)
	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY)

	_, err := env.SdkClient().ExecuteWorkflow(s.Context(), sdkclient.StartWorkflowOptions{
		ID:        tv.WorkflowID(),
		TaskQueue: taskQueue,
	}, tv.WorkflowType().GetName())
	s.Require().NoError(err)

	_, err = env.TaskPoller().PollAndHandleWorkflowTask(
		tv,
		func(*workflowservice.PollWorkflowTaskQueueResponse) (*workflowservice.RespondWorkflowTaskCompletedRequest, error) {
			commands := make([]*commandpb.Command, 0, flowControlTaskCount)
			for i := range flowControlTaskCount {
				commands = append(commands, &commandpb.Command{
					CommandType: enumspb.COMMAND_TYPE_SCHEDULE_ACTIVITY_TASK,
					Attributes: &commandpb.Command_ScheduleActivityTaskCommandAttributes{
						ScheduleActivityTaskCommandAttributes: &commandpb.ScheduleActivityTaskCommandAttributes{
							ActivityId:             fmt.Sprintf("activity-%d", i),
							ActivityType:           tv.ActivityType(),
							TaskQueue:              tv.TaskQueue(),
							ScheduleToCloseTimeout: durationpb.New(time.Minute),
							StartToCloseTimeout:    durationpb.New(time.Minute),
						},
					},
				})
			}
			return &workflowservice.RespondWorkflowTaskCompletedRequest{Commands: commands}, nil
		},
		taskpoller.WithContext(s.Context()),
	)
	s.Require().NoError(err)

	tracker := newConcurrencyTracker(flowControlTaskCount)
	activityType := tv.ActivityType().GetName()
	activityFn := func(context.Context) error {
		return tracker.execute()
	}
	workers := s.startActivityWorkers(env, taskQueue, activityType, activityFn)
	defer stopWorkers(workers)

	s.waitAndVerifyConcurrency(tracker)
}

func (s *flowControlTestSuite) TestWorkflowCloseReleasesRunningActivitySlots() {
	const firstWorkflowActivityCount = int32(3)

	env := testcore.NewEnv(s.T())
	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY)

	activityType := tv.ActivityType().GetName()
	activitiesStarted := atomic.Int32{}
	unblockActivities := make(chan struct{})
	activityFn := func(context.Context) error {
		activitiesStarted.Add(1)
		<-unblockActivities
		return nil
	}
	workflowType := tv.WorkflowType().GetName()
	workflowFn := func(ctx workflow.Context, activityCount int32) error {
		ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			TaskQueue:              taskQueue,
			ScheduleToCloseTimeout: time.Minute,
			StartToCloseTimeout:    time.Minute,
		})
		for range activityCount {
			workflow.ExecuteActivity(ctx, activityType)
		}
		return workflow.Await(ctx, func() bool { return false })
	}

	workflowWorker := worker.New(env.SdkClient(), taskQueue, worker.Options{LocalActivityWorkerOnly: true})
	workflowWorker.RegisterWorkflowWithOptions(workflowFn, workflow.RegisterOptions{Name: workflowType})
	s.Require().NoError(workflowWorker.Start())
	workers := append([]worker.Worker{workflowWorker}, s.startActivityWorkers(env, taskQueue, activityType, activityFn)...)
	defer stopWorkers(workers)
	defer close(unblockActivities)

	firstRun, err := env.SdkClient().ExecuteWorkflow(s.Context(), sdkclient.StartWorkflowOptions{
		ID:        tv.WithWorkflowIDNumber(1).WorkflowID(),
		TaskQueue: taskQueue,
	}, workflowType, firstWorkflowActivityCount)
	s.Require().NoError(err)
	s.Require().Eventually(
		func() bool { return activitiesStarted.Load() == firstWorkflowActivityCount },
		10*time.Second,
		100*time.Millisecond,
	)

	err = env.SdkClient().TerminateWorkflow(s.Context(), firstRun.GetID(), firstRun.GetRunID(), "test workflow closure")
	s.Require().NoError(err)

	secondRun, err := env.SdkClient().ExecuteWorkflow(s.Context(), sdkclient.StartWorkflowOptions{
		ID:        tv.WithWorkflowIDNumber(2).WorkflowID(),
		TaskQueue: taskQueue,
	}, workflowType, flowControlConcurrencyLimit)
	s.Require().NoError(err)
	s.Require().Eventually(
		func() bool {
			return activitiesStarted.Load() == firstWorkflowActivityCount+flowControlConcurrencyLimit
		},
		10*time.Second,
		100*time.Millisecond,
	)
	s.Require().NoError(env.SdkClient().TerminateWorkflow(
		s.Context(),
		secondRun.GetID(),
		secondRun.GetRunID(),
		"test cleanup",
	))
}

func (s *flowControlTestSuite) TestStandaloneActivityTaskConcurrencyLimit() {
	env := testcore.NewEnv(s.T(),
		// force more partitions for better coverage
		testcore.WithDynamicConfig(dynamicconfig.MatchingNumTaskqueueReadPartitions, 5),
		testcore.WithDynamicConfig(dynamicconfig.MatchingNumTaskqueueWritePartitions, 5),
	)
	nsValues := func(value any) []dynamicconfig.ConstrainedValue {
		return []dynamicconfig.ConstrainedValue{{
			Constraints: dynamicconfig.Constraints{Namespace: env.Namespace().String()},
			Value:       value,
		}}
	}
	cluster := env.GetTestCluster()
	cluster.OverrideDynamicConfig(s.T(), dynamicconfig.EnableChasm, nsValues(true))
	cluster.OverrideDynamicConfig(s.T(), chasmactivity.Enabled, nsValues(true))

	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY)

	for i := range flowControlTaskCount {
		_, err := env.FrontendClient().StartActivityExecution(s.Context(), &workflowservice.StartActivityExecutionRequest{
			Namespace:           env.Namespace().String(),
			ActivityId:          fmt.Sprintf("%s-%d", tv.ActivityID(), i),
			ActivityType:        tv.ActivityType(),
			TaskQueue:           tv.TaskQueue(),
			StartToCloseTimeout: durationpb.New(time.Minute),
		})
		s.Require().NoError(err)
	}

	tracker := newConcurrencyTracker(flowControlTaskCount)
	activityFn := func(context.Context) error {
		return tracker.execute()
	}
	workers := s.startActivityWorkers(env, taskQueue, tv.ActivityType().GetName(), activityFn)
	defer stopWorkers(workers)

	s.waitAndVerifyConcurrency(tracker)
}

func (s *flowControlTestSuite) TestStandaloneActivityTerminateReleasesRunningSlots() {
	const terminatedActivityCount = int32(3)

	env := testcore.NewEnv(s.T())
	nsValues := func(value any) []dynamicconfig.ConstrainedValue {
		return []dynamicconfig.ConstrainedValue{{
			Constraints: dynamicconfig.Constraints{Namespace: env.Namespace().String()},
			Value:       value,
		}}
	}
	cluster := env.GetTestCluster()
	cluster.OverrideDynamicConfig(s.T(), dynamicconfig.EnableChasm, nsValues(true))
	cluster.OverrideDynamicConfig(s.T(), chasmactivity.Enabled, nsValues(true))

	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY)

	activitiesStarted := atomic.Int32{}
	unblockActivities := make(chan struct{})
	activityFn := func(context.Context) error {
		activitiesStarted.Add(1)
		<-unblockActivities
		return nil
	}
	workers := s.startActivityWorkers(env, taskQueue, tv.ActivityType().GetName(), activityFn)
	defer stopWorkers(workers)
	defer close(unblockActivities)

	startActivity := func(activityID string) {
		_, err := env.FrontendClient().StartActivityExecution(s.Context(), &workflowservice.StartActivityExecutionRequest{
			Namespace:           env.Namespace().String(),
			ActivityId:          activityID,
			ActivityType:        tv.ActivityType(),
			TaskQueue:           tv.TaskQueue(),
			StartToCloseTimeout: durationpb.New(time.Minute),
		})
		s.Require().NoError(err)
	}

	for i := range terminatedActivityCount {
		startActivity(fmt.Sprintf("%s-terminated-%d", tv.ActivityID(), i))
	}
	s.Require().Eventually(
		func() bool { return activitiesStarted.Load() == terminatedActivityCount },
		10*time.Second,
		100*time.Millisecond,
	)
	for i := range terminatedActivityCount {
		_, err := env.FrontendClient().TerminateActivityExecution(s.Context(), &workflowservice.TerminateActivityExecutionRequest{
			Namespace:  env.Namespace().String(),
			ActivityId: fmt.Sprintf("%s-terminated-%d", tv.ActivityID(), i),
			Reason:     "test activity termination",
		})
		s.Require().NoError(err)
	}

	for i := range flowControlConcurrencyLimit {
		startActivity(fmt.Sprintf("%s-replacement-%d", tv.ActivityID(), i))
	}
	s.Require().Eventually(
		func() bool {
			return activitiesStarted.Load() == terminatedActivityCount+flowControlConcurrencyLimit
		},
		10*time.Second,
		100*time.Millisecond,
	)
}

func (s *flowControlTestSuite) setConcurrencyLimit(
	env *testcore.TestEnv,
	taskQueue string,
	taskQueueType enumspb.TaskQueueType,
) {
	_, err := env.FrontendClient().UpdateTaskQueueConfig(s.Context(), &workflowservice.UpdateTaskQueueConfigRequest{
		Namespace:     env.Namespace().String(),
		TaskQueue:     taskQueue,
		TaskQueueType: taskQueueType,
		UpdateQueueConcurrencyLimit: &workflowservice.UpdateTaskQueueConfigRequest_ConcurrencyLimitUpdate{
			ConcurrencyLimit: &taskqueuepb.ConcurrencyLimit{ConcurrentTasks: flowControlConcurrencyLimit},
			Reason:           "functional test",
		},
	})
	s.Require().NoError(err)
}

func (s *flowControlTestSuite) startActivityWorkers(
	env *testcore.TestEnv,
	taskQueue string,
	activityType string,
	activityFn func(context.Context) error,
) []worker.Worker {
	workers := make([]worker.Worker, 0, flowControlWorkerCount)
	for range flowControlWorkerCount {
		w := worker.New(env.SdkClient(), taskQueue, worker.Options{
			MaxConcurrentActivityExecutionSize: 1,
			DisableWorkflowWorker:              true,
		})
		w.RegisterActivityWithOptions(activityFn, sdkactivity.RegisterOptions{Name: activityType})
		s.Require().NoError(w.Start())
		workers = append(workers, w)
	}
	return workers
}

func (s *flowControlTestSuite) waitAndVerifyConcurrency(tracker *concurrencyTracker) {
	s.Require().True(tracker.wait(30 * time.Second))
	s.Require().Equal(flowControlConcurrencyLimit, tracker.maximum.Load())
}

func stopWorkers(workers []worker.Worker) {
	for _, w := range workers {
		w.Stop()
	}
}

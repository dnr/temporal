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
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	sdkactivity "go.temporal.io/sdk/activity"
	sdkclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
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

func (t *concurrencyTracker) run() {
	running := t.running.Add(1)
	// set t.maximum to max of running
	for maximum := t.maximum.Load(); running > maximum && !t.maximum.CompareAndSwap(maximum, running); maximum = t.maximum.Load() {
	}
	defer t.running.Add(-1)

	// take some time to simulate an activity
	time.Sleep(time.Duration(200+rand.IntN(800)) * time.Millisecond) //nolint:forbidigo
}

func (t *concurrencyTracker) execute() {
	t.run()
	t.done.Done()
}

func (t *concurrencyTracker) wait(timeout time.Duration) bool {
	return common.AwaitWaitGroup(&t.done, timeout)
}

func newRetryingActivity(
	tracker *concurrencyTracker,
	retryCount *atomic.Int32,
) func(context.Context) error {
	return func(ctx context.Context) error {
		tracker.run()
		if sdkactivity.GetInfo(ctx).Attempt == 1 && rand.IntN(2) == 0 {
			retryCount.Add(1)
			return errors.New("retry activity")
		}
		tracker.done.Done()
		return nil
	}
}

func (s *flowControlTestSuite) TestWorkflowTaskConcurrencyLimit() {
	env := s.newFlowControlEnv()
	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_WORKFLOW)

	tracker := newConcurrencyTracker(flowControlTaskCount)
	workflowType := tv.WorkflowType().GetName()
	workflowFn := func(workflow.Context) error {
		tracker.execute()
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

func (s *flowControlTestSuite) TestWorkflowCloseReleasesRunningWorkflowTaskSlots() {
	closeCount := int32(1 + rand.IntN(int(flowControlConcurrencyLimit)))

	env := s.newFlowControlEnv()
	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_WORKFLOW)

	startWorkflow := func(workflowID string) {
		_, err := env.SdkClient().ExecuteWorkflow(s.Context(), sdkclient.StartWorkflowOptions{
			ID:                  workflowID,
			TaskQueue:           taskQueue,
			WorkflowTaskTimeout: time.Minute,
		}, tv.WorkflowType().GetName())
		s.Require().NoError(err)
	}
	pollWorkflowTask := func() *workflowservice.PollWorkflowTaskQueueResponse {
		ctx, cancel := context.WithTimeout(s.Context(), 10*time.Second)
		defer cancel()
		response, err := env.FrontendClient().PollWorkflowTaskQueue(ctx, &workflowservice.PollWorkflowTaskQueueRequest{
			Namespace: env.Namespace().String(),
			TaskQueue: tv.TaskQueue(),
			Identity:  tv.WorkerIdentity(),
		})
		s.Require().NoError(err)
		return response
	}

	for i := range flowControlConcurrencyLimit {
		startWorkflow(fmt.Sprintf("%s-running-%d", tv.WorkflowID(), i))
	}
	for range flowControlConcurrencyLimit {
		pollWorkflowTask()
	}

	for i := range closeCount {
		err := env.SdkClient().TerminateWorkflow(
			s.Context(),
			fmt.Sprintf("%s-running-%d", tv.WorkflowID(), i),
			"",
			"test workflow closure",
		)
		s.Require().NoError(err)
		startWorkflow(fmt.Sprintf("%s-replacement-%d", tv.WorkflowID(), i))
	}
	for range closeCount {
		response := pollWorkflowTask()
		s.Require().Contains(response.GetWorkflowExecution().GetWorkflowId(), "-replacement-")
	}
}

func (s *flowControlTestSuite) TestActivityTaskConcurrencyLimit() {
	env := s.newFlowControlEnv(
		// default pending activities limit is very low, raise it
		testcore.WithDynamicConfig(dynamicconfig.NumPendingActivitiesLimitError, flowControlTaskCount+1),
	)
	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY)
	s.scheduleWorkflowActivities(env, tv, flowControlTaskCount, time.Minute, nil)

	tracker := newConcurrencyTracker(flowControlTaskCount)
	activityType := tv.ActivityType().GetName()
	activityFn := func(context.Context) error {
		tracker.execute()
		return nil
	}
	workers := s.startActivityWorkers(env, taskQueue, activityType, activityFn)
	defer stopWorkers(workers)

	s.waitAndVerifyConcurrency(tracker)
}

func (s *flowControlTestSuite) TestActivityTimeoutReleasesSlots() {
	const activityCount = int(flowControlConcurrencyLimit * 2)

	env := s.newFlowControlEnv()
	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY)
	s.scheduleWorkflowActivities(env, tv, activityCount, time.Second, nil)

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

	s.Require().Eventually(
		func() bool { return activitiesStarted.Load() == int32(activityCount) },
		10*time.Second,
		100*time.Millisecond,
	)
}

func (s *flowControlTestSuite) TestActivityRetriesReleaseAttemptSlots() {
	env := s.newFlowControlEnv(
		testcore.WithDynamicConfig(dynamicconfig.NumPendingActivitiesLimitError, flowControlTaskCount+1),
	)
	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY)
	s.scheduleWorkflowActivities(env, tv, flowControlTaskCount, time.Minute, s.retryPolicy())

	tracker := newConcurrencyTracker(flowControlTaskCount)
	retryCount := atomic.Int32{}
	workers := s.startActivityWorkers(
		env,
		taskQueue,
		tv.ActivityType().GetName(),
		newRetryingActivity(tracker, &retryCount),
	)
	defer stopWorkers(workers)

	s.waitAndVerifyConcurrency(tracker)
	s.Require().Positive(retryCount.Load())
}

func (s *flowControlTestSuite) TestWorkflowCloseReleasesRunningActivitySlots() {
	const firstWorkflowActivityCount = int32(3)

	env := s.newFlowControlEnv()
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
	env := s.newFlowControlEnv()

	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY)
	s.startStandaloneActivities(env, tv, flowControlTaskCount, time.Minute, nil)

	tracker := newConcurrencyTracker(flowControlTaskCount)
	activityFn := func(context.Context) error {
		tracker.execute()
		return nil
	}
	workers := s.startActivityWorkers(env, taskQueue, tv.ActivityType().GetName(), activityFn)
	defer stopWorkers(workers)

	s.waitAndVerifyConcurrency(tracker)
}

func (s *flowControlTestSuite) TestStandaloneActivityTimeoutReleasesSlots() {
	const activityCount = int(flowControlConcurrencyLimit * 2)

	env := s.newFlowControlEnv()
	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY)
	s.startStandaloneActivities(env, tv, activityCount, time.Second, nil)

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

	s.Require().Eventually(
		func() bool { return activitiesStarted.Load() == int32(activityCount) },
		10*time.Second,
		100*time.Millisecond,
	)
}

func (s *flowControlTestSuite) TestStandaloneActivityRetriesReleaseAttemptSlots() {
	env := s.newFlowControlEnv()
	tv := testvars.New(s.T())
	taskQueue := tv.TaskQueue().GetName()
	s.setConcurrencyLimit(env, taskQueue, enumspb.TASK_QUEUE_TYPE_ACTIVITY)
	s.startStandaloneActivities(env, tv, flowControlTaskCount, time.Minute, s.retryPolicy())

	tracker := newConcurrencyTracker(flowControlTaskCount)
	retryCount := atomic.Int32{}
	workers := s.startActivityWorkers(
		env,
		taskQueue,
		tv.ActivityType().GetName(),
		newRetryingActivity(tracker, &retryCount),
	)
	defer stopWorkers(workers)

	s.waitAndVerifyConcurrency(tracker)
	s.Require().Positive(retryCount.Load())
}

func (s *flowControlTestSuite) TestStandaloneActivityTerminateReleasesRunningSlots() {
	const terminatedActivityCount = int32(3)
	env := s.newFlowControlEnv()

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

func (s *flowControlTestSuite) newFlowControlEnv(
	opts ...testcore.TestOption,
) *testcore.TestEnv {
	opts = append([]testcore.TestOption{
		testcore.WithDynamicConfig(dynamicconfig.MatchingNumTaskqueueReadPartitions, 5),
		testcore.WithDynamicConfig(dynamicconfig.MatchingNumTaskqueueWritePartitions, 5),
	}, opts...)
	return testcore.NewEnv(s.T(), opts...)
}

func (s *flowControlTestSuite) scheduleWorkflowActivities(
	env *testcore.TestEnv,
	tv *testvars.TestVars,
	activityCount int,
	startToCloseTimeout time.Duration,
	retryPolicy *commonpb.RetryPolicy,
) {
	_, err := env.SdkClient().ExecuteWorkflow(s.Context(), sdkclient.StartWorkflowOptions{
		ID:        tv.WorkflowID(),
		TaskQueue: tv.TaskQueue().GetName(),
	}, tv.WorkflowType().GetName())
	s.Require().NoError(err)

	_, err = env.TaskPoller().PollAndHandleWorkflowTask(
		tv,
		func(*workflowservice.PollWorkflowTaskQueueResponse) (*workflowservice.RespondWorkflowTaskCompletedRequest, error) {
			commands := make([]*commandpb.Command, 0, activityCount)
			for i := range activityCount {
				commands = append(commands, &commandpb.Command{
					CommandType: enumspb.COMMAND_TYPE_SCHEDULE_ACTIVITY_TASK,
					Attributes: &commandpb.Command_ScheduleActivityTaskCommandAttributes{
						ScheduleActivityTaskCommandAttributes: &commandpb.ScheduleActivityTaskCommandAttributes{
							ActivityId:             fmt.Sprintf("%s-%d", tv.ActivityID(), i),
							ActivityType:           tv.ActivityType(),
							TaskQueue:              tv.TaskQueue(),
							ScheduleToCloseTimeout: durationpb.New(time.Minute),
							StartToCloseTimeout:    durationpb.New(startToCloseTimeout),
							RetryPolicy:            retryPolicy,
						},
					},
				})
			}
			return &workflowservice.RespondWorkflowTaskCompletedRequest{Commands: commands}, nil
		},
		taskpoller.WithContext(s.Context()),
	)
	s.Require().NoError(err)
}

func (s *flowControlTestSuite) startStandaloneActivities(
	env *testcore.TestEnv,
	tv *testvars.TestVars,
	activityCount int,
	startToCloseTimeout time.Duration,
	retryPolicy *commonpb.RetryPolicy,
) {
	for i := range activityCount {
		_, err := env.FrontendClient().StartActivityExecution(s.Context(), &workflowservice.StartActivityExecutionRequest{
			Namespace:           env.Namespace().String(),
			ActivityId:          fmt.Sprintf("%s-%d", tv.ActivityID(), i),
			ActivityType:        tv.ActivityType(),
			TaskQueue:           tv.TaskQueue(),
			StartToCloseTimeout: durationpb.New(startToCloseTimeout),
			RetryPolicy:         retryPolicy,
		})
		s.Require().NoError(err)
	}
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

func (*flowControlTestSuite) retryPolicy() *commonpb.RetryPolicy {
	return &commonpb.RetryPolicy{
		InitialInterval:    durationpb.New(100 * time.Millisecond),
		BackoffCoefficient: 1,
		MaximumInterval:    durationpb.New(100 * time.Millisecond),
		MaximumAttempts:    2,
	}
}

func stopWorkers(workers []worker.Worker) {
	for _, w := range workers {
		w.Stop()
	}
}

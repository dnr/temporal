// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package scheduler

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/maps"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	schedpb "go.temporal.io/api/schedule/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	sdklog "go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	sschedpb "go.temporal.io/server/api/schedule/v1"
	"go.temporal.io/server/common/payload"
	"go.temporal.io/server/common/primitives/timestamp"
)

const (
	// The number of future action times to include in Describe.
	futureActionCount = 10
	// The number of recent actual action results to include in Describe.
	recentActionCount = 10

	// TODO: replace with event count or hint from server
	iterationsBeforeContinueAsNew = 500

	defaultCatchupWindow = 60 * time.Second
	minCatchupWindow     = 10 * time.Second

	searchAttrStartTime    = "TemporalScheduledStartTime"
	searchAttrScheduleById = "TemporalScheduledById"
)

type (
	scheduler struct {
		sschedpb.StartScheduleArgs

		ctx    workflow.Context
		a      *activities
		logger sdklog.Logger

		cspec *compiledSpec

		// watcherFuture != nil iff watcher activity is running
		watcherFuture workflow.Future
	}
)

var (
	defaultActivityRetryPolicy = &temporal.RetryPolicy{
		InitialInterval: 1 * time.Second,
		MaximumInterval: 30 * time.Second,
	}

	errUpdateConflict = errors.New("Conflicting concurrent update")
	errInternal       = errors.New("Internal logic error")
)

func SchedulerWorkflow(ctx workflow.Context, args *sschedpb.StartScheduleArgs) error {
	id := workflow.GetInfo(ctx).WorkflowExecution.ID
	scheduler := &scheduler{
		StartScheduleArgs: *args,
		ctx:               ctx,
		a:                 nil,
		logger:            sdklog.With(workflow.GetLogger(ctx), "schedule-id", id),
	}
	return scheduler.run()
}

func (s *scheduler) run() error {
	s.logger.Info("Schedule starting", "schedule", s.Schedule)

	s.ensureFields()
	s.compileSpec()

	if err := workflow.SetQueryHandler(s.ctx, "describe", s.describe); err != nil {
		return err
	}

	if s.State.LastProcessedTime == nil {
		s.logger.Debug("Initializing internal state")
		s.State.LastProcessedTime = timestamp.TimePtr(s.now())
		s.Info = &schedpb.ScheduleInfo{
			CreateTime: s.State.LastProcessedTime,
		}
		s.incSeqNo()
	}

	// Recreate watcher activity if needed
	if s.State.WatcherRequest != nil {
		s.startWatcher()
	}

	// A schedule may be created with an initial Patch, e.g. start one immediately. Handle that now.
	s.processPatch(s.InitialPatch)
	s.InitialPatch = nil

	for iters := iterationsBeforeContinueAsNew; iters >= 0; iters-- {
		t1 := timestamp.TimeValue(s.State.LastProcessedTime)
		t2 := s.now()
		if t2.Before(t1) {
			// Time went backwards. Currently this can only happen across a continue-as-new boundary.
			s.logger.Warn("Time went backwards", "from", t1, "to", t2)
			t2 = t1
		}
		nextSleep, hasNext := s.processTimeRange(
			t1, t2,
			// resolve this to the schedule's policy as late as possible
			enumspb.SCHEDULE_OVERLAP_POLICY_UNSPECIFIED,
			false,
		)
		s.State.LastProcessedTime = timestamp.TimePtr(t2)
		for s.processBuffer() {
		}
		// sleep returns on any of:
		// 1. requested time elapsed
		// 2. we got a signal (an update or request)
		// 3. a workflow that we created finished
		s.sleep(nextSleep, hasNext)
	}

	// The watcher activity will get cancelled automatically if running.

	s.logger.Info("Schedule doing continue-as-new")
	return workflow.NewContinueAsNewError(s.ctx, WorkflowName, &s.StartScheduleArgs)
}

func (s *scheduler) ensureFields() {
	if s.Schedule == nil {
		s.Schedule = &schedpb.Schedule{}
	}
	if s.Schedule.Spec == nil {
		s.Schedule.Spec = &schedpb.ScheduleSpec{}
	}
	if s.Schedule.Action == nil {
		s.Schedule.Action = &schedpb.ScheduleAction{}
	}
	if s.Schedule.Policies == nil {
		s.Schedule.Policies = &schedpb.SchedulePolicies{}
	}
	if s.Schedule.State == nil {
		s.Schedule.State = &schedpb.ScheduleState{}
	}
	if s.Info == nil {
		s.Info = &schedpb.ScheduleInfo{}
	}
	if s.State == nil {
		s.State = &sschedpb.InternalState{}
	}
}

func (s *scheduler) compileSpec() {
	cspec, err := newCompiledSpec(s.Schedule.Spec)
	if err != nil {
		s.logger.Error("Invalid schedule", "error", err)
		s.Info.InvalidScheduleError = err.Error()
		s.cspec = nil
	} else {
		s.Info.InvalidScheduleError = ""
		s.cspec = cspec
	}
}

func (s *scheduler) now() time.Time {
	// Notes:
	// 1. The time returned here is actually the timestamp of the WorkflowTaskStarted
	// event, which is generated in history, not any time on the worker itself.
	// 2. There will be some delay between when history stamps the time on the
	// WorkflowTaskStarted event and when this code runs, as the event+task goes through
	// matching and frontend. But it should be well under a second, which is our minimum
	// granularity anyway.
	// 3. It's actually the maximum of all of those events: the go sdk enforces that
	// workflow time is monotonic. So if the clock on a history node is wrong and workflow
	// time is ahead of real time for a while, and then goes back, scheduled jobs will run
	// ahead of time, and then nothing will happen until real time catches up to where it
	// was temporarily ahead. Currently the only way to "recover" from this situation is
	// to recreate the schedule/scheduler workflow.
	// 4. Actually there is one way time can appear to go backwards from the point of view
	// of this workflow: across a continue-as-new, since monotonicity isn't preserved
	// there (as far as I know). We'll treat that the same since we keep track of the last
	// processed schedule time.
	return workflow.Now(s.ctx)
}

func (s *scheduler) processPatch(patch *schedpb.SchedulePatch) {
	s.logger.Debug("processPatch", "patch", patch)

	if patch == nil {
		return
	}

	if trigger := patch.TriggerImmediately; trigger != nil {
		now := s.now()
		s.addStart(now, now, trigger.OverlapPolicy, true)
	}

	for _, bfr := range patch.BackfillRequest {
		s.processTimeRange(
			timestamp.TimeValue(bfr.GetStartTime()),
			timestamp.TimeValue(bfr.GetEndTime()),
			bfr.GetOverlapPolicy(),
			true,
		)
	}

	if patch.Pause != "" {
		s.Schedule.State.Paused = true
		s.Schedule.State.Notes = patch.Pause
		s.incSeqNo()
	}
	if patch.Unpause != "" {
		s.Schedule.State.Paused = false
		s.Schedule.State.Notes = patch.Unpause
		s.incSeqNo()
	}
}

func (s *scheduler) processTimeRange(
	t1, t2 time.Time,
	overlapPolicy enumspb.ScheduleOverlapPolicy,
	manual bool,
) (nextSleep time.Duration, hasNext bool) {
	s.logger.Debug("processTimeRange", "t1", t1, "t2", t2, "overlapPolicy", overlapPolicy, "manual", manual)

	if s.cspec == nil {
		return 0, false
	}

	catchupWindow := s.getCatchupWindow()

	for {
		nominalTime, nextTime, hasNext := s.cspec.getNextTime(s.Schedule.State, t1)
		t1 = nextTime
		if !hasNext {
			return 0, false
		} else if nextTime.After(t2) {
			return nextTime.Sub(t2), true
		}
		if !manual && t2.Sub(nextTime) > catchupWindow {
			s.logger.Warn("Schedule missed catchup window", "now", t2, "time", nextTime)
			s.Info.MissedCatchupWindow++
			continue
		}
		// Peek at paused/remaining actions state and don't even bother adding
		// to buffer if we're not going to take an action now.
		if s.canTakeScheduledAction(manual, false) {
			s.addStart(nominalTime, nextTime, overlapPolicy, manual)
		}
	}
	return 0, false
}

func (s *scheduler) canTakeScheduledAction(manual, decrement bool) bool {
	// If manual (trigger immediately or backfill), always allow
	if manual {
		return true
	}
	// If paused, don't do anything
	if s.Schedule.State.Paused {
		return false
	}
	// If unlimited actions, allow
	if !s.Schedule.State.LimitedActions {
		return true
	}
	// Otherwise check and decrement limit
	if s.Schedule.State.RemainingActions > 0 {
		if decrement {
			s.Schedule.State.RemainingActions--
			s.incSeqNo()
		}
		return true
	}
	// No actions left
	return false
}

func (s *scheduler) sleep(nextSleep time.Duration, hasNext bool) {
	sel := workflow.NewSelector(s.ctx)

	upCh := workflow.GetSignalChannel(s.ctx, "update")
	sel.AddReceive(upCh, s.handleUpdateSignal)

	reqCh := workflow.GetSignalChannel(s.ctx, "patch")
	sel.AddReceive(reqCh, s.handlePatchSignal)

	if hasNext {
		tmr := workflow.NewTimer(s.ctx, nextSleep)
		sel.AddFuture(tmr, func(_ workflow.Future) {})
	}

	if s.watcherFuture != nil {
		sel.AddFuture(s.watcherFuture, s.wfWatcherReturned)
	}

	s.logger.Debug("sleeping", "hasNext", hasNext, "watcher", s.watcherFuture != nil)
	sel.Select(s.ctx)
	for sel.HasPending() {
		sel.Select(s.ctx)
	}
}

func (s *scheduler) wfWatcherReturned(f workflow.Future) {
	// workflow is not running anymore
	s.watcherFuture = nil
	s.State.WatcherRequest = nil

	var res sschedpb.WatchWorkflowResponse
	err := f.Get(s.ctx, &res)
	if err != nil {
		// shouldn't happen since this is a select callback
		s.logger.Error("Error from workflow watcher future", "error", err)
		return
	}

	// handle pause-on-failure
	failedStatus := res.Status == enumspb.WORKFLOW_EXECUTION_STATUS_FAILED || res.Status == enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT
	pauseOnFailure := s.Schedule.Policies.PauseOnFailure && failedStatus && !s.Schedule.State.Paused
	if pauseOnFailure {
		s.Schedule.State.Paused = true
		if res.Status == enumspb.WORKFLOW_EXECUTION_STATUS_FAILED {
			s.Schedule.State.Notes = fmt.Sprintf("paused due to workflow failure (%s)", res.GetFailure().GetMessage())
			s.logger.Info("paused due to workflow failure", "message", res.GetFailure().GetMessage())
		} else if res.Status == enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT {
			s.Schedule.State.Notes = fmt.Sprintf("paused due to workflow timeout")
			s.logger.Info("paused due to workflow timeout")
		}
		s.incSeqNo()
	}

	// handle last completion/failure
	if res.GetResult() != nil {
		s.State.LastCompletionResult = res.GetResult()
		s.State.ContinuedFailure = nil
	} else if res.GetFailure() != nil {
		// leave LastCompletionResult from previous run
		s.State.ContinuedFailure = res.GetFailure()
	}

	s.logger.Info("started workflow finished", "status", res.Status, "pause-after-failure", pauseOnFailure)
}

func (s *scheduler) handleUpdateSignal(ch workflow.ReceiveChannel, _ bool) {
	var req sschedpb.FullUpdateRequest
	ch.Receive(s.ctx, &req)
	if err := s.checkConflict(req.ConflictToken); err != nil {
		s.logger.Warn("Update conflicted with concurrent change")
		return
	}

	s.logger.Info("Schedule update", "new-schedule", req.Schedule.String())

	s.Schedule.Spec = req.Schedule.Spec
	s.Schedule.Action = req.Schedule.Action
	s.Schedule.Policies = req.Schedule.Policies
	s.Schedule.State = req.Schedule.State
	// don't touch Info

	s.ensureFields()
	s.compileSpec()

	s.Info.UpdateTime = timestamp.TimePtr(s.now())
	s.incSeqNo()
}

func (s *scheduler) handlePatchSignal(ch workflow.ReceiveChannel, _ bool) {
	var patch schedpb.SchedulePatch
	ch.Receive(s.ctx, &patch)
	s.logger.Info("Schedule patch", "patch", patch.String())
	s.processPatch(&patch)
}

func (s *scheduler) describe() (*sschedpb.DescribeResponse, error) {
	// update future actions
	if s.cspec != nil {
		s.Info.FutureActionTimes = make([]*time.Time, 0, futureActionCount)
		t1 := timestamp.TimeValue(s.State.LastProcessedTime)
		for len(s.Info.FutureActionTimes) < futureActionCount {
			_, t1, has := s.cspec.getNextTime(s.Schedule.State, t1)
			if !has {
				break
			}
			s.Info.FutureActionTimes = append(s.Info.FutureActionTimes, timestamp.TimePtr(t1))
		}
	} else {
		s.Info.FutureActionTimes = nil
	}

	return &sschedpb.DescribeResponse{
		Schedule: s.Schedule,
		Info:     s.Info,
	}, nil
}

func (s *scheduler) incSeqNo() {
	if len(s.State.ConflictToken) != 8 {
		s.State.ConflictToken = make([]byte, 8)
	}
	v := binary.BigEndian.Uint64(s.State.ConflictToken)
	binary.BigEndian.PutUint64(s.State.ConflictToken, v+1)
}

func (s *scheduler) checkConflict(token []byte) error {
	if token == nil || bytes.Equal(token, s.State.ConflictToken) {
		return nil
	}
	return errUpdateConflict
}

func (s *scheduler) getCatchupWindow() time.Duration {
	// Use MutableSideEffect so that we can change the defaults without breaking determinism.
	get := func(ctx workflow.Context) interface{} {
		cw := s.Schedule.Policies.CatchupWindow
		if cw == nil {
			return defaultCatchupWindow
		} else if *cw < minCatchupWindow {
			return minCatchupWindow
		} else {
			return *cw
		}
	}
	eq := func(a, b interface{}) bool {
		return a.(time.Duration) == b.(time.Duration)
	}
	var cw time.Duration
	if workflow.MutableSideEffect(s.ctx, "getCatchupWindow", get, eq).Get(&cw) != nil {
		return defaultCatchupWindow
	}
	return cw
}

func (s *scheduler) resolveOverlapPolicy(overlapPolicy enumspb.ScheduleOverlapPolicy) enumspb.ScheduleOverlapPolicy {
	if overlapPolicy == enumspb.SCHEDULE_OVERLAP_POLICY_UNSPECIFIED {
		overlapPolicy = s.Schedule.Policies.OverlapPolicy
	}
	if overlapPolicy == enumspb.SCHEDULE_OVERLAP_POLICY_UNSPECIFIED {
		overlapPolicy = enumspb.SCHEDULE_OVERLAP_POLICY_SKIP
	}
	return overlapPolicy
}

func (s *scheduler) addStart(nominalTime, actualTime time.Time, overlapPolicy enumspb.ScheduleOverlapPolicy, manual bool) {
	s.logger.Debug("addStart", "nominal", nominalTime, "actual", actualTime, "overlapPolicy", overlapPolicy, "manual", manual)
	s.State.BufferedStarts = append(s.State.BufferedStarts, &sschedpb.BufferedStart{
		NominalTime:   timestamp.TimePtr(nominalTime),
		ActualTime:    timestamp.TimePtr(actualTime),
		OverlapPolicy: overlapPolicy,
		Manual:        manual,
	})
}

// processBuffer should return true if there might be more work to do right now.
func (s *scheduler) processBuffer() bool {
	s.logger.Debug("processBuffer")

	// We should try to do something reasonable with any combination of overlap
	// policies in the buffer, although some combinations don't make much sense
	// and would require a convoluted series of calls to set up.
	//
	// Buffer entries that have unspecified overlap policy are resolved to the
	// current policy here, not earlier, so that updates to the policy can
	// affect them.

	// Make sure we have something to start. If not, we can clear the buffer.
	// This part will have to change when we support more actions.
	req := s.Schedule.Action.GetStartWorkflow()
	if req == nil {
		s.State.BufferedStarts = nil
		s.logger.Debug("processBuffer: no action")
		return false
	}

	// We might or might not have one running now. If the watcher activity is still running, we
	// probably have one running. Even if the workflow has exited and our notification from the
	// watcher activity is delayed, acting as if it's still running is correct, we'll just
	// buffer the action and run it when we get the notification.
	isRunning := s.watcherFuture != nil

	action := processBuffer(s.State.BufferedStarts, isRunning, s.resolveOverlapPolicy)

	s.State.BufferedStarts = action.newBuffer
	s.Info.OverlapSkipped += action.overlapSkipped

	// Handle all ALLOW_ALL first
	for _, start := range action.overlappingStarts {
		if !s.canTakeScheduledAction(start.Manual, true) {
			continue
		}
		result, err := s.startWorkflow(start, req)
		if err != nil {
			s.logger.Error("Failed to start workflow", "error", err)
			// The "start workflow" activity has an unlimited retry policy, so if we get an
			// error here, it must be an unretryable one. Drop this from the buffer.
			continue
		}
		s.recordAction(result)
	}

	if action.needTerminate {
		s.terminateWorkflow(s.State.WatcherRequest.Execution)
	} else if action.needCancel {
		s.cancelWorkflow(s.State.WatcherRequest.Execution)
	}

	if action.nonOverlappingStart == nil {
		return false
	}

	// Check paused/remaining actions again, and decrement this time.
	if !s.canTakeScheduledAction(action.nonOverlappingStart.Manual, true) {
		return true
	}
	result, err := s.startWorkflowAndWatch(action.nonOverlappingStart, req)
	if err != nil {
		s.logger.Error("Failed to start workflow", "error", err)
		// The "start workflow" activity has an unlimited retry policy, so if we get an
		// error here, it must be an unretryable one. Drop this from the buffer.
		return true
	}
	s.recordAction(result)
	return false
}

func (s *scheduler) recordAction(result *schedpb.ScheduleActionResult) {
	s.Info.ActionCount++
	s.Info.RecentActions = append(s.Info.RecentActions, result)
	extra := len(s.Info.RecentActions) - 10
	if extra > 0 {
		s.Info.RecentActions = s.Info.RecentActions[extra:]
	}
}

func (s *scheduler) startWorkflow(
	start *sschedpb.BufferedStart,
	origStartReq *workflowpb.NewWorkflowExecutionInfo,
) (*schedpb.ScheduleActionResult, error) {
	sreq := &workflowservice.StartWorkflowExecutionRequest{	Namespace:"asdf",
	}
	// sreq.WorkflowId = sreq.WorkflowId + "-" + start.NominalTime.UTC().Format(time.RFC3339)
	// sreq.Identity = s.identity()
	// sreq.RequestId = uuid.NewString()
	// sreq.SearchAttributes = s.addSearchAttr(sreq.SearchAttributes, start.NominalTime.UTC())

	// FIXME: need to set NonRetryableErrorTypes
	ctx := workflow.WithActivityOptions(s.ctx, workflow.ActivityOptions{RetryPolicy: defaultActivityRetryPolicy})
	req := &sschedpb.StartWorkflowRequest{
		NamespaceId: s.State.NamespaceId,
		Request:     &sreq,
		StartTime:   start.ActualTime, // used to set expiration time, so use actual instead of nominal
	}
	var res sschedpb.StartWorkflowResponse
	err := workflow.ExecuteActivity(ctx, s.a.StartWorkflow, req).Get(s.ctx, &res)
	if err != nil {
		return nil, err
	}

	return &schedpb.ScheduleActionResult{
		ScheduleTime: start.ActualTime,
		ActualTime:   res.RealStartTime,
		StartWorkflowResult: &commonpb.WorkflowExecution{
			WorkflowId: sreq.WorkflowId,
			RunId:      res.RunId,
		},
	}, nil
}

func (s *scheduler) startWorkflowAndWatch(
	start *sschedpb.BufferedStart,
	origStartReq *workflowpb.NewWorkflowExecutionInfo,
) (*schedpb.ScheduleActionResult, error) {
	// Watcher should not be running now
	if s.State.WatcherRequest != nil || s.watcherFuture != nil {
		return nil, errInternal
	}

	result, err := s.startWorkflow(start, origStartReq)
	if err != nil {
		return nil, err
	}

	// Start background activity to watch the workflow
	s.State.WatcherRequest = &sschedpb.WatchWorkflowRequest{
		Execution: result.StartWorkflowResult,
	}
	s.startWatcher()

	return result, nil
}

func (s *scheduler) identity() string {
	info := workflow.GetInfo(s.ctx)
	return fmt.Sprintf("temporal-scheduler-%s-%s", s.State.Namespace, info.WorkflowExecution.ID)
}

func (s *scheduler) addSearchAttr(
	attrs *commonpb.SearchAttributes,
	nominal time.Time,
) *commonpb.SearchAttributes {
	fields := maps.Clone(attrs.GetIndexedFields())
	if p, err := payload.Encode(nominal); err == nil {
		fields[searchAttrStartTime] = p
	}
	info := workflow.GetInfo(s.ctx)
	if p, err := payload.Encode(info.WorkflowExecution.ID); err == nil {
		fields[searchAttrScheduleById] = p
	}
	return &commonpb.SearchAttributes{
		IndexedFields: fields,
	}
}

func (s *scheduler) startWatcher() {
	s.logger.Debug("starting wf watcher", s.State.WatcherRequest)
	ctx := workflow.WithActivityOptions(s.ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 365 * 24 * time.Hour,
		// FIXME: need to set NonRetryableErrorTypes
		RetryPolicy: defaultActivityRetryPolicy,
		// see activities.tryWatchWorkflow for this timeout
		HeartbeatTimeout: 65 * time.Second,
	})
	s.watcherFuture = workflow.ExecuteActivity(ctx, s.a.WatchWorkflow, s.State.WatcherRequest)
}

func (s *scheduler) cancelWorkflow(execution *commonpb.WorkflowExecution) {
	ctx := workflow.WithActivityOptions(s.ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 1 * time.Minute,
		// FIXME: need to set NonRetryableErrorTypes
		RetryPolicy: defaultActivityRetryPolicy,
	})
	areq := &sschedpb.CancelWorkflowRequest{
		NamespaceId: s.State.NamespaceId,
		Namespace:   s.State.Namespace,
		RequestId:   uuid.NewString(),
		Identity:    s.identity(),
		Execution:   execution,
	}
	workflow.ExecuteActivity(ctx, s.a.CancelWorkflow, areq)
	// do not wait for cancel to complete
}

func (s *scheduler) terminateWorkflow(execution *commonpb.WorkflowExecution) {
	ctx := workflow.WithActivityOptions(s.ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 1 * time.Minute,
		// FIXME: need to set NonRetryableErrorTypes
		RetryPolicy: defaultActivityRetryPolicy,
	})
	areq := &sschedpb.TerminateWorkflowRequest{
		NamespaceId: s.State.NamespaceId,
		Namespace:   s.State.Namespace,
		Identity:    s.identity(),
		Execution:   execution,
		Reason:      "terminated by schedule overlap policy",
	}
	workflow.ExecuteActivity(ctx, s.a.TerminateWorkflow, areq)
	// do not wait for terminate to complete
}

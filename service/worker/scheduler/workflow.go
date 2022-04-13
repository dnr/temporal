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
	failurepb "go.temporal.io/api/failure/v1"
	schedpb "go.temporal.io/api/schedule/v1"
	"go.temporal.io/api/workflowservice/v1"
	sdklog "go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/payload"
	"go.temporal.io/server/common/primitives/timestamp"
)

const (
	WorkflowName = "temporal-sys-scheduler-workflow"

	// The number of future action times to include in Describe.
	futureActionCount = 10
	// The number of recent actual action results to include in Describe.
	recentActionCount = 10

	// TODO: can we estimate event count and do it that way?
	iterationsBeforeContinueAsNew = 500

	defaultCatchupWindow = 60 * time.Second
	minCatchupWindow     = 10 * time.Second

	searchAttrStartTimeKey = "TemporalScheduledStartTime"
)

type (
	// FIXME: convert these all to protos?

	bufferedStart struct {
		// Nominal (pre-jitter) and Actual (post-jitter) time of action
		Nominal, Actual time.Time
		// Overridden overlap policy
		Overlap enumspb.ScheduleOverlapPolicy
		// Trigger-immediately or backfill
		Manual bool
	}

	internalState struct {
		Namespace         namespace.Name
		NamespaceID       namespace.ID
		ID                string
		LastProcessedTime time.Time
		BufferedStarts    []*bufferedStart

		// last completion/failure
		LastCompletionResult *commonpb.Payloads
		ContinuedFailure     *failurepb.Failure

		// conflict token is implemented as simple sequence number
		SequenceNumber uint64
		ConflictToken  []byte

		// WatcherReq != nil iff watcher activity is running
		WatcherReq *watchWorkflowRequest
	}

	StartArgs struct {
		Schedule       *schedpb.Schedule
		Info           *schedpb.ScheduleInfo
		InitialRequest *schedpb.ScheduleRequest
		Internal       internalState
	}

	UpdateArgs struct {
		Schedule      *schedpb.Schedule
		ConflictToken []byte
	}

	DescribeResult struct {
		Schedule *schedpb.Schedule
		Info     *schedpb.ScheduleInfo
	}

	scheduler struct {
		StartArgs

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
	errInternal       = errors.New("internal logic error")
)

func SchedulerWorkflow(ctx workflow.Context, args *StartArgs) error {
	scheduler := &scheduler{
		StartArgs: *args,
		ctx:       ctx,
		a:         nil,
		logger:    sdklog.With(workflow.GetLogger(ctx), "schedule-id", args.Internal.ID),
	}
	return scheduler.run()
}

func (s *scheduler) run() error {
	s.logger.Info("Schedule started")

	s.ensureFields()
	s.compileSpec()

	if err := workflow.SetQueryHandler(s.ctx, "describe", s.describe); err != nil {
		return err
	}

	if s.Internal.LastProcessedTime.IsZero() {
		s.logger.Debug("Initializing internal state")
		s.Internal.LastProcessedTime = s.now()
		s.Info = &schedpb.ScheduleInfo{
			CreateTime: timestamp.TimePtr(s.Internal.LastProcessedTime),
		}
		s.incSeqNo()
	}

	// Recreate watcher activity if needed
	if s.Internal.WatcherReq != nil {
		s.startWatcher()
	}

	// A schedule may be created with an initial Request, e.g. start one
	// immediately. Handle that now.
	s.processRequest(s.InitialRequest)
	s.InitialRequest = nil

	for iters := iterationsBeforeContinueAsNew; iters >= 0; iters-- {
		t2 := s.now()
		if t2.Before(s.Internal.LastProcessedTime) {
			// Time went backwards. If this is a large jump, we may want to
			// handle it differently, but for now, do nothing until we get back
			// up to the last processed time.
			s.logger.Warn("Time went backwards", "from", s.Internal.LastProcessedTime, "to", t2)
			t2 = s.Internal.LastProcessedTime
		}
		nextSleep, hasNext := s.processTimeRange(
			s.Internal.LastProcessedTime, t2,
			// resolve this to the schedule's policy as late as possible
			enumspb.SCHEDULE_OVERLAP_POLICY_UNSPECIFIED,
			false,
		)
		s.Internal.LastProcessedTime = t2
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
	return workflow.NewContinueAsNewError(s.ctx, WorkflowName, &s.StartArgs)
}

func (s *scheduler) ensureFields() {
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
	// TODO: potential issues:
	// 1. the go sdk enforces monotonic time. but we might want to do something
	// different if the real time (on the server worker) goes backwards, e.g.
	// due to a large time adjustment. so maybe we could get time.Now in a side
	// effect? I think that'll increase the history size though.
	// 2. how accurate is this? it's the time of the workflowtaskstarted event,
	// which is generated by the history service. actually it's a transient
	// event: it's not written to history until the wft is completed (I think),
	// and it's pasted into the history that the worker sees by the frontend.
	// as far as I can tell, this value is taken from wall time by history at
	// the time when matching tells it that the wft is about to be returned to a
	// worker. the delay from history->matching->frontend->worker is hopefully
	// well under a second, which is all we care about, so it's probably close
	// enough.
	return workflow.Now(s.ctx)
}

func (s *scheduler) processRequest(req *schedpb.ScheduleRequest) {
	if req == nil {
		return
	}

	if trigger := req.TriggerImmediately; trigger != nil {
		now := s.now()
		s.addStart(now, now, trigger.OverlapPolicy, true)
	}

	for _, bfr := range req.BackfillRequest {
		s.processTimeRange(
			timestamp.TimeValue(bfr.GetFromTime()),
			timestamp.TimeValue(bfr.GetToTime()),
			bfr.GetOverlapPolicy(),
			true,
		)
	}

	if req.Pause != "" {
		s.Schedule.State.Paused = true
		s.Schedule.State.Notes = req.Pause
		s.incSeqNo()
	}
	if req.Unpause != "" {
		s.Schedule.State.Paused = false
		s.Schedule.State.Notes = req.Unpause
		s.incSeqNo()
	}
}

func (s *scheduler) processTimeRange(
	t1, t2 time.Time,
	overlapPolicy enumspb.ScheduleOverlapPolicy,
	manual bool,
) (nextSleep time.Duration, hasNext bool) {
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
		s.addStart(nominalTime, nextTime, overlapPolicy, manual)
	}
	return 0, false
}

func (s *scheduler) canTakeScheduledAction(manual bool) bool {
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
		s.Schedule.State.RemainingActions--
		s.incSeqNo()
		return true
	}
	// No actions left
	return false
}

func (s *scheduler) sleep(nextSleep time.Duration, hasNext bool) {
	sel := workflow.NewSelector(s.ctx)

	upCh := workflow.GetSignalChannel(s.ctx, "update")
	sel.AddReceive(upCh, s.update)

	reqCh := workflow.GetSignalChannel(s.ctx, "request")
	sel.AddReceive(reqCh, s.request)

	if hasNext {
		tmr := workflow.NewTimer(s.ctx, nextSleep)
		sel.AddFuture(tmr, func(_ workflow.Future) {})
	}

	if s.watcherFuture != nil {
		sel.AddFuture(s.watcherFuture, s.wfWatcherReturned)
	}

	sel.Select(s.ctx)
	for sel.HasPending() {
		sel.Select(s.ctx)
	}
}

func (s *scheduler) wfWatcherReturned(f workflow.Future) {
	// workflow is not running anymore
	s.watcherFuture = nil
	s.Internal.WatcherReq = nil

	var res watchWorkflowResponse
	err := f.Get(s.ctx, &res)
	if err != nil {
		// shouldn't happen since this is a select callback
		s.logger.Error("Error from workflow watcher future", "error", err)
		return
	}

	// handle pause-on-failure
	pauseAfterFailure := s.Schedule.Policies.PauseAfterFailure && res.Failed && !s.Schedule.State.Paused
	if pauseAfterFailure {
		s.Schedule.State.Paused = true
		s.Schedule.State.Notes = fmt.Sprintf("paused due to failure (%s)", res.WorkflowError)
		s.logger.Info("paused due to failure", "error", res.WorkflowError)
		s.incSeqNo()
	}

	// handle last completion/failure
	if res.CompletionResult != nil {
		s.Internal.LastCompletionResult = res.CompletionResult
		s.Internal.ContinuedFailure = nil
	} else if res.Failure != nil {
		// leave LastCompletionResult from previous run
		s.Internal.ContinuedFailure = res.Failure
	}

	s.logger.Info("started workflow finished",
		"failed", res.Failed, "error", res.WorkflowError, "pause-after-failure", pauseAfterFailure)
}

func (s *scheduler) update(ch workflow.ReceiveChannel, _ bool) {
	var req UpdateArgs
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

func (s *scheduler) request(ch workflow.ReceiveChannel, _ bool) {
	var req schedpb.ScheduleRequest
	ch.Receive(s.ctx, &req)
	s.logger.Info("Schedule request", "request", req.String())
	s.processRequest(&req)
}

func (s *scheduler) describe() (*DescribeResult, error) {
	// update future actions
	if s.cspec != nil {
		s.Info.FutureActions = make([]*time.Time, 0, futureActionCount)
		t1 := s.Internal.LastProcessedTime
		for len(s.Info.FutureActions) < futureActionCount {
			_, t1, has := s.cspec.getNextTime(s.Schedule.State, t1)
			if !has {
				break
			}
			s.Info.FutureActions = append(s.Info.FutureActions, timestamp.TimePtr(t1))
		}
	} else {
		s.Info.FutureActions = nil
	}

	return &DescribeResult{
		Schedule: s.Schedule,
		Info:     s.Info,
	}, nil
}

func (s *scheduler) incSeqNo() {
	s.Internal.SequenceNumber++
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, s.Internal.SequenceNumber)
	s.Internal.ConflictToken = buf
}

func (s *scheduler) checkConflict(token []byte) error {
	if token == nil || bytes.Equal(token, s.Internal.ConflictToken) {
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
	s.Internal.BufferedStarts = append(s.Internal.BufferedStarts, &bufferedStart{
		Nominal: nominalTime,
		Actual:  actualTime,
		Overlap: overlapPolicy,
		Manual:  manual,
	})
}

// processBuffer should return true if there might be more work to do right now.
func (s *scheduler) processBuffer() bool {
	// We should try to do something reasonable with any combination of overlap
	// policies in the buffer, although some combinations don't make much sense
	// and would require a convoluted series of calls to set up.
	//
	// Buffer entries that have unspecified overlap policy are resolved to the
	// current policy here, not earlier, so that updates to the policy can
	// affect them.

	// Make sure we have something to start. If not, we can clear the buffer.
	// TODO: This part will have to change when we support more actions.
	req := s.Schedule.Action.GetStartWorkflow()
	if req == nil {
		s.Internal.BufferedStarts = nil
		return false
	}

	// Just run everything with allow-all since they can't conflict. Filter them
	// out of the buffer at the same time.
	var nextBufferedStarts []*bufferedStart
	for _, start := range s.Internal.BufferedStarts {
		if s.resolveOverlapPolicy(start.Overlap) != enumspb.SCHEDULE_OVERLAP_POLICY_ALLOW_ALL {
			// pass through to loop below
			nextBufferedStarts = append(nextBufferedStarts, start)
			continue
		}

		if !s.canTakeScheduledAction(start.Manual) {
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
	s.Internal.BufferedStarts = nextBufferedStarts

	// We can start either zero or one workflows now (since we handled allow-all above).
	// This is the one that we want to start, or nil.
	var pendingStart *bufferedStart

	// We might or might not have one running now. If the watcher activity is still running, we
	// probably have one running. Even if the workflow has exited and our notification from the
	// watcher activity is delayed, acting as if it's still running is correct, we'll just
	// buffer the action and run it when we get the notification.
	isRunning := s.watcherFuture != nil

	// Recreate the rest of the buffer in here as we're processing
	nextBufferedStarts = nil
	for _, start := range s.Internal.BufferedStarts {
		// If there's nothing running, we can start this one no matter what the policy is
		if !isRunning && pendingStart == nil {
			pendingStart = start
			continue
		}

		// Otherwise this one overlaps and we should apply the policy
		switch s.resolveOverlapPolicy(start.Overlap) {
		case enumspb.SCHEDULE_OVERLAP_POLICY_SKIP:
			// just skip
			s.Info.OverlapSkipped++
		case enumspb.SCHEDULE_OVERLAP_POLICY_BUFFER_ONE:
			// allow one (the first one) in the buffer
			if len(nextBufferedStarts) == 0 {
				nextBufferedStarts = append(nextBufferedStarts, start)
			} else {
				s.Info.OverlapSkipped++
			}
		case enumspb.SCHEDULE_OVERLAP_POLICY_BUFFER_ALL:
			// always add to buffer
			nextBufferedStarts = append(nextBufferedStarts, start)
		case enumspb.SCHEDULE_OVERLAP_POLICY_CANCEL_OTHER:
			if isRunning {
				// an actual workflow is running, cancel it (asynchronously)
				s.cancelWorkflow(req)
				// keep in buffer so it will get started once cancel completes
				nextBufferedStarts = append(nextBufferedStarts, start)
			} else {
				// it's not running yet, it's just the one we were going to
				// start. replace it
				pendingStart = start
			}
		case enumspb.SCHEDULE_OVERLAP_POLICY_TERMINATE_OTHER:
			// we can start immediately using terminate id-reuse-policy
			pendingStart = start
		}
	}
	s.Internal.BufferedStarts = nextBufferedStarts

	if pendingStart == nil {
		return false
	}

	if !s.canTakeScheduledAction(pendingStart.Manual) {
		return true
	}

	result, err := s.startWorkflowAndWatch(pendingStart, req)
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
	start *bufferedStart,
	origStartReq *workflowservice.StartWorkflowExecutionRequest,
) (*schedpb.ScheduleActionResult, error) {
	sreq := *origStartReq
	sreq.WorkflowId = sreq.WorkflowId + "-" + start.Nominal.UTC().Format(time.RFC3339)
	sreq.Identity = s.identity()
	sreq.RequestId = uuid.NewString()
	sreq.SearchAttributes = s.addSearchAttr(sreq.SearchAttributes, start.Nominal.UTC())

	ctx := workflow.WithActivityOptions(s.ctx, workflow.ActivityOptions{RetryPolicy: defaultActivityRetryPolicy})
	req := &startWorkflowRequest{
		NamespaceID:     s.Internal.NamespaceID,
		Request:         &sreq,
		ActualStartTime: start.Actual, // used to set expiration time
	}
	var res startWorkflowResponse
	err := workflow.ExecuteActivity(ctx, s.a.StartWorkflow, req).Get(s.ctx, &res)
	if err != nil {
		return nil, err
	}

	// No watcher needed
	return &schedpb.ScheduleActionResult{
		ScheduleTime: timestamp.TimePtr(start.Actual),
		ActualTime:   timestamp.TimePtr(res.RealTime),
		StartWorkflowResult: &commonpb.WorkflowExecution{
			WorkflowId: sreq.WorkflowId,
			RunId:      res.RunID,
		},
	}, nil
}

func (s *scheduler) startWorkflowAndWatch(
	start *bufferedStart,
	origStartReq *workflowservice.StartWorkflowExecutionRequest,
) (*schedpb.ScheduleActionResult, error) {
	// Watcher should not be running now
	if s.Internal.WatcherReq != nil || s.watcherFuture != nil {
		return nil, errInternal
	}

	result, err := s.startWorkflow(start, origStartReq)
	if err != nil {
		return nil, err
	}

	// Start background activity to watch the workflow
	s.Internal.WatcherReq = &watchWorkflowRequest{
		WorkflowID: sreq.WorkflowId,
		RunID:      res.RunID,
	}
	s.startWatcher()

	return result, nil
}

func (s *scheduler) identity() string {
	return fmt.Sprintf("temporal-scheduler-%s", s.Internal.ID)
}

func (s *scheduler) addSearchAttr(
	attrs *commonpb.SearchAttributes,
	nominal time.Time,
) *commonpb.SearchAttributes {
	out := maps.Clone(attrs.GetIndexedFields())
	if p, err := payload.Encode(nominal); err == nil {
		out[searchAttrStartTimeKey] = p
	}
	return &commonpb.SearchAttributes{
		IndexedFields: out,
	}
}

func (s *scheduler) startWatcher() {
	ctx := workflow.WithActivityOptions(s.ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 365 * 24 * time.Hour,
		RetryPolicy:         defaultActivityRetryPolicy,
		// see activities.tryWatchWorkflow for this timeout
		HeartbeatTimeout: 65 * time.Second,
	})
	s.watcherFuture = workflow.ExecuteActivity(ctx, s.a.WatchWorkflow, s.Internal.WatcherReq)
}

func (s *scheduler) cancelWorkflow(req *workflowservice.StartWorkflowExecutionRequest) {
	ctx := workflow.WithActivityOptions(s.ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 1 * time.Minute,
		RetryPolicy:         defaultActivityRetryPolicy,
	})
	areq := &cancelWorkflowRequest{
		WorkflowID: req.WorkflowId,
	}
	workflow.ExecuteActivity(ctx, s.a.CancelWorkflow, areq)
	// do not wait for cancel to complete
}

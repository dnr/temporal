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
	"fmt"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	schedpb "go.temporal.io/api/schedule/v1"
	"go.temporal.io/api/workflowservice/v1"
	sdklog "go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

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
)

type (
	bufferedStart struct {
		Nominal, Actual time.Time
		Overlap         enumspb.ScheduleOverlapPolicy
	}

	// FIXME: should this be a proto? pass Schedule in here too?
	internalState struct {
		ID                string
		LastProcessedTime time.Time
		BufferedStarts    []*bufferedStart
	}

	scheduler struct {
		ctx    workflow.Context
		a      *activities
		logger sdklog.Logger

		schedpb.Schedule

		internalState

		cspec *compiledSpec

		// Invariant: wfWatcher != nil iff [we think] a workflow is running
		// "we think" because there will be some delay before we're notified
		wfWatcher       workflow.Future
		wfWatcherCancel workflow.CancelFunc

		// FIXME: conflict token
		// FIXME: request id list for deduping
	}
)

var (
	defaultActivityRetryPolicy = &temporal.RetryPolicy{
		InitialInterval: 1 * time.Second,
		MaximumInterval: 30 * time.Second,
	}
)

func SchedulerWorkflow(ctx workflow.Context, sched *schedpb.Schedule, internal *internalState) error {
	scheduler := &scheduler{
		ctx:           ctx,
		a:             nil,
		logger:        sdklog.With(workflow.GetLogger(ctx), "schedule-id", internal.ID),
		Schedule:      *sched,
		internalState: *internal,
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

	if s.LastProcessedTime.IsZero() {
		s.logger.Debug("Initializing processed time")
		s.LastProcessedTime = s.now()
		s.Info.CreateTime = timestamp.TimePtr(s.LastProcessedTime)
	}

	// A schedule may be created with an initial Request, e.g. start one
	// immediately. Handle that now.
	s.processRequest(s.Request)
	s.Request = nil

	for iters := iterationsBeforeContinueAsNew; iters >= 0; iters-- {
		t2 := s.now()
		if t2.Before(s.LastProcessedTime) {
			// Time went backwards. If this is a large jump, we may want to
			// handle it differently, but for now, do nothing until we get back
			// up to the last processed time.
			s.logger.Warn("Time went backwards", "from", s.LastProcessedTime, "to", t2)
			t2 = s.LastProcessedTime
		}
		nextSleep, hasNext := s.processTimeRange(
			s.LastProcessedTime, t2,
			// resolve this to the schedule's policy as late as possible
			enumspb.SCHEDULE_OVERLAP_POLICY_UNSPECIFIED,
			false,
		)
		s.LastProcessedTime = t2
		s.processBuffer()
		// sleep returns on any of:
		// 1. requested time elapsed
		// 2. we got a signal (an update or request)
		// 3. a workflow that we created finished
		s.sleep(nextSleep, hasNext)
	}

	// FIXME: what do we do about our outstanding activity? cancel it?

	s.logger.Info("Schedule doing continue-as-new")
	return workflow.NewContinueAsNewError(s.ctx, WorkflowName, &s.Schedule, &s.internalState)
}

func (s *scheduler) ensureFields() {
	if s.Spec == nil {
		s.Spec = &schedpb.ScheduleSpec{}
	}
	if s.Action == nil {
		s.Action = &schedpb.ScheduleAction{}
	}
	if s.Policies == nil {
		s.Policies = &schedpb.SchedulePolicies{}
	}
	if s.State == nil {
		s.State = &schedpb.ScheduleState{}
	}
	if s.Info == nil {
		s.Info = &schedpb.ScheduleInfo{}
	}
}

func (s *scheduler) compileSpec() {
	cspec, err := newCompiledSpec(s.Spec)
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
		s.action(now, now, trigger.OverlapPolicy)
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
		s.State.Paused = true
		s.State.Notes = req.Pause
	}
	if req.Unpause != "" {
		s.State.Paused = false
		s.State.Notes = req.Unpause
	}
}

func (s *scheduler) processTimeRange(
	t1, t2 time.Time,
	overlapPolicy enumspb.ScheduleOverlapPolicy,
	doingBackfill bool,
) (nextSleep time.Duration, hasNext bool) {
	if s.cspec == nil {
		return 0, false
	}

	catchupWindow := s.getCatchupWindow()

	for {
		// FIXME: consider wrapping in side effect so it can be changed easily?
		nominalTime, nextTime, hasNext := s.cspec.getNextTime(s.State, t1, doingBackfill)
		if !hasNext {
			return 0, false
		} else if nextTime.After(t2) {
			return nextTime.Sub(t2), true
		}
		// FIXME: should this be nextTime or nominalTime? what if someone sets jitter above catchup window?
		if !doingBackfill && t2.Sub(nextTime) > catchupWindow {
			s.logger.Warn("Schedule missed catchup window", "now", t2, "time", nextTime)
			s.Info.MissedCatchupWindow++
			continue
		}
		s.action(nominalTime, nextTime, overlapPolicy)
		t1 = nextTime
	}
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

	if s.wfWatcher != nil {
		sel.AddFuture(s.wfWatcher, s.wfWatcherReturned)
	}

	sel.Select(s.ctx)
	for sel.HasPending() {
		sel.Select(s.ctx)
	}
}

func (s *scheduler) wfWatcherReturned(f workflow.Future) {
	var res watchWorkflowResponse
	err := f.Get(s.ctx, &res)
	if err != nil {
		// shouldn't happen since this is a select callback
		s.logger.Error("Error from workflow watcher future", "error", err)
		return
	}

	// workflow is not running anymore
	s.wfWatcher = nil

	// handle pause-on-failure
	pauseAfterFailure := s.Policies.PauseAfterFailure && res.Failed && !s.State.Paused
	if pauseAfterFailure {
		s.State.Paused = true
		s.State.Notes = fmt.Sprintf("paused due to failure (%s)", res.WorkflowError)
		s.logger.Info("paused due to failure", "error", res.WorkflowError)
		// FIXME: clear buffer here? or let it drain elsewhere?
	}

	s.logger.Info("started workflow finished",
		"failed", res.Failed, "error", res.WorkflowError, "pause-after-failure", pauseAfterFailure)
}

func (s *scheduler) update(ch workflow.ReceiveChannel, _ bool) {
	var newSchedule schedpb.Schedule
	ch.Receive(s.ctx, &newSchedule)

	s.logger.Info("Schedule update", "new-schedule", newSchedule.String())

	s.Spec = newSchedule.Spec
	s.Action = newSchedule.Action
	s.Policies = newSchedule.Policies
	s.State = newSchedule.State
	// don't touch Info or Request

	s.ensureFields()
	s.compileSpec()

	s.Info.UpdateTime = timestamp.TimePtr(s.now())

	// If the update had a request, handle it
	s.processRequest(newSchedule.Request)
}

func (s *scheduler) request(ch workflow.ReceiveChannel, _ bool) {
	var req schedpb.ScheduleRequest
	ch.Receive(s.ctx, &req)
	s.logger.Info("Schedule request", "request", req.String())
	s.processRequest(&req)
}

func (s *scheduler) describe() (*schedpb.Schedule, error) {
	// update future actions
	if s.cspec != nil {
		s.Info.FutureActions = make([]*time.Time, 0, futureActionCount)
		t1 := s.now()
		for len(s.Info.FutureActions) < cap(s.Info.FutureActions) {
			_, t1, has := s.cspec.getNextTime(s.State, t1, false)
			if !has {
				break
			}
			s.Info.FutureActions = append(s.Info.FutureActions, timestamp.TimePtr(t1))
		}
	} else {
		s.Info.FutureActions = nil
	}

	return &s.Schedule, nil
}

func (s *scheduler) getCatchupWindow() time.Duration {
	cw := s.Policies.CatchupWindow
	if cw == nil {
		return 60 * time.Second
	} else if *cw < 10*time.Second {
		return 10 * time.Second
	} else {
		return *cw
	}
}

func (s *scheduler) resolveOverlapPolicy(overlapPolicy enumspb.ScheduleOverlapPolicy) enumspb.ScheduleOverlapPolicy {
	if overlapPolicy == enumspb.SCHEDULE_OVERLAP_POLICY_UNSPECIFIED {
		overlapPolicy = s.Policies.OverlapPolicy
	}
	if overlapPolicy == enumspb.SCHEDULE_OVERLAP_POLICY_UNSPECIFIED {
		overlapPolicy = enumspb.SCHEDULE_OVERLAP_POLICY_SKIP
	}
	return overlapPolicy
}

func (s *scheduler) isRunning() bool {
	return s.wfWatcher != nil
}

func (s *scheduler) action(nominalTime, actualTime time.Time, overlapPolicy enumspb.ScheduleOverlapPolicy) {
	s.BufferedStarts = append(s.BufferedStarts, &bufferedStart{
		Nominal: nominalTime,
		Actual:  actualTime,
		Overlap: overlapPolicy,
	})
}

func (s *scheduler) processBuffer() {
	// We should try to do something reasonable with any combination of overlap
	// policies in the buffer, although some combinations don't make much sense
	// and would require a convoluted series of calls to set up.
	//
	// Buffer entries that have unspecified overlap policy are resolved to the
	// current policy here, not earlier, so that updates to the policy can
	// affect them.

	// Make sure we have something to start. If not, we can clear the buffer.
	// TODO: what about actions that don't start a workflow? if they start
	// activities, they should all be treated as allow all. what if they start
	// both? does it need to be oneof?
	req := s.Action.GetStartWorkflow()
	if req == nil {
		s.BufferedStarts = nil
		return
	}

	// Just run everything with allow all since they can't conflict. Filter them
	// out of the buffer at the same time.
	var nextBufferedStarts []*bufferedStart
	for _, start := range s.BufferedStarts {
		if s.resolveOverlapPolicy(start.Overlap) != enumspb.SCHEDULE_OVERLAP_POLICY_ALLOW_ALL {
			// pass through to loop below
			nextBufferedStarts = append(nextBufferedStarts, start)
		}

		result, err := s.startWorkflowWithAllowAll(start, req)
		if err != nil {
			// failed to start somehow
			// FIXME: log here? we can't just add it to the buffer again
			// because then the loop below will have to deal with it
		} else {
			s.finishAction(result)
		}
	}
	s.BufferedStarts = nextBufferedStarts

	// Maybe we're done
	if len(s.BufferedStarts) == 0 {
		return
	}

	// We can start either zero or one workflows now (since we handled allow all above).
	// This is the one that we want to start, or nil.
	var pendingStart *bufferedStart

	// Recreate the rest of the buffer in here as we're processing
	nextBufferedStarts = nil
	for _, start := range s.BufferedStarts {
		// If there's nothing running, we can start this one no matter what the policy is
		if !s.isRunning() && pendingStart == nil {
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
			if s.isRunning() {
				// an actual workflow is running, cancel it
				s.cancelWorkflow()
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
	s.BufferedStarts = nextBufferedStarts

	if pendingStart == nil {
		return
	}

	result, err := s.startWorkflow(pendingStart, req)
	if err != nil {
		s.logger.Error("Failed to start workflow", "error", err)
		return
	}

	s.finishAction(result)
}

func (s *scheduler) finishAction(result *schedpb.ScheduleActionResult) {
	s.Info.ActionCount++
	s.Info.RecentActions = append(s.Info.RecentActions, result)
	extra := len(s.Info.RecentActions) - 10
	if extra > 0 {
		s.Info.RecentActions = s.Info.RecentActions[extra:]
	}

	if s.State.LimitedActions && s.State.RemainingActions > 0 {
		s.State.RemainingActions--
		// FIXME: what if this drops to zero while we have stuff in the buffer?
		// need to clear it. but only natural scheduled starts, not requested
		// ones... kind of a mess
	}
}

func (s *scheduler) startWorkflow(
	start *bufferedStart,
	req *workflowservice.StartWorkflowExecutionRequest,
) (*schedpb.ScheduleActionResult, error) {
	// Validation
	// namespace: already validated at creation time
	// identity: set by SDK? FIXME
	// request_id: FIXME
	// workflow_id_reuse_policy: FIXME must be omitted or set to WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE.
	// cron_schedule: FIXME

	actCtx1 := workflow.WithActivityOptions(s.ctx, workflow.ActivityOptions{RetryPolicy: defaultActivityRetryPolicy})
	var response startWorkflowResponse
	err := workflow.ExecuteActivity(actCtx1, s.a.StartWorkflow, req).Get(s.ctx, &response)
	if err != nil {
		// FIXME
		return nil, err
	}
	if response.Error != nil {
		// FIXME
		return nil, response.Error
	}

	// Start background activity to watch the workflow
	actCtx2 := workflow.WithActivityOptions(s.ctx, workflow.ActivityOptions{
		RetryPolicy:      defaultActivityRetryPolicy,
		HeartbeatTimeout: 1 * time.Minute,
	})
	actCtx2, s.wfWatcherCancel = workflow.WithCancel(actCtx2)
	s.wfWatcher = workflow.ExecuteActivity(actCtx2, s.a.WatchWorkflow, &watchWorkflowRequest{
		WorkflowID: req.WorkflowId,
		RunID:      response.Response.RunId,
	})

	return &schedpb.ScheduleActionResult{
		ScheduleTime: timestamp.TimePtr(start.Actual),
		ActualTime:   timestamp.TimePtr(response.RealTime),
		StartWorkflowResult: &commonpb.WorkflowExecution{
			WorkflowId: req.WorkflowId,
			RunId:      response.Response.RunId,
		},
	}, nil
}

func (s *scheduler) startWorkflowWithAllowAll(
	start *bufferedStart,
	req *workflowservice.StartWorkflowExecutionRequest,
) (*schedpb.ScheduleActionResult, error) {
	// We append the nominal time to the workflow id
	timeStr := start.Nominal.UTC().Format(time.RFC3339)
	_ = timeStr

	// No watcher needed
	return nil, nil // FIXME
}

func (s *scheduler) cancelWorkflow() {
	// FIXME: start and wait for cancel activity
}

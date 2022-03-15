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
	"context"
	"errors"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	schedpb "go.temporal.io/api/schedule/v1"
	"go.temporal.io/api/workflowservice/v1"
	sdklog "go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/primitives/timestamp"
)

const (
	WorkflowName = "temporal-sys-scheduler-workflow"
)

type (
	activities struct {
		metricsClient metrics.Client
		logger        log.Logger
	}

	scheduler struct {
		ctx    workflow.Context
		id     string
		a      *activities
		logger sdklog.Logger
		schedpb.Schedule
	}
)

func SchedulerWorkflow(ctx workflow.Context, id string, s *schedpb.Schedule) error {
	scheduler := &scheduler{
		ctx:      ctx,
		id:       id,
		a:        nil,
		logger:   sdklog.With(workflow.GetLogger(ctx), "schedule-id", id),
		Schedule: *s,
	}
	return scheduler.run()
}

func (s *scheduler) run() error {
	s.logger.Info("Schedule started")

	s.ensureFields()

	if err := workflow.SetQueryHandler(s.ctx, "describe", s.describe); err != nil {
		return err
	}

	t1 := s.now()
	s.Info.CreateTime = timestamp.TimePtr(t1)

	s.processRequest()

	// FIXME: from dynconfig, or estimate event count
	for iters := 1000; iters >= 0; iters-- {
		t2 := s.now()
		// FIXME: what if time went backwards?
		nextSleep, hasNext := s.processTimeRange(
			t1, t2,
			s.Policies.OverlapPolicy,
			false,
		)
		s.sleep(nextSleep, hasNext)
		t1 = t2
	}

	s.logger.Info("Schedule doing continue-as-new")
	return workflow.NewContinueAsNewError(s.ctx, WorkflowName, s.id, &s.Schedule)
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

func (s *scheduler) now() time.Time {
	return workflow.Now(s.ctx)
}

func (s *scheduler) processRequest() {
	if s.Request == nil {
		return
	}

	if s.Request.TriggerImmediately {
		s.action(s.now(), s.Policies.OverlapPolicy)
	}

	for _, bfr := range s.Request.BackfillRequest {
		s.processTimeRange(
			timestamp.TimeValue(bfr.GetFromTime()),
			timestamp.TimeValue(bfr.GetToTime()),
			bfr.GetOverlapPolicy(),
			true,
		)
	}

	s.Request = nil
}

func (s *scheduler) processTimeRange(
	t1, t2 time.Time,
	overlapPolicy enumspb.ScheduleOverlapPolicy,
	doingBackfill bool,
) (nextSleep time.Duration, hasNext bool) {
	if overlapPolicy == enumspb.SCHEDULE_OVERLAP_POLICY_UNSPECIFIED {
		overlapPolicy = s.Policies.OverlapPolicy
	}
	catchupWindow := s.getCatchupWindow()

	for {
		// FIXME: consider wrapping in side effect so it can be changed easily?
		nominalTime, nextTime, hasNext := getNextTime(s.Spec, s.State, t1, doingBackfill)
		if !hasNext {
			return 0, false
		} else if nextTime.After(t2) {
			return nextTime.Sub(t2), true
		}
		// FIXME: should this be nextTime or nominalTime? what if someone sets jitter above catchup window?
		if !doingBackfill && t2.Sub(nextTime) > catchupWindow {
			s.logger.Warn("Schedule missed catchup window", "now", t2, "nominal-time", nominalTime)
			// FIXME: s.Info.MissedCatchupWindow++
			continue
		}
		s.action(nominalTime, overlapPolicy)
		// FIXME: if pause-after-failure, set up goroutine to watch this one
		t1 = nextTime
	}
}

func (s *scheduler) sleep(nextSleep time.Duration, hasNext bool) {
	sel := workflow.NewSelector(s.ctx)

	sch := workflow.GetSignalChannel(s.ctx, "update")
	sel.AddReceive(sch, s.update)

	if hasNext {
		tmr := workflow.NewTimer(s.ctx, nextSleep)
		sel.AddFuture(tmr, func(_ workflow.Future) {})
	}

	sel.Select(s.ctx)
}

func (s *scheduler) update(ch workflow.ReceiveChannel, _ bool) {
	var newSchedule schedpb.Schedule
	ch.Receive(s.ctx, &newSchedule)

	s.logger.Info("Schedule update", "new-schedule", newSchedule.String())

	// FIXME: any special transition handling here?

	s.Schedule.Spec = newSchedule.Spec
	s.Schedule.Action = newSchedule.Action
	s.Schedule.Policies = newSchedule.Policies
	s.Schedule.State = newSchedule.State
	s.Schedule.Request = newSchedule.Request
	// don't touch Info

	s.ensureFields()

	s.Info.UpdateTime = timestamp.TimePtr(s.now())

	s.processRequest()
}

func (s *scheduler) describe() (*schedpb.Schedule, error) {
	// update future actions
	s.Info.FutureActions = make([]*time.Time, 0, 10)
	t1 := s.now()
	for len(s.Info.FutureActions) < cap(s.Info.FutureActions) {
		nominal, next, has := getNextTime(s.Spec, s.State, t1)
		if !has {
			break
		}
		// FIXME: next or nominal here?
		s.Info.FutureActions = append(s.Info.FutureActions, next)
		t1 = next
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

func (s *scheduler) action(nominalTime time.Time, overlapPolicy enumspb.ScheduleOverlapPolicy) {
	if overlapPolicy == enumspb.SCHEDULE_OVERLAP_POLICY_UNSPECIFIED {
		overlapPolicy = enumspb.SCHEDULE_OVERLAP_POLICY_SKIP
	}
	appendTime := overlapPolicy == enumspb.SCHEDULE_OVERLAP_POLICY_ALLOW_ALL

	if s.Action.GetStartWorkflow() != nil {
		// Validation
		// namespace: already validated at creation time
		// identity: set by SDK? FIXME
		// request_id: FIXME
		// workflow_id_reuse_policy: FIXME must be omitted or set to WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE.
		// cron_schedule: FIXME

		ctx1 := workflow.WithActivityOptions(s.ctx, workflow.ActivityOptions{
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval: 1 * time.Second,
			},
		})
		var result error
		err := workflow.ExecuteActivity(ctx1, s.a.StartWorkflow, req).Get(s.ctx, &result)
		// FIXME: do something with err, result
	}

	s.Info.ActionCount++
	s.Info.RecentActions = append(s.Info.RecentActions, nominalTime)
	extra := len(s.Info.RecentActions) - 10
	if extra > 0 {
		s.Info.RecentActions = s.Info.RecentActions[extra:]
	}

	if s.State.LimitedActions && s.State.RemainingActions > 0 {
		s.State.RemainingActions--
	}
}

func (a *activities) StartWorkflow(ctx context.Context, req *workflowservice.StartWorkflowExecutionRequest) error {
	// send req directly to frontend
	return errors.New("impl")
}

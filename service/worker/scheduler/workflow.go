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
		logger:   workflow.GetLogger(ctx),
		Schedule: *s,
	}
	return scheduler.run()
}

func (s *scheduler) run() error {
	s.logger.Info("Schedule started", "schedule-id", s.id)

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
		s.sleepUntil(
			s.processTimeRange(
				t1, t2,
				s.Policies.OverlapPolicy,
				s.getCatchupWindow(),
			),
		)
		t1 = t2
	}

	s.logger.Info("Schedule doing continue-as-new", "schedule-id", s.id)
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
			-1, // ignore catchup policy
		)
	}

	s.Request = nil
}

func (s *scheduler) processTimeRange(
	t1, t2 time.Time,
	overlapPolicy enumspb.ScheduleOverlapPolicy,
	catchupWindow time.Duration, // -1 means ignore
) (nominalTime, nextTime time.Time, hasNextTime bool) {
	if overlapPolicy == enumspb.SCHEDULE_OVERLAP_POLICY_UNSPECIFIED {
		overlapPolicy = s.Policies.OverlapPolicy
	}

	for {
		nominalTime, nextTime, hasNextTime = getNextTime(s.Spec, s.State, t1)
		if !hasNextTime || nextTime.After(t2) {
			return
		}
		// FIXME: should this be nextTime or nominalTime? what if someone sets
		// jitter above catchup window?
		if catchupWindow >= 0 && t2.Sub(nextTime) > catchupWindow {
			s.logger.Warn("Schedule missed catchup window", "schedule-id", s.id, "now", t2, "nominal-time", nominalTime)
			// FIXME: s.State.MissedCatchupWindow++
			continue
		}
		s.action(nominalTime, overlapPolicy)
		// FIXME: if pause-after-failure, set up goroutine to watch this one
		t1 = nextTime
	}
}

func (s *scheduler) sleepUntil(nominalTime, nextTime time.Time, hasNextTime bool) {
	sel := workflow.NewSelector(s.ctx)

	sch := workflow.GetSignalChannel(s.ctx, "update")
	sel.AddReceive(sch, s.update)

	if hasNextTime {
		// FIXME: use t2 instead of s.now()
		tmr := workflow.NewTimer(s.ctx, s.now().Sub(nextTime))
		sel.AddFuture(tmr, func(_) {})
	}

	sel.Select(s.ctx)
}

func (s *scheduler) update(ch workflow.ReceiveChannel, _ bool) {
	var newSchedule schedpb.Schedule
	ch.Receive(s.ctx, &newSchedule)

	// FIXME: any special transition handling here?

	s.Schedule.Spec = newSchedule.Spec
	s.Schedule.Action = newSchedule.Action
	s.Schedule.Policies = newSchedule.Policies
	s.Schedule.State = newSchedule.State
	s.Schedule.Request = newSchedule.Request
	// not Info

	s.ensureFields()

	s.processRequest()
}

func (s *scheduler) describe() (*schedpb.Schedule, error) {
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

	if s.State.LimitedActions && s.State.RemainingActions > 0 {
		s.State.RemainingActions--
	}
}

func (a *activities) StartWorkflow(ctx context.Context, req *workflowservice.StartWorkflowExecutionRequest) error {
	// send req directly to frontend
	return errors.New("impl")
}

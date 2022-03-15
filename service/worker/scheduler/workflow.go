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
		ctx workflow.Context
		id  string
		a   *activities
		schedpb.Schedule
	}
)

func SchedulerWorkflow(ctx workflow.Context, id string, s *schedpb.Schedule) error {
	scheduler := &scheduler{
		ctx:      ctx,
		id:       id,
		a:        nil,
		Schedule: *s,
	}
	return scheduler.Run()
}

func (s *scheduler) Run() error {
	logger := workflow.GetLogger(s.ctx)
	logger.Info("Schedule started", "wf-type", WorkflowName)

	if s.Policies == nil {
		s.Policies = &schedpb.SchedulePolicies{}
	}
	if s.State == nil {
		s.State = &schedpb.ScheduleState{}
	}
	if s.Info == nil {
		s.Info = &schedpb.ScheduleInfo{}
	}

	t1 := workflow.Now(s.ctx)
	s.Info.CreateTime = timestamp.TimePtr(t1)

	if err := workflow.SetQueryHandler(s.ctx, "describe", s.describe); err != nil {
		return err
	}

	// FIXME: register signals

	// FIXME: from dynconfig, or estimate event count
	for iters := 1000; iters >= 0; iters-- {
		t2 := workflow.Now(s.ctx)
		s.processTimeRange(t1, t2)
		t1 = t2
	}

	return workflow.NewContinueAsNewError(s.ctx, WorkflowName, s.id, &s.Schedule)
}

func (s *scheduler) processTimeRange(t1, t2 time.Time) (nominal, next time.Time, has bool) {
	for {
		nominalTime, nextTime, hasNextTime := getNextTime(s.Spec, s.State, t1)
		if !hasNextTime || nextTime.After(t2) {
			return nominalTime, nextTime, hasNextTime
		}
		s.action(nominalTime)
		t1 = nextTime
	}
}

func (s *scheduler) processTimeRangeAndSleep(t1, t2 time.Time) {
	nominalTime, nextTime, hasNextTime := s.processTimeRange(t1, t2)

	if hasNextTime {
		// sleep until nextTime or signal
	} else {
		// sleep until signal
	}

	if not_manual_trigger {
		// catchups
		now := workflow.Now(s.ctx)
		if now.Sub(nominalTime) > s.getCatchupWindow() {
			// missed catchup!
			// log error
			return
		}
	}

	s.Info.ActionCount++

	if s.State.LimitedActions && s.State.RemainingActions > 0 {
		s.State.RemainingActions--
	}

	// FIXME: if pause-after-failure, set up goroutine to watch this one
}

func (s *scheduler) describe() (*schedpb.Schedule, error) {
	return &s.Schedule, nil
}

func (s *scheduler) getCatchupWindow() time.Duration {
	t := s.Policies.CatchupWindow
	if t == nil {
		return 60 * time.Second
	} else if *t < 10*time.Second {
		return 10 * time.Second
	} else {
		return *t
	}
}

func (s *scheduler) action(nominalTime time.Time) {
	appendTime := s.Policies.OverlapPolicy == enumspb.SCHEDULE_OVERLAP_POLICY_ALLOW_ALL

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
		err := workflow.ExecuteActivity(ctx1, s.a.StartWorkflow, req).Get(ctx, &result)
		// FIXME: do something with err, result
	}
}

func (a *activities) StartWorkflow(ctx context.Context, req *workflowservice.StartWorkflowExecutionRequest) error {
	// send req directly to frontend
	return errors.New("impl")
}

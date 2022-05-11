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
	"reflect"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	failurepb "go.temporal.io/api/failure/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	"go.temporal.io/server/api/historyservice/v1"
	schedspb "go.temporal.io/server/api/schedule/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/primitives/timestamp"
)

type (
	activities struct {
		activityDeps
	}

	errFollow string
)

var (
	errTryAgain = errors.New("try again")
)

func (e errFollow) Error() string { return string(e) }

func (a *activities) StartWorkflow(ctx context.Context, req *schedspb.StartWorkflowRequest) (*schedspb.StartWorkflowResponse, error) {
	request := common.CreateHistoryStartWorkflowRequest(
		req.NamespaceId,
		req.Request,
		nil,
		timestamp.TimeValue(req.StartTime),
	)
	request.LastCompletionResult = req.LastCompletionResult
	request.ContinuedFailure = req.ContinuedFailure

	res, err := a.HistoryClient.StartWorkflowExecution(ctx, request)
	if err != nil {
		if common.IsServiceTransientError(err) {
			return nil, temporal.NewApplicationError(err.Error(), reflect.TypeOf(err).Name())
		}
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), reflect.TypeOf(err).Name(), nil)
	}

	// TODO: ideally, get the time of the workflow execution started event
	// instead of this one, which will be close but not the same
	now := time.Now()

	return &schedspb.StartWorkflowResponse{
		RunId:         res.RunId,
		RealStartTime: timestamp.TimePtr(now),
	}, nil
}

func (a *activities) tryWatchWorkflow(ctx context.Context, req *schedspb.WatchWorkflowRequest) (*schedspb.WatchWorkflowResponse, error) {
	// make sure we return and heartbeat 5s before the timeout
	ctx2, cancel := context.WithTimeout(ctx, activity.GetInfo(ctx).HeartbeatTimeout-5*time.Second)
	defer cancel()

	// poll history service directly instead of just going to frontend to avoid
	// using resources on frontend while waiting.
	// note that on the first time through the loop, Execution.RunId will be
	// empty, so we'll get the latest run, whatever it is (whether it's part of
	// the desired chain or not). if we have to follow (unlikely), we'll end up
	// back here with non-empty RunId.
	pollReq := &historyservice.PollMutableStateRequest{
		NamespaceId: req.NamespaceId,
		Execution:   req.Execution,
	}
	if req.LongPoll {
		pollReq.ExpectedNextEventId = common.EndEventID
	}
	// if long-polling, this will block up for workflow completion to 20s (default) and return
	// the current mutable state at that point
	pollRes, err := a.HistoryClient.PollMutableState(ctx, pollReq)

	// FIXME: separate out retriable vs unretriable errors
	// FIXME: treat not found as not running with unknown status
	if err != nil {
		return nil, err
	}

	makeResponse := func(result *commonpb.Payloads, failure *failurepb.Failure) *schedspb.WatchWorkflowResponse {
		res := &schedspb.WatchWorkflowResponse{Status: pollRes.WorkflowStatus}
		if result != nil {
			res.ResultFailure = &schedspb.WatchWorkflowResponse_Result{Result: result}
		} else if failure != nil {
			res.ResultFailure = &schedspb.WatchWorkflowResponse_Failure{Failure: failure}
		}
		return res
	}

	// TODO: check FirstExecutionRunId to make sure it's the same chain

	if pollRes.WorkflowStatus == enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING {
		if req.LongPoll {
			return nil, errTryAgain // not closed yet, just try again
		}
		return makeResponse(nil, nil), nil
	}

	// get last event from history
	// TODO: could we read from persistence directly or is that too crazy?
	histReq := &workflowservice.GetWorkflowExecutionHistoryRequest{
		Namespace:              req.Namespace,
		Execution:              req.Execution,
		MaximumPageSize:        1,
		HistoryEventFilterType: enumspb.HISTORY_EVENT_FILTER_TYPE_CLOSE_EVENT,
		SkipArchival:           true, // should be recently closed, no need for archival
	}
	histRes, err := a.FrontendClient.GetWorkflowExecutionHistory(ctx2, histReq)

	// FIXME: separate out retriable vs unretriable errors
	if err != nil {
		return nil, err
	}

	events := histRes.GetHistory().GetEvents()
	if len(events) < 1 {
		return nil, errInternal
	}
	lastEvent := events[0]

	switch pollRes.WorkflowStatus {
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		attrs := lastEvent.GetWorkflowExecutionCompletedEventAttributes()
		if attrs == nil {
			return nil, errInternal
		}
		if len(attrs.NewExecutionRunId) > 0 {
			return nil, errFollow(attrs.NewExecutionRunId)
		}
		return makeResponse(attrs.Result, nil), nil
	case enumspb.WORKFLOW_EXECUTION_STATUS_FAILED:
		attrs := lastEvent.GetWorkflowExecutionFailedEventAttributes()
		if attrs == nil {
			return nil, errInternal
		}
		if len(attrs.NewExecutionRunId) > 0 {
			return nil, errFollow(attrs.NewExecutionRunId)
		}
		return makeResponse(nil, attrs.Failure), nil
	case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED:
		attrs := lastEvent.GetWorkflowExecutionCanceledEventAttributes()
		if attrs == nil {
			return nil, errInternal
		}
		return makeResponse(nil, nil), nil
	case enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		attrs := lastEvent.GetWorkflowExecutionTerminatedEventAttributes()
		if attrs == nil {
			return nil, errInternal
		}
		return makeResponse(nil, nil), nil
	case enumspb.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		attrs := lastEvent.GetWorkflowExecutionContinuedAsNewEventAttributes()
		if attrs == nil {
			return nil, errInternal
		}
		return nil, errFollow(attrs.NewExecutionRunId)
	case enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		attrs := lastEvent.GetWorkflowExecutionTimedOutEventAttributes()
		if attrs == nil {
			return nil, errInternal
		}
		if len(attrs.NewExecutionRunId) > 0 {
			return nil, errFollow(attrs.NewExecutionRunId)
		}
		return makeResponse(nil, nil), nil
	}

	return nil, errInternal
}

func (a *activities) WatchWorkflow(ctx context.Context, req *schedspb.WatchWorkflowRequest) (*schedspb.WatchWorkflowResponse, error) {
	for {
		activity.RecordHeartbeat(ctx)
		res, err := a.tryWatchWorkflow(ctx, req)
		if err == errTryAgain || common.IsContextDeadlineExceededErr(err) {
			continue
		}
		if newRunID, ok := err.(errFollow); ok {
			req.Execution.RunId = string(newRunID)
			continue
		}
		return res, err
	}
}

func (a *activities) CancelWorkflow(ctx context.Context, req *schedspb.CancelWorkflowRequest) error {
	rreq := &historyservice.RequestCancelWorkflowExecutionRequest{
		NamespaceId: req.NamespaceId,
		CancelRequest: &workflowservice.RequestCancelWorkflowExecutionRequest{
			Namespace:           req.Namespace,
			WorkflowExecution:   &commonpb.WorkflowExecution{WorkflowId: req.WorkflowId},
			Identity:            req.Identity,
			RequestId:           req.RequestId,
			FirstExecutionRunId: req.FirstExecutionRunId,
			Reason:              req.Reason,
		},
	}
	_, err := a.HistoryClient.RequestCancelWorkflowExecution(ctx, rreq)

	// FIXME: check this error handling
	switch err := err.(type) {
	case nil:
		return nil
	case *serviceerror.Unavailable:
		return temporal.NewApplicationError(err.Error(), reflect.TypeOf(err).Name())
	case *serviceerror.Internal:
		return temporal.NewApplicationError(err.Error(), reflect.TypeOf(err).Name())
	default:
		return temporal.NewNonRetryableApplicationError(err.Error(), reflect.TypeOf(err).Name(), nil)
	}
}

func (a *activities) TerminateWorkflow(ctx context.Context, req *schedspb.TerminateWorkflowRequest) error {
	rreq := &historyservice.TerminateWorkflowExecutionRequest{
		NamespaceId: req.NamespaceId,
		TerminateRequest: &workflowservice.TerminateWorkflowExecutionRequest{
			Namespace:           req.Namespace,
			WorkflowExecution:   &commonpb.WorkflowExecution{WorkflowId: req.WorkflowId},
			Reason:              req.Reason,
			Identity:            req.Identity,
			FirstExecutionRunId: req.FirstExecutionRunId,
		},
	}
	_, err := a.HistoryClient.TerminateWorkflowExecution(ctx, rreq)

	// FIXME: check this error handling
	switch err := err.(type) {
	case nil:
		return nil
	case *serviceerror.Unavailable:
		return temporal.NewApplicationError(err.Error(), reflect.TypeOf(err).Name())
	case *serviceerror.Internal:
		return temporal.NewApplicationError(err.Error(), reflect.TypeOf(err).Name())
	default:
		return temporal.NewNonRetryableApplicationError(err.Error(), reflect.TypeOf(err).Name(), nil)
	}
}

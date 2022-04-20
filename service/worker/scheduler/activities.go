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
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/namespace"
)

type (
	activities struct {
		activityDeps
	}

	// FIXME: convert these all to protos?

	watchWorkflowRequest struct {
		Namespace   namespace.Name
		NamespaceID namespace.ID
		Execution   *commonpb.WorkflowExecution
	}

	watchWorkflowResponse struct {
		Status  enumspb.WorkflowExecutionStatus
		Result  *commonpb.Payloads
		Failure *failurepb.Failure
	}

	startWorkflowRequest struct {
		NamespaceID          namespace.ID
		Request              *workflowservice.StartWorkflowExecutionRequest
		ActualStartTime      time.Time
		LastCompletionResult *commonpb.Payloads
		ContinuedFailure     *failurepb.Failure
	}

	startWorkflowResponse struct {
		RunID    string
		RealTime time.Time
	}

	cancelWorkflowRequest struct {
		Namespace   namespace.Name
		NamespaceID namespace.ID
		RequestID   string
		Identity    string
		Execution   *commonpb.WorkflowExecution
	}

	terminateWorkflowRequest struct {
		Namespace   namespace.Name
		NamespaceID namespace.ID
		Identity    string
		Execution   *commonpb.WorkflowExecution
		Reason      string
	}

	errFollow string
)

var (
	errTryAgain = errors.New("try again")
)

func (e errFollow) Error() string { return string(e) }

func (a *activities) StartWorkflow(ctx context.Context, req *startWorkflowRequest) (*startWorkflowResponse, error) {
	request := common.CreateHistoryStartWorkflowRequest(
		req.NamespaceID.String(),
		req.Request,
		nil,
		req.ActualStartTime,
	)
	request.LastCompletionResult = req.LastCompletionResult
	request.ContinuedFailure = req.ContinuedFailure

	// TODO: ideally, get the time of the workflow execution started event
	// instead of this one, which will be close but not the same
	now := time.Now()

	res, err := a.HistoryClient.StartWorkflowExecution(ctx, request)
	if err != nil {
		if common.IsServiceTransientError(err) {
			return nil, temporal.NewApplicationError(err.Error(), reflect.TypeOf(err).Name())
		}
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), reflect.TypeOf(err).Name(), nil)
	}

	return &startWorkflowResponse{
		RunID:    res.RunId,
		RealTime: now,
	}, nil
}

func (a *activities) tryWatchWorkflow(ctx context.Context, req *watchWorkflowRequest) (*watchWorkflowResponse, error) {
	// make sure we return and heartbeat 5s before the timeout
	ctx2, cancel := context.WithTimeout(ctx, activity.GetInfo(ctx).HeartbeatTimeout-5*time.Second)
	defer cancel()

	// poll history service directly instead of just going to frontend to avoid
	// using resources on frontend while waiting.
	pollReq := &historyservice.PollMutableStateRequest{
		NamespaceId:         req.NamespaceID.String(),
		Execution:           req.Execution,
		ExpectedNextEventId: common.EndEventID,
	}
	// this will block up for workflow completion to 20s (default) and return
	// the current mutable state at that point
	pollResp, err := a.HistoryClient.PollMutableState(ctx, pollReq)

	// FIXME: separate out retriable vs unretriable errors
	if err != nil {
		return nil, err
	}

	if pollResp.WorkflowStatus == enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING {
		return nil, errTryAgain // not completed yet, just try again
	}

	// get last event from history
	// TODO: could we read from persistence directly or is that too crazy?
	histReq := &workflowservice.GetWorkflowExecutionHistoryRequest{
		Namespace:              req.Namespace.String(),
		Execution:              req.Execution,
		MaximumPageSize:        1,
		HistoryEventFilterType: enumspb.HISTORY_EVENT_FILTER_TYPE_CLOSE_EVENT,
		SkipArchival:           true, // should be recently completed, no need for archival
	}
	histResp, err := a.FrontendClient.GetWorkflowExecutionHistory(ctx2, histReq)

	// FIXME: separate out retriable vs unretriable errors
	if err != nil {
		return nil, err
	}

	events := histResp.GetHistory().GetEvents()
	if len(events) < 1 {
		return nil, errInternal
	}
	lastEvent := events[0]

	switch pollResp.WorkflowStatus {
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		attrs := lastEvent.GetWorkflowExecutionCompletedEventAttributes()
		if attrs == nil {
			return nil, errInternal
		}
		if len(attrs.NewExecutionRunId) > 0 {
			return nil, errFollow(attrs.NewExecutionRunId)
		}
		return &watchWorkflowResponse{Status: pollResp.WorkflowStatus, Result: attrs.Result}, nil
	case enumspb.WORKFLOW_EXECUTION_STATUS_FAILED:
		attrs := lastEvent.GetWorkflowExecutionFailedEventAttributes()
		if attrs == nil {
			return nil, errInternal
		}
		if len(attrs.NewExecutionRunId) > 0 {
			return nil, errFollow(attrs.NewExecutionRunId)
		}
		return &watchWorkflowResponse{Status: pollResp.WorkflowStatus, Failure: attrs.Failure}, nil
	case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED:
		attrs := lastEvent.GetWorkflowExecutionCanceledEventAttributes()
		if attrs == nil {
			return nil, errInternal
		}
		return &watchWorkflowResponse{Status: pollResp.WorkflowStatus}, nil
	case enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		attrs := lastEvent.GetWorkflowExecutionTerminatedEventAttributes()
		if attrs == nil {
			return nil, errInternal
		}
		return &watchWorkflowResponse{Status: pollResp.WorkflowStatus}, nil
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
		return &watchWorkflowResponse{Status: pollResp.WorkflowStatus}, nil
	}

	return nil, errInternal
}

func (a *activities) WatchWorkflow(ctx context.Context, req *watchWorkflowRequest) (*watchWorkflowResponse, error) {
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

func (a *activities) CancelWorkflow(ctx context.Context, req *cancelWorkflowRequest) error {
	rreq := &historyservice.RequestCancelWorkflowExecutionRequest{
		NamespaceId: req.NamespaceID.String(),
		CancelRequest: &workflowservice.RequestCancelWorkflowExecutionRequest{
			Namespace:         req.Namespace.String(),
			WorkflowExecution: req.Execution,
			Identity:          req.Identity,
			RequestId:         req.RequestID,
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
		// TODO: should we retry these?
		return temporal.NewApplicationError(err.Error(), reflect.TypeOf(err).Name())
	default:
		return temporal.NewNonRetryableApplicationError(err.Error(), reflect.TypeOf(err).Name(), nil)
	}
}

func (a *activities) TerminateWorkflow(ctx context.Context, req *terminateWorkflowRequest) error {
	rreq := &historyservice.TerminateWorkflowExecutionRequest{
		NamespaceId: req.NamespaceID.String(),
		TerminateRequest: &workflowservice.TerminateWorkflowExecutionRequest{
			Namespace:         req.Namespace.String(),
			WorkflowExecution: req.Execution,
			Reason:            req.Reason,
			Identity:          req.Identity,
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
		// TODO: should we retry these?
		return temporal.NewApplicationError(err.Error(), reflect.TypeOf(err).Name())
	default:
		return temporal.NewNonRetryableApplicationError(err.Error(), reflect.TypeOf(err).Name(), nil)
	}
}

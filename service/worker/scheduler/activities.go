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
	"reflect"
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/activity"
	sdkclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/server/api/historyservice/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/namespace"
)

type (
	activities struct {
		metricsClient metrics.Client
		logger        log.Logger
		sdkClient     sdkclient.Client
		historyClient historyservice.HistoryServiceClient
	}

	watchWorkflowRequest struct {
		WorkflowID string
		RunID      string // FIXME: do we need or want this?
	}

	watchWorkflowResponse struct {
		// Failed is true iff workflow "failed" or "timed out" (cancel and terminate do not count)
		Failed bool
		// WorkflowError has error details if any
		WorkflowError error
	}

	startWorkflowRequest struct {
		NamespaceID     namespace.ID
		Request         *workflowservice.StartWorkflowExecutionRequest
		ActualStartTime time.Time
	}

	startWorkflowResponse struct {
		RunID    string
		RealTime time.Time
	}

	cancelWorkflowRequest struct {
		WorkflowID string
	}
)

func (a *activities) StartWorkflow(ctx context.Context, req *startWorkflowRequest) (*startWorkflowResponse, error) {
	request := common.CreateHistoryStartWorkflowRequest(
		string(req.NamespaceID),
		req.Request,
		nil,
		req.ActualStartTime,
	)

	// TODO: ideally, get the time of the workflow execution started event
	// instead of this one, which will be close but not the same
	now := time.Now()

	res, err := a.historyClient.StartWorkflowExecution(ctx, request)
	if err != nil {
		if common.IsPersistenceTransientError(err) {
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
	// sdk uses 65 seconds for a single long poll grpc call. we want to do individual
	// calls and heartbeat in between, so we should set a slightly smaller timeout.
	ctx2, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	err := a.sdkClient.GetWorkflow(ctx2, req.WorkflowID, req.RunID).Get(ctx2, nil)
	if ctx2.Err() != nil { // FIXME: is that the best way to tell if the call timed out? what will Get actually return?
		return nil, ctx2.Err()
	}
	switch err := err.(type) {
	case nil:
		return &watchWorkflowResponse{Failed: false, WorkflowError: nil}, nil
	// FIXME: what does a "not found" error come out as here?
	case *temporal.WorkflowExecutionError:
		switch err := err.Unwrap().(type) {
		case *temporal.ApplicationError:
			return &watchWorkflowResponse{Failed: true, WorkflowError: err}, nil
		case *temporal.TimeoutError:
			return &watchWorkflowResponse{Failed: true, WorkflowError: err}, nil
		case *temporal.CanceledError:
			return &watchWorkflowResponse{Failed: false, WorkflowError: err}, nil
		case *temporal.TerminatedError:
			return &watchWorkflowResponse{Failed: false, WorkflowError: err}, nil
		}
	}
	a.logger.Error("unexpected error from WorkflowRun.Get", tag.Error(err))
	return nil, err
}

func (a *activities) WatchWorkflow(ctx context.Context, req *watchWorkflowRequest) (*watchWorkflowResponse, error) {
	for {
		res, err := a.tryWatchWorkflow(ctx, req)
		if err == context.DeadlineExceeded {
			activity.RecordHeartbeat(ctx)
			continue
		}
		return res, err
	}
}

func (a *activities) CancelWorkflow(ctx context.Context, req *cancelWorkflowRequest) error {
	// TODO: does ctx get set up with the correct deadline? (from StartToCloseTimeout in my activity options?)
	err := a.sdkClient.CancelWorkflow(ctx, req.WorkflowID, "")
	// Differentiate between error types
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

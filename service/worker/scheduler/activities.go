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

	"go.temporal.io/api/workflowservice/v1"
	sdkclient "go.temporal.io/sdk/client"
)

type (
	watchWorkflowRequest struct {
		WorkflowID string
		RunID      string
	}

	watchWorkflowResponse struct {
		// failed is true iff workflow "failed" or "timed out" (cancel and terminate do not count)
		Failed bool
		// err has error details if any
		WorkflowError string
		// unretriable client error
		Error error
	}

	startWorkflowRequest struct {
		Request *workflowservice.StartWorkflowExecutionRequest
	}

	startWorkflowResponse struct {
		Response workflowservice.StartWorkflowExecutionResponse
		Error    error
	}
)

func (a *activities) StartWorkflow(ctx context.Context, req *startWorkflowRequest) *startWorkflowResponse {
	// send req directly to frontend, or to history?
	return &startWorkflowResponse{Error: errors.New("FIXME")}
}

func (a *activities) WatchWorkflow(ctx context.Context, req *watchWorkflowRequest) *watchWorkflowResponse {
	// FIXME: don't forget to heartbeat
	return &watchWorkflowResponse{Error: errors.New("FIXME")}
}

func (a *activities) CancelWorkflow(ctx context.Context, id string) error {
	return nil, errors.New("FIXME")
	cli := sdkclient.Client()
	err := cli.CancelWorkflow(id, "")
}

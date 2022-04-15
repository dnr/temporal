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

// Code generated by MockGen. DO NOT EDIT.
// Source: replicationDLQHandler.go

// Package history is a generated GoMock package.
package history

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	repication "go.temporal.io/server/api/replication/v1"
)

// MockreplicationDLQHandler is a mock of replicationDLQHandler interface.
type MockreplicationDLQHandler struct {
	ctrl     *gomock.Controller
	recorder *MockreplicationDLQHandlerMockRecorder
}

// MockreplicationDLQHandlerMockRecorder is the mock recorder for MockreplicationDLQHandler.
type MockreplicationDLQHandlerMockRecorder struct {
	mock *MockreplicationDLQHandler
}

// NewMockreplicationDLQHandler creates a new mock instance.
func NewMockreplicationDLQHandler(ctrl *gomock.Controller) *MockreplicationDLQHandler {
	mock := &MockreplicationDLQHandler{ctrl: ctrl}
	mock.recorder = &MockreplicationDLQHandlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockreplicationDLQHandler) EXPECT() *MockreplicationDLQHandlerMockRecorder {
	return m.recorder
}

// getMessages mocks base method.
func (m *MockreplicationDLQHandler) getMessages(ctx context.Context, sourceCluster string, lastMessageID int64, pageSize int, pageToken []byte) ([]*repication.ReplicationTask, []byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getMessages", ctx, sourceCluster, lastMessageID, pageSize, pageToken)
	ret0, _ := ret[0].([]*repication.ReplicationTask)
	ret1, _ := ret[1].([]byte)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// getMessages indicates an expected call of getMessages.
func (mr *MockreplicationDLQHandlerMockRecorder) getMessages(ctx, sourceCluster, lastMessageID, pageSize, pageToken interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getMessages", reflect.TypeOf((*MockreplicationDLQHandler)(nil).getMessages), ctx, sourceCluster, lastMessageID, pageSize, pageToken)
}

// mergeMessages mocks base method.
func (m *MockreplicationDLQHandler) mergeMessages(ctx context.Context, sourceCluster string, lastMessageID int64, pageSize int, pageToken []byte) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "mergeMessages", ctx, sourceCluster, lastMessageID, pageSize, pageToken)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// mergeMessages indicates an expected call of mergeMessages.
func (mr *MockreplicationDLQHandlerMockRecorder) mergeMessages(ctx, sourceCluster, lastMessageID, pageSize, pageToken interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "mergeMessages", reflect.TypeOf((*MockreplicationDLQHandler)(nil).mergeMessages), ctx, sourceCluster, lastMessageID, pageSize, pageToken)
}

// purgeMessages mocks base method.
func (m *MockreplicationDLQHandler) purgeMessages(ctx context.Context, sourceCluster string, lastMessageID int64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "purgeMessages", ctx, sourceCluster, lastMessageID)
	ret0, _ := ret[0].(error)
	return ret0
}

// purgeMessages indicates an expected call of purgeMessages.
func (mr *MockreplicationDLQHandlerMockRecorder) purgeMessages(ctx, sourceCluster, lastMessageID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "purgeMessages", reflect.TypeOf((*MockreplicationDLQHandler)(nil).purgeMessages), ctx, sourceCluster, lastMessageID)
}

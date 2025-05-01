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

//go:generate mockgen -copyright_file ../../LICENSE -package $GOPACKAGE -source $GOFILE -destination factory_mock.go

package sdk

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"
	"sync/atomic"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	sdkclient "go.temporal.io/sdk/client"
	sdkworker "go.temporal.io/sdk/worker"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/primitives"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type (
	ClientFactory interface {
		// options must include Namespace and should not include: HostPort, ConnectionOptions,
		// MetricsHandler, or Logger (they will be overwritten)
		NewClient(options sdkclient.Options) sdkclient.Client
		GetSystemClient() sdkclient.Client
		NewWorker(client sdkclient.Client, taskQueue string, options sdkworker.Options) sdkworker.Worker
	}

	clientFactory struct {
		hostPort        string
		tlsConfig       *tls.Config
		metricsHandler  *MetricsHandler
		logger          log.Logger
		systemSdkClient sdkclient.Client
		stickyCacheSize dynamicconfig.IntPropertyFn
		once            sync.Once
	}

	autoresetter struct {
		client atomic.Value // sdkclient.Client
	}
)

var (
	_ ClientFactory = (*clientFactory)(nil)
)

func NewClientFactory(
	hostPort string,
	tlsConfig *tls.Config,
	metricsHandler metrics.Handler,
	logger log.Logger,
	stickyCacheSize dynamicconfig.IntPropertyFn,
) *clientFactory {
	return &clientFactory{
		hostPort:        hostPort,
		tlsConfig:       tlsConfig,
		metricsHandler:  NewMetricsHandler(metricsHandler),
		logger:          logger,
		stickyCacheSize: stickyCacheSize,
	}
}

func (f *clientFactory) options(options sdkclient.Options, a *autoresetter) sdkclient.Options {
	options.HostPort = f.hostPort
	options.MetricsHandler = f.metricsHandler
	logger := log.NewSdkLogger(f.logger)
	logger.OnError = a.onError
	options.Logger = logger
	options.ConnectionOptions = sdkclient.ConnectionOptions{
		TLS: f.tlsConfig,
		DialOptions: []grpc.DialOption{
			grpc.WithUnaryInterceptor(sdkClientNameHeadersInjectorInterceptor()),
		},
	}
	return options
}

func (f *clientFactory) NewClient(options sdkclient.Options) sdkclient.Client {
	// this shouldn't fail if the first client was created successfully
	a := new(autoresetter)
	client, err := sdkclient.NewClientFromExisting(f.GetSystemClient(), f.options(options, a))
	if err != nil {
		f.logger.Fatal("error creating sdk client", tag.Error(err))
	}
	a.setClient(client)
	return client
}

func (f *clientFactory) GetSystemClient() sdkclient.Client {
	f.once.Do(func() {
		err := backoff.ThrottleRetry(func() error {
			a := new(autoresetter)
			sdkClient, err := sdkclient.Dial(f.options(sdkclient.Options{
				Namespace: primitives.SystemLocalNamespace,
			}, a))
			if err != nil {
				f.logger.Warn("error creating sdk client", tag.Error(err))
				return err
			}
			a.setClient(sdkClient)
			f.systemSdkClient = sdkClient
			return nil
		}, common.CreateSdkClientFactoryRetryPolicy(), func(err error) bool {
			// note err is wrapped by sdk
			var unavail *serviceerror.Unavailable
			return common.IsContextDeadlineExceededErr(err) || errors.As(err, &unavail)
		})
		if err != nil {
			f.logger.Fatal("error creating sdk client", tag.Error(err))
		}

		if size := f.stickyCacheSize(); size > 0 {
			f.logger.Info("setting sticky workflow cache size", tag.NewInt("size", size))
			sdkworker.SetStickyWorkflowCacheSize(size)
		}
	})
	return f.systemSdkClient
}

func (f *clientFactory) NewWorker(
	client sdkclient.Client,
	taskQueue string,
	options sdkworker.Options,
) sdkworker.Worker {
	return sdkworker.New(client, taskQueue, options)
}

// Overwrite the 'client-name' and 'client-version' headers on gRPC requests sent using the Go SDK
// so they clearly indicate that the request is coming from the Temporal server.
func sdkClientNameHeadersInjectorInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Can't use headers.SetVersions() here because it is _appending_ headers to the context
		// rather than _replacing_ them, which means Go SDK's default headers would still be present.
		md, mdExist := metadata.FromOutgoingContext(ctx)
		if !mdExist {
			md = metadata.New(nil)
		}
		md.Set(headers.ClientNameHeaderName, headers.ClientNameServer)
		md.Set(headers.ClientVersionHeaderName, headers.ServerVersion)
		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func (a *autoresetter) setClient(client sdkclient.Client) {
	a.client.Store(client)
}
func (a *autoresetter) onError(msg string, kv []any) {
	client, ok := a.client.Load().(sdkclient.Client)
	if !ok {
		return
	}
	req := workflowservice.ResetWorkflowExecutionRequest{
		Reason:                    "auto-reset for internal workflow nondeterminism error",
		WorkflowExecution:         &commonpb.WorkflowExecution{},
		WorkflowTaskFinishEventId: 2, // FIXME: can we use 2 as the first wtf started event or do we have to look it up?
	}
	for i := 0; i+1 < len(kv); i += 2 {
		k, _ := kv[i].(string)
		v, _ := kv[i+1].(string)
		if k == "" || v == "" {
			continue
		}
		switch k {
		case "Namespace":
			req.Namespace = v
		case "WorkflowID":
			req.WorkflowExecution.WorkflowId = v
		}
	}
	if req.Namespace == "" && req.WorkflowExecution.WorkflowId == "" {
		return
	}
	_, _ = client.ResetWorkflowExecution(context.TODO(), &req)
}

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

package deadlock

import (
	"context"
	"runtime/pprof"
	"strings"
	"time"

	"go.uber.org/fx"
	"google.golang.org/grpc/health"

	"go.temporal.io/server/common"
	"go.temporal.io/server/common/cluster"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/internal/goro"
	"go.temporal.io/server/service/history/shard"
)

type (
	params struct {
		fx.In

		Logger       log.Logger
		Collection   *dynamicconfig.Collection
		HealthServer *health.Server

		// root pingables:
		NamespaceRegistry namespace.Registry
		ClusterMetadata   cluster.Metadata
		ShardController   shard.Controller `optional:"true"`
	}

	config struct {
		DumpGoroutines  dynamicconfig.BoolPropertyFn
		FailHealthCheck dynamicconfig.BoolPropertyFn
		AbortProcess    dynamicconfig.BoolPropertyFn
	}

	deadlockDetector struct {
		logger       log.Logger
		healthServer *health.Server
		config       config
		roots        []common.Pingable
		loopGoro     *goro.Handle
		checkCh      chan common.PingCheck
	}
)

func NewDeadlockDetector(params params) *deadlockDetector {
	roots := []common.Pingable{
		params.NamespaceRegistry,
		params.ClusterMetadata,
	}
	if params.ShardController != nil {
		roots = append(roots, params.ShardController)
	}
	return &deadlockDetector{
		logger:       params.Logger,
		healthServer: params.HealthServer,
		config: config{
			DumpGoroutines:  params.Collection.GetBoolProperty(dynamicconfig.DeadlockDumpGoroutines, true),
			FailHealthCheck: params.Collection.GetBoolProperty(dynamicconfig.DeadlockFailHealthCheck, true),
			AbortProcess:    params.Collection.GetBoolProperty(dynamicconfig.DeadlockAbortProcess, false),
		},
		roots:   roots,
		checkCh: make(chan common.PingCheck, 10),
	}
}

func (dd *deadlockDetector) Start() error {
	dd.loopGoro = goro.NewHandle(context.Background())
	dd.loopGoro.Go(dd.loop)
	for i := 0; i < 10; i++ { //FIXME: dynconfig or something
		go dd.pingWorker()
	}
	return nil
}

func (dd *deadlockDetector) Stop() error {
	dd.loopGoro.Cancel()
	<-dd.loopGoro.Done()
	close(dd.checkCh)
	// don't wait for workers to exit, they may be blocked
	return nil
}

func (dd *deadlockDetector) loop(ctx context.Context) error {
	dd.logger.Info("deadlock detector starting")
	for {
		dd.ping(dd.roots)
		select {
		case <-time.After(30 * time.Second): // FIXME: dynconfig or something
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (dd *deadlockDetector) ping(pingables []common.Pingable) {
	for _, pingable := range pingables {
		for _, check := range pingable.GetPingChecks() {
			dd.checkCh <- check
		}
	}
}

func (dd *deadlockDetector) pingWorker() {
	for check := range dd.checkCh {
		// Using AfterFunc is (hopefully?) cheaper than creating another goroutine to be
		// the waiter, since we expect to always cancel it. If the go runtime is so messed
		// up that it can't create a goroutine, that's a bigger problem than we can handle.
		t := time.AfterFunc(check.Timeout, func() { dd.detected(check.Name) })
		newPingables := check.Ping()
		t.Stop()

		dd.logger.Debug("ping check succeeded", tag.Name(check.Name))

		dd.ping(newPingables)
	}
}

func (dd *deadlockDetector) detected(name string) {
	if dd.loopGoro.Err() != nil {
		// we shut down already, ignore any detected deadlocks
		return
	}

	dd.logger.Error("deadlock detected", tag.Name(name))

	if dd.config.FailHealthCheck() {
		dd.logger.Info("marking grpc services unhealthy")
		dd.healthServer.Shutdown()
	}

	if dd.config.DumpGoroutines() {
		dd.dumpGoroutines()
	}

	if dd.config.AbortProcess() {
		dd.logger.Fatal("deadlock detected", tag.Name(name))
	}
}

func (dd *deadlockDetector) dumpGoroutines() {
	profile := pprof.Lookup("goroutine")
	if profile == nil {
		dd.logger.Error("could not find goroutine profile")
		return
	}
	var b strings.Builder
	err := profile.WriteTo(&b, 1) // 1 is magic value that means "text format"
	if err != nil {
		dd.logger.Error("failed to get goroutine profile", tag.Error(err))
		return
	}
	// write it as a single log line with embedded newlines.
	// the value starts with "goroutine profile: total ...\n" so it should be clear
	dd.logger.Info(b.String())
}

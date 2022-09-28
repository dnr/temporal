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
		Collection   dynamicconfig.Collection
		HealthServer *health.Server

		// pingables:
		NamespaceRegistry namespace.Registry
		ClusterMetadata   cluster.Metadata
		ShardController   shard.Controller `optional:"true"`
	}

	pingable struct {
		target  common.Pingable
		timeout time.Duration
	}

	config struct {
	}

	deadlockDetector struct {
		logger       log.Logger
		healthServer *health.Server
		config       config
		pingables    map[string]pingable
		loopGoro     *goro.Handle
	}
)

func NewDeadlockDetector(params params) *deadlockDetector {
	pingables := map[string]common.Pingable{
		"NamespaceRegistry": pingable{
			target:  params.NamespaceRegistry,
			timeout: 15 * time.Second, // FIXME: check and document
		},
		"ClusterMetadata": pingable{
			target:  params.ClusterMetadata,
			timeout: 15 * time.Second, // FIXME: check and document
		},
	}
	if params.ShardController != nil {
		pingables["ShardController"] = pingable{
			target:  params.ShardController,
			timeout: 15 * time.Second, // FIXME: check and document
		}
	}
	return &deadlockDetector{
		logger:       params.Logger,
		healthServer: params.HealthServer,
		config:       config{},
		pingables:    pingables,
	}
}

func (dd *deadlockDetector) Start() error {
	dd.loopGoro = goro.NewHandle(context.Background())
	dd.loopGoro.Go(dd.loop)
	return nil
}

func (dd *deadlockDetector) Stop() error {
	dd.loopGoro.Cancel()
	return nil
}

func (dd *deadlockDetector) loop(ctx context.Context) {
	t := time.NewTicker(30 * time.Second) // FIXME: max of all timeouts
	defer t.Stop()
	for {
		select {
		case <-t.C:
			dd.ping()
		case <-ctx.Done():
			return
		}
	}
}

func (dd *deadlockDetector) ping() {
	for name, p := range dd.pingables {
		go func(name string, p pingable) {
			// Using AfterFunc is (hopefully?) cheaper than creating another goroutine to be
			// the waiter, since we expect to always cancel it. If the go runtime is so messed
			// up that it can't create a goroutine, that's a bigger problem than we can handle.
			t := time.AfterFunc(p.timeout, func() { dd.detected(name) })
			p.target.PingLock()
			t.Stop()
		}(name, p)
	}
}

func (dd *deadlockDetector) detected(name string) {
	dd.logger.Error("deadlock detected", tag.Name(name))

	if dd.config.DumpGoroutines() {
		if profile := pprof.Lookup("goroutine"); profile != nil {
			var b strings.Builder
			err := profile.WriteTo(&b, 1) // 1 is magic value that means "text format"
			if err == nil {
				// write it as a single log line with embedded newlines.
				// the value starts with "goroutine profile: total ...\n" so it should be clear
				dd.logger.Info(b.String())
			} else {
				dd.logger.Error("failed to get goroutine profile", tag.Error(err))
			}
		} else {
			dd.logger.Error("could not find goroutine profile")
		}
	}

	if dd.config.FailHeathCheck() {
		dd.logger.Error("marking unhealthy")
		dd.healthServer.Shutdown()
	}

	if dd.config.Abort() {
		dd.logger.Fatal("deadlock detected", tag.Name(name))
	}
}

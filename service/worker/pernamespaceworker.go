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

package worker

import (
	"sync"
	"sync/atomic"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	sdkclient "go.temporal.io/sdk/client"
	sdkworker "go.temporal.io/sdk/worker"
	"go.uber.org/fx"

	"go.temporal.io/server/common"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/sdk"
	workercommon "go.temporal.io/server/service/worker/common"
)

type (
	perNamespaceWorkers struct {
		name    namespace.Name
		clients []sdkclient.Client
		workers []sdkworker.Worker
	}

	perNamespaceWorkerManager struct {
		status            int32
		logger            log.Logger
		sdkClientFactory  sdk.ClientFactory
		namespaceRegistry namespace.Registry
		components        []workercommon.PerNamespaceWorkerComponent

		lock    sync.Mutex
		workers map[namespace.ID]*perNamespaceWorkers
	}

	perNamespaceWorkerManagerInitParams struct {
		fx.In
		Logger            log.Logger
		SdkClientFactory  sdk.ClientFactory
		NamespaceRegistry namespace.Registry
		Components        []workercommon.PerNamespaceWorkerComponent `group:"perNamespaceWorkerComponent"`
	}
)

func NewPerNamespaceWorkerManager(params perNamespaceWorkerManagerInitParams) *perNamespaceWorkerManager {
	return &perNamespaceWorkerManager{
		logger:            params.Logger,
		sdkClientFactory:  params.SdkClientFactory,
		namespaceRegistry: params.NamespaceRegistry,
		components:        params.Components,
		workers:           make(map[namespace.ID]*perNamespaceWorkers),
	}
}

func (wm *perNamespaceWorkerManager) Start() {
	if !atomic.CompareAndSwapInt32(
		&wm.status,
		common.DaemonStatusInitialized,
		common.DaemonStatusStarted,
	) {
		return
	}

	wm.logger.Info("", tag.ComponentPerNSWorkerManager, tag.LifeCycleStarting)

	// this will call namespaceCallback with current namespaces
	wm.namespaceRegistry.RegisterStateChangeCallback(wm, wm.namespaceCallback)

	wm.logger.Info("", tag.ComponentPerNSWorkerManager, tag.LifeCycleStarted)
}

func (wm *perNamespaceWorkerManager) Stop() {
	if !atomic.CompareAndSwapInt32(
		&wm.status,
		common.DaemonStatusStarted,
		common.DaemonStatusStopped,
	) {
		return
	}

	wm.logger.Info("", tag.ComponentPerNSWorkerManager, tag.LifeCycleStopping)

	wm.namespaceRegistry.UnregisterStateChangeCallback(wm)

	wm.lock.Lock()
	defer wm.lock.Unlock()

	for _, pnw := range wm.workers {
		for _, worker := range pnw.workers {
			worker.Stop()
		}
		for _, client := range pnw.clients {
			client.Close()
		}
	}

	wm.logger.Info("", tag.ComponentPerNSWorkerManager, tag.LifeCycleStopped)
}

func (wm *perNamespaceWorkerManager) Running() bool {
	return atomic.LoadInt32(&wm.status) == common.DaemonStatusStarted
}

func (wm *perNamespaceWorkerManager) namespaceCallback(ns *namespace.Namespace) {
	switch ns.State() {
	case enumspb.NAMESPACE_STATE_REGISTERED:
		wm.setupNamespace(ns)
	case enumspb.NAMESPACE_STATE_DEPRECATED:
		// TODO: handle namespace deprecation/deletion
	case enumspb.NAMESPACE_STATE_DELETED:
		// TODO: handle namespace deprecation/deletion
	}
}

func (wm *perNamespaceWorkerManager) setupNamespace(ns *namespace.Namespace) {
	wm.lock.Lock()
	defer wm.lock.Unlock()

	if _, ok := wm.workers[ns.ID()]; ok {
		return
	}

	pnw := &perNamespaceWorkers{name: ns.Name()}
	wm.workers[ns.ID()] = pnw
	for _, wc := range wm.components {
		go wm.setupComponent(ns, wc, pnw)
	}
}

func (wm *perNamespaceWorkerManager) setupComponent(
	ns *namespace.Namespace,
	wc workercommon.PerNamespaceWorkerComponent,
	pnw *perNamespaceWorkers,
) {
	op := func() error {
		if !wm.Running() {
			return nil
		}
		client, err := wm.sdkClientFactory.NewClient(ns.Name().String(), wm.logger)
		if err != nil {
			return err
		}
		options := wc.DedicatedWorkerOptions()
		worker := sdkworker.New(client, options.TaskQueue, options.Options)
		wc.Register(worker)
		worker.Start()

		wm.lock.Lock()
		defer wm.lock.Unlock()

		if !wm.Running() {
			worker.Stop()
			client.Close()
			return nil
		}

		pnw.clients = append(pnw.clients, client)
		pnw.workers = append(pnw.workers, worker)

		return nil
	}
	policy := backoff.NewExponentialRetryPolicy(1 * time.Second)
	backoff.Retry(op, policy, nil)
}

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
	"context"

	"go.temporal.io/api/serviceerror"

	"go.temporal.io/server/api/matchingservice/v1"
	"go.temporal.io/server/client"
	"go.temporal.io/server/common/cluster"
	"go.temporal.io/server/common/config"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/membership"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/persistence"
	persistenceClient "go.temporal.io/server/common/persistence/client"
	"go.temporal.io/server/common/persistence/visibility"
	"go.temporal.io/server/common/persistence/visibility/manager"
	esclient "go.temporal.io/server/common/persistence/visibility/store/elasticsearch/client"
	"go.temporal.io/server/common/primitives"
	"go.temporal.io/server/common/resource"
	"go.temporal.io/server/common/sdk"
	"go.temporal.io/server/service/worker/batcher"
	"go.temporal.io/server/service/worker/parentclosepolicy"
	"go.temporal.io/server/service/worker/replicator"
	"go.temporal.io/server/service/worker/scanner"
)

type (
	// Service represents the temporal-worker service. This service hosts all background processing needed for temporal cluster:
	// Replicator: Handles applying replication tasks generated by remote clusters.
	// Archiver: Handles archival of workflow histories.
	Service struct {
		logger                 log.Logger
		clusterMetadata        cluster.Metadata
		clientBean             client.Bean
		clusterMetadataManager persistence.ClusterMetadataManager
		metadataManager        persistence.MetadataManager
		membershipMonitor      membership.Monitor
		hostInfo               membership.HostInfo
		executionManager       persistence.ExecutionManager
		taskManager            persistence.TaskManager
		historyClient          resource.HistoryClient
		namespaceRegistry      namespace.Registry
		workerServiceResolver  membership.ServiceResolver
		visibilityManager      manager.VisibilityManager

		namespaceReplicationQueue persistence.NamespaceReplicationQueue

		persistenceBean persistenceClient.Bean

		metricsHandler metrics.Handler

		sdkClientFactory sdk.ClientFactory
		esClient         esclient.Client
		config           *Config

		workerManager                    *workerManager
		perNamespaceWorkerManager        *perNamespaceWorkerManager
		scanner                          *scanner.Scanner
		matchingClient                   matchingservice.MatchingServiceClient
		namespaceReplicationTaskExecutor namespace.ReplicationTaskExecutor
	}

	// Config contains all the service config for worker
	Config struct {
		ScannerCfg                            *scanner.Config
		ParentCloseCfg                        *parentclosepolicy.Config
		ThrottledLogRPS                       dynamicconfig.IntPropertyFn
		PersistenceMaxQPS                     dynamicconfig.IntPropertyFn
		PersistenceGlobalMaxQPS               dynamicconfig.IntPropertyFn
		PersistenceNamespaceMaxQPS            dynamicconfig.IntPropertyFnWithNamespaceFilter
		PersistenceGlobalNamespaceMaxQPS      dynamicconfig.IntPropertyFnWithNamespaceFilter
		PersistencePerShardNamespaceMaxQPS    dynamicconfig.IntPropertyFnWithNamespaceFilter
		EnablePersistencePriorityRateLimiting dynamicconfig.BoolPropertyFn
		PersistenceDynamicRateLimitingParams  dynamicconfig.MapPropertyFn
		PersistenceQPSBurstRatio              dynamicconfig.FloatPropertyFn
		OperatorRPSRatio                      dynamicconfig.FloatPropertyFn
		EnableBatcher                         dynamicconfig.BoolPropertyFn
		BatcherRPS                            dynamicconfig.IntPropertyFnWithNamespaceFilter
		BatcherConcurrency                    dynamicconfig.IntPropertyFnWithNamespaceFilter
		EnableParentClosePolicyWorker         dynamicconfig.BoolPropertyFn
		PerNamespaceWorkerCount               dynamicconfig.IntPropertyFnWithNamespaceFilter
		PerNamespaceWorkerOptions             dynamicconfig.MapPropertyFnWithNamespaceFilter
		PerNamespaceWorkerStartRate           dynamicconfig.FloatPropertyFn

		VisibilityPersistenceMaxReadQPS   dynamicconfig.IntPropertyFn
		VisibilityPersistenceMaxWriteQPS  dynamicconfig.IntPropertyFn
		EnableReadFromSecondaryVisibility dynamicconfig.BoolPropertyFnWithNamespaceFilter
		VisibilityDisableOrderByClause    dynamicconfig.BoolPropertyFnWithNamespaceFilter
		VisibilityEnableManualPagination  dynamicconfig.BoolPropertyFnWithNamespaceFilter
	}
)

func NewService(
	logger log.SnTaggedLogger,
	serviceConfig *Config,
	sdkClientFactory sdk.ClientFactory,
	esClient esclient.Client,
	clusterMetadata cluster.Metadata,
	clientBean client.Bean,
	clusterMetadataManager persistence.ClusterMetadataManager,
	namespaceRegistry namespace.Registry,
	executionManager persistence.ExecutionManager,
	persistenceBean persistenceClient.Bean,
	membershipMonitor membership.Monitor,
	hostInfoProvider membership.HostInfoProvider,
	namespaceReplicationQueue persistence.NamespaceReplicationQueue,
	metricsHandler metrics.Handler,
	metadataManager persistence.MetadataManager,
	taskManager persistence.TaskManager,
	historyClient resource.HistoryClient,
	workerManager *workerManager,
	perNamespaceWorkerManager *perNamespaceWorkerManager,
	visibilityManager manager.VisibilityManager,
	matchingClient resource.MatchingClient,
	namespaceReplicationTaskExecutor namespace.ReplicationTaskExecutor,
) (*Service, error) {
	workerServiceResolver, err := membershipMonitor.GetResolver(primitives.WorkerService)
	if err != nil {
		return nil, err
	}

	s := &Service{
		config:                    serviceConfig,
		sdkClientFactory:          sdkClientFactory,
		esClient:                  esClient,
		logger:                    logger,
		clusterMetadata:           clusterMetadata,
		clientBean:                clientBean,
		clusterMetadataManager:    clusterMetadataManager,
		namespaceRegistry:         namespaceRegistry,
		executionManager:          executionManager,
		persistenceBean:           persistenceBean,
		workerServiceResolver:     workerServiceResolver,
		membershipMonitor:         membershipMonitor,
		hostInfo:                  hostInfoProvider.HostInfo(),
		namespaceReplicationQueue: namespaceReplicationQueue,
		metricsHandler:            metricsHandler,
		metadataManager:           metadataManager,
		taskManager:               taskManager,
		historyClient:             historyClient,
		visibilityManager:         visibilityManager,

		workerManager:                    workerManager,
		perNamespaceWorkerManager:        perNamespaceWorkerManager,
		matchingClient:                   matchingClient,
		namespaceReplicationTaskExecutor: namespaceReplicationTaskExecutor,
	}
	if err := s.initScanner(); err != nil {
		return nil, err
	}
	return s, nil
}

// NewConfig builds the new Config for worker service
func NewConfig(
	dc *dynamicconfig.Collection,
	persistenceConfig *config.Persistence,
) *Config {
	config := &Config{
		ParentCloseCfg: &parentclosepolicy.Config{
			MaxConcurrentActivityExecutionSize:     dc.GetInt(dynamicconfig.WorkerParentCloseMaxConcurrentActivityExecutionSize),
			MaxConcurrentWorkflowTaskExecutionSize: dc.GetInt(dynamicconfig.WorkerParentCloseMaxConcurrentWorkflowTaskExecutionSize),
			MaxConcurrentActivityTaskPollers:       dc.GetInt(dynamicconfig.WorkerParentCloseMaxConcurrentActivityTaskPollers),
			MaxConcurrentWorkflowTaskPollers:       dc.GetInt(dynamicconfig.WorkerParentCloseMaxConcurrentWorkflowTaskPollers),
			NumParentClosePolicySystemWorkflows:    dc.GetInt(dynamicconfig.NumParentClosePolicySystemWorkflows),
		},
		ScannerCfg: &scanner.Config{
			MaxConcurrentActivityExecutionSize:     dc.GetInt(dynamicconfig.WorkerScannerMaxConcurrentActivityExecutionSize),
			MaxConcurrentWorkflowTaskExecutionSize: dc.GetInt(dynamicconfig.WorkerScannerMaxConcurrentWorkflowTaskExecutionSize),
			MaxConcurrentActivityTaskPollers:       dc.GetInt(dynamicconfig.WorkerScannerMaxConcurrentActivityTaskPollers),
			MaxConcurrentWorkflowTaskPollers:       dc.GetInt(dynamicconfig.WorkerScannerMaxConcurrentWorkflowTaskPollers),

			PersistenceMaxQPS:                       dc.GetInt(dynamicconfig.ScannerPersistenceMaxQPS),
			Persistence:                             persistenceConfig,
			TaskQueueScannerEnabled:                 dc.GetBool(dynamicconfig.TaskQueueScannerEnabled),
			BuildIdScavengerEnabled:                 dc.GetBool(dynamicconfig.BuildIdScavengerEnabled),
			HistoryScannerEnabled:                   dc.GetBool(dynamicconfig.HistoryScannerEnabled),
			ExecutionsScannerEnabled:                dc.GetBool(dynamicconfig.ExecutionsScannerEnabled),
			HistoryScannerDataMinAge:                dc.GetDuration(dynamicconfig.HistoryScannerDataMinAge),
			HistoryScannerVerifyRetention:           dc.GetBool(dynamicconfig.HistoryScannerVerifyRetention),
			ExecutionScannerPerHostQPS:              dc.GetInt(dynamicconfig.ExecutionScannerPerHostQPS),
			ExecutionScannerPerShardQPS:             dc.GetInt(dynamicconfig.ExecutionScannerPerShardQPS),
			ExecutionDataDurationBuffer:             dc.GetDuration(dynamicconfig.ExecutionDataDurationBuffer),
			ExecutionScannerWorkerCount:             dc.GetInt(dynamicconfig.ExecutionScannerWorkerCount),
			ExecutionScannerHistoryEventIdValidator: dc.GetBool(dynamicconfig.ExecutionScannerHistoryEventIdValidator),
			RemovableBuildIdDurationSinceDefault:    dc.GetDuration(dynamicconfig.RemovableBuildIdDurationSinceDefault),
			BuildIdScavengerVisibilityRPS:           dc.GetFloat(dynamicconfig.BuildIdScavengerVisibilityRPS),
		},
		EnableBatcher:                         dc.GetBool(dynamicconfig.EnableBatcherGlobal),
		BatcherRPS:                            dc.GetIntByNamespace(dynamicconfig.BatcherRPS),
		BatcherConcurrency:                    dc.GetIntByNamespace(dynamicconfig.BatcherConcurrency),
		EnableParentClosePolicyWorker:         dc.GetBool(dynamicconfig.EnableParentClosePolicyWorker),
		PerNamespaceWorkerCount:               dc.GetIntByNamespace(dynamicconfig.WorkerPerNamespaceWorkerCount),
		PerNamespaceWorkerOptions:             dc.GetMapByNamespace(dynamicconfig.WorkerPerNamespaceWorkerOptions),
		PerNamespaceWorkerStartRate:           dc.GetFloat(dynamicconfig.WorkerPerNamespaceWorkerStartRate),
		ThrottledLogRPS:                       dc.GetInt(dynamicconfig.WorkerThrottledLogRPS),
		PersistenceMaxQPS:                     dc.GetInt(dynamicconfig.WorkerPersistenceMaxQPS),
		PersistenceGlobalMaxQPS:               dc.GetInt(dynamicconfig.WorkerPersistenceGlobalMaxQPS),
		PersistenceNamespaceMaxQPS:            dc.GetIntByNamespace(dynamicconfig.WorkerPersistenceNamespaceMaxQPS),
		PersistenceGlobalNamespaceMaxQPS:      dc.GetIntByNamespace(dynamicconfig.WorkerPersistenceGlobalNamespaceMaxQPS),
		PersistencePerShardNamespaceMaxQPS:    dynamicconfig.DefaultPerShardNamespaceRPSMax,
		EnablePersistencePriorityRateLimiting: dc.GetBool(dynamicconfig.WorkerEnablePersistencePriorityRateLimiting),
		PersistenceDynamicRateLimitingParams:  dc.GetMap(dynamicconfig.WorkerPersistenceDynamicRateLimitingParams),
		PersistenceQPSBurstRatio:              dc.GetFloat(dynamicconfig.PersistenceQPSBurstRatio),
		OperatorRPSRatio:                      dc.GetFloat(dynamicconfig.OperatorRPSRatio),

		VisibilityPersistenceMaxReadQPS:   visibility.GetVisibilityPersistenceMaxReadQPS(dc),
		VisibilityPersistenceMaxWriteQPS:  visibility.GetVisibilityPersistenceMaxWriteQPS(dc),
		EnableReadFromSecondaryVisibility: visibility.GetEnableReadFromSecondaryVisibilityConfig(dc),
		VisibilityDisableOrderByClause:    dc.GetBoolByNamespace(dynamicconfig.VisibilityDisableOrderByClause),
		VisibilityEnableManualPagination:  dc.GetBoolByNamespace(dynamicconfig.VisibilityEnableManualPagination),
	}
	return config
}

// Start is called to start the service
func (s *Service) Start() {
	s.logger.Info(
		"worker starting",
		tag.ComponentWorker,
	)

	metrics.RestartCount.With(s.metricsHandler).Record(1)

	s.clusterMetadata.Start()
	s.namespaceRegistry.Start()

	s.membershipMonitor.Start()

	s.ensureSystemNamespaceExists(context.TODO())
	s.startScanner()

	if s.clusterMetadata.IsGlobalNamespaceEnabled() {
		s.startReplicator()
	}
	if s.config.EnableParentClosePolicyWorker() {
		s.startParentClosePolicyProcessor()
	}
	if s.config.EnableBatcher() {
		s.startBatcher()
	}

	s.workerManager.Start()
	s.perNamespaceWorkerManager.Start(
		// TODO: get these from fx instead of passing through Start
		s.hostInfo,
		s.workerServiceResolver,
	)

	s.logger.Info(
		"worker service started",
		tag.ComponentWorker,
		tag.Address(s.hostInfo.GetAddress()),
	)
}

// Stop is called to stop the service
func (s *Service) Stop() {
	s.scanner.Stop()
	s.perNamespaceWorkerManager.Stop()
	s.workerManager.Stop()
	s.namespaceRegistry.Stop()
	s.clusterMetadata.Stop()
	s.persistenceBean.Close()
	s.visibilityManager.Close()

	s.logger.Info(
		"worker service stopped",
		tag.ComponentWorker,
		tag.Address(s.hostInfo.GetAddress()),
	)
}

func (s *Service) startParentClosePolicyProcessor() {
	params := &parentclosepolicy.BootstrapParams{
		Config:           *s.config.ParentCloseCfg,
		SdkClientFactory: s.sdkClientFactory,
		MetricsHandler:   s.metricsHandler,
		Logger:           s.logger,
		ClientBean:       s.clientBean,
		CurrentCluster:   s.clusterMetadata.GetCurrentClusterName(),
	}
	processor := parentclosepolicy.New(params)
	if err := processor.Start(); err != nil {
		s.logger.Fatal(
			"error starting parentclosepolicy processor",
			tag.Error(err),
		)
	}
}

func (s *Service) startBatcher() {
	if err := batcher.New(
		s.metricsHandler,
		s.logger,
		s.sdkClientFactory,
		s.config.BatcherRPS,
		s.config.BatcherConcurrency,
	).Start(); err != nil {
		s.logger.Fatal(
			"error starting batcher worker",
			tag.Error(err),
		)
	}
}

func (s *Service) initScanner() error {
	currentCluster := s.clusterMetadata.GetCurrentClusterName()
	adminClient, err := s.clientBean.GetRemoteAdminClient(currentCluster)
	if err != nil {
		return err
	}
	s.scanner = scanner.New(
		s.logger,
		s.config.ScannerCfg,
		s.sdkClientFactory,
		s.metricsHandler,
		s.executionManager,
		s.metadataManager,
		s.visibilityManager,
		s.taskManager,
		s.historyClient,
		adminClient,
		s.matchingClient,
		s.namespaceRegistry,
		currentCluster,
	)
	return nil
}

func (s *Service) startScanner() {
	if err := s.scanner.Start(); err != nil {
		s.logger.Fatal(
			"error starting scanner",
			tag.Error(err),
		)
	}
}

func (s *Service) startReplicator() {
	msgReplicator := replicator.NewReplicator(
		s.clusterMetadata,
		s.clientBean,
		s.logger,
		s.metricsHandler,
		s.hostInfo,
		s.workerServiceResolver,
		s.namespaceReplicationQueue,
		s.namespaceReplicationTaskExecutor,
		s.matchingClient,
		s.namespaceRegistry,
	)
	msgReplicator.Start()
}

func (s *Service) ensureSystemNamespaceExists(
	ctx context.Context,
) {
	_, err := s.metadataManager.GetNamespace(ctx, &persistence.GetNamespaceRequest{Name: primitives.SystemLocalNamespace})
	switch err.(type) {
	case nil:
		// noop
	case *serviceerror.NamespaceNotFound:
		s.logger.Fatal(
			"temporal-system namespace does not exist",
			tag.Error(err),
		)
	default:
		s.logger.Fatal(
			"failed to verify if temporal system namespace exists",
			tag.Error(err),
		)
	}
}

// This is intended for use by integration tests only.
func (s *Service) RefreshPerNSWorkerManager() {
	s.perNamespaceWorkerManager.refreshAll()
}

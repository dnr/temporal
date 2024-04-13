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

package frontend

import (
	"net"
	"sync"
	"time"

	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/workflowservice/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/membership"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/persistence/client"
	"go.temporal.io/server/common/persistence/visibility"
	"go.temporal.io/server/common/persistence/visibility/manager"
)

// Config represents configuration for frontend service
type Config struct {
	NumHistoryShards                      int32
	PersistenceMaxQPS                     dynamicconfig.IntPropertyFn
	PersistenceGlobalMaxQPS               dynamicconfig.IntPropertyFn
	PersistenceNamespaceMaxQPS            dynamicconfig.IntPropertyFnWithNamespaceFilter
	PersistenceGlobalNamespaceMaxQPS      dynamicconfig.IntPropertyFnWithNamespaceFilter
	PersistencePerShardNamespaceMaxQPS    dynamicconfig.IntPropertyFnWithNamespaceFilter
	EnablePersistencePriorityRateLimiting dynamicconfig.BoolPropertyFn
	PersistenceDynamicRateLimitingParams  dynamicconfig.MapPropertyFn
	PersistenceQPSBurstRatio              dynamicconfig.FloatPropertyFn

	VisibilityPersistenceMaxReadQPS       dynamicconfig.IntPropertyFn
	VisibilityPersistenceMaxWriteQPS      dynamicconfig.IntPropertyFn
	VisibilityMaxPageSize                 dynamicconfig.IntPropertyFnWithNamespaceFilter
	EnableReadFromSecondaryVisibility     dynamicconfig.BoolPropertyFnWithNamespaceFilter
	VisibilityDisableOrderByClause        dynamicconfig.BoolPropertyFnWithNamespaceFilter
	VisibilityEnableManualPagination      dynamicconfig.BoolPropertyFnWithNamespaceFilter
	VisibilityAllowList                   dynamicconfig.BoolPropertyFnWithNamespaceFilter
	SuppressErrorSetSystemSearchAttribute dynamicconfig.BoolPropertyFnWithNamespaceFilter

	HistoryMaxPageSize                                                dynamicconfig.IntPropertyFnWithNamespaceFilter
	RPS                                                               dynamicconfig.IntPropertyFn
	GlobalRPS                                                         dynamicconfig.IntPropertyFn
	OperatorRPSRatio                                                  dynamicconfig.FloatPropertyFn
	NamespaceReplicationInducingAPIsRPS                               dynamicconfig.IntPropertyFn
	MaxNamespaceRPSPerInstance                                        dynamicconfig.IntPropertyFnWithNamespaceFilter
	MaxNamespaceBurstRatioPerInstance                                 dynamicconfig.FloatPropertyFnWithNamespaceFilter
	MaxConcurrentLongRunningRequestsPerInstance                       dynamicconfig.IntPropertyFnWithNamespaceFilter
	MaxGlobalConcurrentLongRunningRequests                            dynamicconfig.IntPropertyFnWithNamespaceFilter
	MaxNamespaceVisibilityRPSPerInstance                              dynamicconfig.IntPropertyFnWithNamespaceFilter
	MaxNamespaceVisibilityBurstRatioPerInstance                       dynamicconfig.FloatPropertyFnWithNamespaceFilter
	MaxNamespaceNamespaceReplicationInducingAPIsRPSPerInstance        dynamicconfig.IntPropertyFnWithNamespaceFilter
	MaxNamespaceNamespaceReplicationInducingAPIsBurstRatioPerInstance dynamicconfig.FloatPropertyFnWithNamespaceFilter
	GlobalNamespaceRPS                                                dynamicconfig.IntPropertyFnWithNamespaceFilter
	InternalFEGlobalNamespaceRPS                                      dynamicconfig.IntPropertyFnWithNamespaceFilter
	GlobalNamespaceVisibilityRPS                                      dynamicconfig.IntPropertyFnWithNamespaceFilter
	InternalFEGlobalNamespaceVisibilityRPS                            dynamicconfig.IntPropertyFnWithNamespaceFilter
	GlobalNamespaceNamespaceReplicationInducingAPIsRPS                dynamicconfig.IntPropertyFnWithNamespaceFilter
	MaxIDLengthLimit                                                  dynamicconfig.IntPropertyFn
	WorkerBuildIdSizeLimit                                            dynamicconfig.IntPropertyFn
	ReachabilityTaskQueueScanLimit                                    dynamicconfig.IntPropertyFn
	ReachabilityQueryBuildIdLimit                                     dynamicconfig.IntPropertyFn
	ReachabilityQuerySetDurationSinceDefault                          dynamicconfig.DurationPropertyFn
	DisallowQuery                                                     dynamicconfig.BoolPropertyFnWithNamespaceFilter
	ShutdownDrainDuration                                             dynamicconfig.DurationPropertyFn
	ShutdownFailHealthCheckDuration                                   dynamicconfig.DurationPropertyFn

	MaxBadBinaries dynamicconfig.IntPropertyFnWithNamespaceFilter

	// security protection settings
	DisableListVisibilityByFilter dynamicconfig.BoolPropertyFnWithNamespaceFilter

	// size limit system protection
	BlobSizeLimitError dynamicconfig.IntPropertyFnWithNamespaceFilter
	BlobSizeLimitWarn  dynamicconfig.IntPropertyFnWithNamespaceFilter

	ThrottledLogRPS dynamicconfig.IntPropertyFn

	// Namespace specific config
	EnableNamespaceNotActiveAutoForwarding dynamicconfig.BoolPropertyFnWithNamespaceFilter

	SearchAttributesNumberOfKeysLimit dynamicconfig.IntPropertyFnWithNamespaceFilter
	SearchAttributesSizeOfValueLimit  dynamicconfig.IntPropertyFnWithNamespaceFilter
	SearchAttributesTotalSizeLimit    dynamicconfig.IntPropertyFnWithNamespaceFilter

	// DefaultWorkflowRetryPolicy represents default values for unset fields on a Workflow's
	// specified RetryPolicy
	DefaultWorkflowRetryPolicy dynamicconfig.MapPropertyFnWithNamespaceFilter

	// VisibilityArchival system protection
	VisibilityArchivalQueryMaxPageSize dynamicconfig.IntPropertyFn

	// DEPRECATED
	SendRawWorkflowHistory dynamicconfig.BoolPropertyFnWithNamespaceFilter

	// DefaultWorkflowTaskTimeout the default workflow task timeout
	DefaultWorkflowTaskTimeout dynamicconfig.DurationPropertyFnWithNamespaceFilter

	// EnableServerVersionCheck disables periodic version checking performed by the frontend
	EnableServerVersionCheck dynamicconfig.BoolPropertyFn

	// EnableTokenNamespaceEnforcement enables enforcement that namespace in completion token matches namespace of the request
	EnableTokenNamespaceEnforcement dynamicconfig.BoolPropertyFn

	// gRPC keep alive options
	// If a client pings too frequently, terminate the connection.
	KeepAliveMinTime dynamicconfig.DurationPropertyFn
	//  Allow pings even when there are no active streams (RPCs)
	KeepAlivePermitWithoutStream dynamicconfig.BoolPropertyFn
	// Close the connection if a client is idle.
	KeepAliveMaxConnectionIdle dynamicconfig.DurationPropertyFn
	// Close the connection if it is too old.
	KeepAliveMaxConnectionAge dynamicconfig.DurationPropertyFn
	// Additive period after MaxConnectionAge after which the connection will be forcibly closed.
	KeepAliveMaxConnectionAgeGrace dynamicconfig.DurationPropertyFn
	// Ping the client if it is idle to ensure the connection is still active.
	KeepAliveTime dynamicconfig.DurationPropertyFn
	// Wait for the ping ack before assuming the connection is dead.
	KeepAliveTimeout dynamicconfig.DurationPropertyFn

	// RPS per every parallel delete executions activity.
	// Total RPS is equal to DeleteNamespaceDeleteActivityRPS * DeleteNamespaceConcurrentDeleteExecutionsActivities.
	// Default value is 100.
	DeleteNamespaceDeleteActivityRPS dynamicconfig.IntPropertyFn
	// Page size to read executions from visibility for delete executions activity.
	// Default value is 1000.
	DeleteNamespacePageSize dynamicconfig.IntPropertyFn
	// Number of pages before returning ContinueAsNew from delete executions activity.
	// Default value is 256.
	DeleteNamespacePagesPerExecution dynamicconfig.IntPropertyFn
	// Number of concurrent delete executions activities.
	// Must be not greater than 256 and number of worker cores in the cluster.
	// Default is 4.
	DeleteNamespaceConcurrentDeleteExecutionsActivities dynamicconfig.IntPropertyFn
	// Duration for how long namespace stays in database
	// after all namespace resources (i.e. workflow executions) are deleted.
	// Default is 0, means, namespace will be deleted immediately.
	DeleteNamespaceNamespaceDeleteDelay dynamicconfig.DurationPropertyFn

	// Enable schedule-related RPCs
	EnableSchedules dynamicconfig.BoolPropertyFnWithNamespaceFilter

	// Enable batcher RPCs
	EnableBatcher dynamicconfig.BoolPropertyFnWithNamespaceFilter
	// Batch operation dynamic configs
	MaxConcurrentBatchOperation     dynamicconfig.IntPropertyFnWithNamespaceFilter
	MaxExecutionCountBatchOperation dynamicconfig.IntPropertyFnWithNamespaceFilter

	EnableWorkflowIdConflictPolicy dynamicconfig.BoolPropertyFnWithNamespaceFilter

	EnableUpdateWorkflowExecution              dynamicconfig.BoolPropertyFnWithNamespaceFilter
	EnableUpdateWorkflowExecutionAsyncAccepted dynamicconfig.BoolPropertyFnWithNamespaceFilter

	EnableExecuteMultiOperation dynamicconfig.BoolPropertyFnWithNamespaceFilter

	EnableWorkerVersioningData     dynamicconfig.BoolPropertyFnWithNamespaceFilter
	EnableWorkerVersioningWorkflow dynamicconfig.BoolPropertyFnWithNamespaceFilter
	EnableWorkerVersioningRules    dynamicconfig.BoolPropertyFnWithNamespaceFilter

	// AccessHistoryFraction are interim flags across 2 minor releases and will be removed once fully enabled.
	AccessHistoryFraction            dynamicconfig.FloatPropertyFn
	AdminDeleteAccessHistoryFraction dynamicconfig.FloatPropertyFn

	// EnableNexusAPIs controls whether to allow invoking Nexus related APIs and whether to register a handler for Nexus
	// HTTP requests.
	EnableNexusAPIs dynamicconfig.BoolPropertyFn

	// EnableCallbackAttachment enables attaching callbacks to workflows.
	EnableCallbackAttachment    dynamicconfig.BoolPropertyFnWithNamespaceFilter
	AdminEnableListHistoryTasks dynamicconfig.BoolPropertyFn
}

// NewConfig returns new service config with default values
func NewConfig(
	dc *dynamicconfig.Collection,
	numHistoryShards int32,
) *Config {
	return &Config{
		NumHistoryShards:                      numHistoryShards,
		PersistenceMaxQPS:                     dc.GetInt(dynamicconfig.FrontendPersistenceMaxQPS),
		PersistenceGlobalMaxQPS:               dc.GetInt(dynamicconfig.FrontendPersistenceGlobalMaxQPS),
		PersistenceNamespaceMaxQPS:            dc.GetIntByNamespace(dynamicconfig.FrontendPersistenceNamespaceMaxQPS),
		PersistenceGlobalNamespaceMaxQPS:      dc.GetIntByNamespace(dynamicconfig.FrontendPersistenceGlobalNamespaceMaxQPS),
		PersistencePerShardNamespaceMaxQPS:    dynamicconfig.DefaultPerShardNamespaceRPSMax,
		EnablePersistencePriorityRateLimiting: dc.GetBool(dynamicconfig.FrontendEnablePersistencePriorityRateLimiting),
		PersistenceDynamicRateLimitingParams:  dc.GetMap(dynamicconfig.FrontendPersistenceDynamicRateLimitingParams),
		PersistenceQPSBurstRatio:              dc.GetFloat(dynamicconfig.PersistenceQPSBurstRatio),

		VisibilityPersistenceMaxReadQPS:       visibility.GetVisibilityPersistenceMaxReadQPS(dc),
		VisibilityPersistenceMaxWriteQPS:      visibility.GetVisibilityPersistenceMaxWriteQPS(dc),
		VisibilityMaxPageSize:                 dc.GetIntByNamespace(dynamicconfig.FrontendVisibilityMaxPageSize),
		EnableReadFromSecondaryVisibility:     visibility.GetEnableReadFromSecondaryVisibilityConfig(dc),
		VisibilityDisableOrderByClause:        dc.GetBoolByNamespace(dynamicconfig.VisibilityDisableOrderByClause),
		VisibilityEnableManualPagination:      dc.GetBoolByNamespace(dynamicconfig.VisibilityEnableManualPagination),
		VisibilityAllowList:                   dc.GetBoolByNamespace(dynamicconfig.VisibilityAllowList),
		SuppressErrorSetSystemSearchAttribute: dc.GetBoolByNamespace(dynamicconfig.SuppressErrorSetSystemSearchAttribute),

		HistoryMaxPageSize:                  dc.GetIntByNamespace(dynamicconfig.FrontendHistoryMaxPageSize),
		RPS:                                 dc.GetInt(dynamicconfig.FrontendRPS),
		GlobalRPS:                           dc.GetInt(dynamicconfig.FrontendGlobalRPS),
		OperatorRPSRatio:                    dc.GetFloat(dynamicconfig.OperatorRPSRatio),
		NamespaceReplicationInducingAPIsRPS: dc.GetInt(dynamicconfig.FrontendNamespaceReplicationInducingAPIsRPS),

		MaxNamespaceRPSPerInstance:                                        dc.GetIntByNamespace(dynamicconfig.FrontendMaxNamespaceRPSPerInstance),
		MaxNamespaceBurstRatioPerInstance:                                 dc.GetFloatByNamespace(dynamicconfig.FrontendMaxNamespaceBurstRatioPerInstance),
		MaxConcurrentLongRunningRequestsPerInstance:                       dc.GetIntByNamespace(dynamicconfig.FrontendMaxConcurrentLongRunningRequestsPerInstance),
		MaxGlobalConcurrentLongRunningRequests:                            dc.GetIntByNamespace(dynamicconfig.FrontendGlobalMaxConcurrentLongRunningRequests),
		MaxNamespaceVisibilityRPSPerInstance:                              dc.GetIntByNamespace(dynamicconfig.FrontendMaxNamespaceVisibilityRPSPerInstance),
		MaxNamespaceVisibilityBurstRatioPerInstance:                       dc.GetFloatByNamespace(dynamicconfig.FrontendMaxNamespaceVisibilityBurstRatioPerInstance),
		MaxNamespaceNamespaceReplicationInducingAPIsRPSPerInstance:        dc.GetIntByNamespace(dynamicconfig.FrontendMaxNamespaceNamespaceReplicationInducingAPIsRPSPerInstance),
		MaxNamespaceNamespaceReplicationInducingAPIsBurstRatioPerInstance: dc.GetFloatByNamespace(dynamicconfig.FrontendMaxNamespaceNamespaceReplicationInducingAPIsBurstRatioPerInstance),

		GlobalNamespaceRPS:                     dc.GetIntByNamespace(dynamicconfig.FrontendGlobalNamespaceRPS),
		InternalFEGlobalNamespaceRPS:           dc.GetIntByNamespace(dynamicconfig.InternalFrontendGlobalNamespaceRPS),
		GlobalNamespaceVisibilityRPS:           dc.GetIntByNamespace(dynamicconfig.FrontendGlobalNamespaceVisibilityRPS),
		InternalFEGlobalNamespaceVisibilityRPS: dc.GetIntByNamespace(dynamicconfig.InternalFrontendGlobalNamespaceVisibilityRPS),
		// Overshoot since these low rate limits don't work well in an uncoordinated global limiter.
		GlobalNamespaceNamespaceReplicationInducingAPIsRPS: dc.GetIntByNamespace(dynamicconfig.FrontendGlobalNamespaceNamespaceReplicationInducingAPIsRPS),
		MaxIDLengthLimit:                         dc.GetInt(dynamicconfig.MaxIDLengthLimit),
		WorkerBuildIdSizeLimit:                   dc.GetInt(dynamicconfig.WorkerBuildIdSizeLimit),
		ReachabilityTaskQueueScanLimit:           dc.GetInt(dynamicconfig.ReachabilityTaskQueueScanLimit),
		ReachabilityQueryBuildIdLimit:            dc.GetInt(dynamicconfig.ReachabilityQueryBuildIdLimit),
		ReachabilityQuerySetDurationSinceDefault: dc.GetDuration(dynamicconfig.ReachabilityQuerySetDurationSinceDefault),
		MaxBadBinaries:                           dc.GetIntByNamespace(dynamicconfig.FrontendMaxBadBinaries),
		DisableListVisibilityByFilter:            dc.GetBoolByNamespace(dynamicconfig.DisableListVisibilityByFilter),
		BlobSizeLimitError:                       dc.GetIntByNamespace(dynamicconfig.BlobSizeLimitError),
		BlobSizeLimitWarn:                        dc.GetIntByNamespace(dynamicconfig.BlobSizeLimitWarn),
		ThrottledLogRPS:                          dc.GetInt(dynamicconfig.FrontendThrottledLogRPS),
		ShutdownDrainDuration:                    dc.GetDuration(dynamicconfig.FrontendShutdownDrainDuration),
		ShutdownFailHealthCheckDuration:          dc.GetDuration(dynamicconfig.FrontendShutdownFailHealthCheckDuration),
		EnableNamespaceNotActiveAutoForwarding:   dc.GetBoolByNamespace(dynamicconfig.EnableNamespaceNotActiveAutoForwarding),
		SearchAttributesNumberOfKeysLimit:        dc.GetIntByNamespace(dynamicconfig.SearchAttributesNumberOfKeysLimit),
		SearchAttributesSizeOfValueLimit:         dc.GetIntByNamespace(dynamicconfig.SearchAttributesSizeOfValueLimit),
		SearchAttributesTotalSizeLimit:           dc.GetIntByNamespace(dynamicconfig.SearchAttributesTotalSizeLimit),
		VisibilityArchivalQueryMaxPageSize:       dc.GetInt(dynamicconfig.VisibilityArchivalQueryMaxPageSize),
		DisallowQuery:                            dc.GetBoolByNamespace(dynamicconfig.DisallowQuery),
		SendRawWorkflowHistory:                   dc.GetBoolByNamespace(dynamicconfig.SendRawWorkflowHistory),
		DefaultWorkflowRetryPolicy:               dc.GetMapByNamespace(dynamicconfig.DefaultWorkflowRetryPolicy),
		DefaultWorkflowTaskTimeout:               dc.GetDurationByNamespace(dynamicconfig.DefaultWorkflowTaskTimeout),
		EnableServerVersionCheck:                 dc.GetBool(dynamicconfig.EnableServerVersionCheck),
		EnableTokenNamespaceEnforcement:          dc.GetBool(dynamicconfig.EnableTokenNamespaceEnforcement),
		KeepAliveMinTime:                         dc.GetDuration(dynamicconfig.KeepAliveMinTime),
		KeepAlivePermitWithoutStream:             dc.GetBool(dynamicconfig.KeepAlivePermitWithoutStream),
		KeepAliveMaxConnectionIdle:               dc.GetDuration(dynamicconfig.KeepAliveMaxConnectionIdle),
		KeepAliveMaxConnectionAge:                dc.GetDuration(dynamicconfig.KeepAliveMaxConnectionAge),
		KeepAliveMaxConnectionAgeGrace:           dc.GetDuration(dynamicconfig.KeepAliveMaxConnectionAgeGrace),
		KeepAliveTime:                            dc.GetDuration(dynamicconfig.KeepAliveTime),
		KeepAliveTimeout:                         dc.GetDuration(dynamicconfig.KeepAliveTimeout),

		DeleteNamespaceDeleteActivityRPS:                    dc.GetInt(dynamicconfig.DeleteNamespaceDeleteActivityRPS),
		DeleteNamespacePageSize:                             dc.GetInt(dynamicconfig.DeleteNamespacePageSize),
		DeleteNamespacePagesPerExecution:                    dc.GetInt(dynamicconfig.DeleteNamespacePagesPerExecution),
		DeleteNamespaceConcurrentDeleteExecutionsActivities: dc.GetInt(dynamicconfig.DeleteNamespaceConcurrentDeleteExecutionsActivities),
		DeleteNamespaceNamespaceDeleteDelay:                 dc.GetDuration(dynamicconfig.DeleteNamespaceNamespaceDeleteDelay),

		EnableSchedules: dc.GetBoolByNamespace(dynamicconfig.FrontendEnableSchedules),

		EnableBatcher:                   dc.GetBoolByNamespace(dynamicconfig.FrontendEnableBatcher),
		MaxConcurrentBatchOperation:     dc.GetIntByNamespace(dynamicconfig.FrontendMaxConcurrentBatchOperationPerNamespace),
		MaxExecutionCountBatchOperation: dc.GetIntByNamespace(dynamicconfig.FrontendMaxExecutionCountBatchOperationPerNamespace),

		EnableWorkflowIdConflictPolicy: dc.GetBoolByNamespace(dynamicconfig.EnableWorkflowIdConflictPolicy),

		EnableExecuteMultiOperation: dc.GetBoolByNamespace(dynamicconfig.FrontendEnableExecuteMultiOperation),

		EnableUpdateWorkflowExecution:              dc.GetBoolByNamespace(dynamicconfig.FrontendEnableUpdateWorkflowExecution),
		EnableUpdateWorkflowExecutionAsyncAccepted: dc.GetBoolByNamespace(dynamicconfig.FrontendEnableUpdateWorkflowExecutionAsyncAccepted),

		EnableWorkerVersioningData:     dc.GetBoolByNamespace(dynamicconfig.FrontendEnableWorkerVersioningDataAPIs),
		EnableWorkerVersioningWorkflow: dc.GetBoolByNamespace(dynamicconfig.FrontendEnableWorkerVersioningWorkflowAPIs),
		EnableWorkerVersioningRules:    dc.GetBoolByNamespace(dynamicconfig.FrontendEnableWorkerVersioningRuleAPIs),

		AccessHistoryFraction:            dc.GetFloat(dynamicconfig.FrontendAccessHistoryFraction),
		AdminDeleteAccessHistoryFraction: dc.GetFloat(dynamicconfig.FrontendAdminDeleteAccessHistoryFraction),

		EnableNexusAPIs:             dc.GetBool(dynamicconfig.FrontendEnableNexusAPIs),
		EnableCallbackAttachment:    dc.GetBoolByNamespace(dynamicconfig.FrontendEnableCallbackAttachment),
		AdminEnableListHistoryTasks: dc.GetBool(dynamicconfig.AdminEnableListHistoryTasks),
	}
}

// Service represents the frontend service
type Service struct {
	config *Config

	healthServer      *health.Server
	handler           Handler
	adminHandler      *AdminHandler
	operatorHandler   *OperatorHandlerImpl
	versionChecker    *VersionChecker
	visibilityManager manager.VisibilityManager
	server            *grpc.Server
	httpAPIServer     *HTTPAPIServer

	logger                         log.Logger
	grpcListener                   net.Listener
	metricsHandler                 metrics.Handler
	faultInjectionDataStoreFactory *client.FaultInjectionDataStoreFactory
	membershipMonitor              membership.Monitor
}

func NewService(
	serviceConfig *Config,
	server *grpc.Server,
	healthServer *health.Server,
	httpAPIServer *HTTPAPIServer,
	handler Handler,
	adminHandler *AdminHandler,
	operatorHandler *OperatorHandlerImpl,
	versionChecker *VersionChecker,
	visibilityMgr manager.VisibilityManager,
	logger log.Logger,
	grpcListener net.Listener,
	metricsHandler metrics.Handler,
	faultInjectionDataStoreFactory *client.FaultInjectionDataStoreFactory,
	membershipMonitor membership.Monitor,
) *Service {
	return &Service{
		config:                         serviceConfig,
		server:                         server,
		healthServer:                   healthServer,
		httpAPIServer:                  httpAPIServer,
		handler:                        handler,
		adminHandler:                   adminHandler,
		operatorHandler:                operatorHandler,
		versionChecker:                 versionChecker,
		visibilityManager:              visibilityMgr,
		logger:                         logger,
		grpcListener:                   grpcListener,
		metricsHandler:                 metricsHandler,
		faultInjectionDataStoreFactory: faultInjectionDataStoreFactory,
		membershipMonitor:              membershipMonitor,
	}
}

// Start starts the service
func (s *Service) Start() {
	s.logger.Info("frontend starting")

	healthpb.RegisterHealthServer(s.server, s.healthServer)
	workflowservice.RegisterWorkflowServiceServer(s.server, s.handler)
	adminservice.RegisterAdminServiceServer(s.server, s.adminHandler)
	operatorservice.RegisterOperatorServiceServer(s.server, s.operatorHandler)

	reflection.Register(s.server)

	// must start resource first
	metrics.RestartCount.With(s.metricsHandler).Record(1)

	s.versionChecker.Start()
	s.adminHandler.Start()
	s.operatorHandler.Start()
	s.handler.Start()

	go func() {
		s.logger.Info("Starting to serve on frontend listener")
		if err := s.server.Serve(s.grpcListener); err != nil {
			s.logger.Fatal("Failed to serve on frontend listener", tag.Error(err))
		}
	}()

	if s.httpAPIServer != nil {
		go func() {
			if err := s.httpAPIServer.Serve(); err != nil {
				s.logger.Fatal("Failed to serve HTTP API server", tag.Error(err))
			}
		}()
	}

	go s.membershipMonitor.Start()
}

// Stop stops the service
func (s *Service) Stop() {
	// initiate graceful shutdown:
	// 1. Fail rpc health check, this will cause client side load balancer to stop forwarding requests to this node
	// 2. wait for failure detection time
	// 3. stop taking new requests by returning InternalServiceError
	// 4. Wait for X second
	// 5. Stop everything forcefully and return

	requestDrainTime := max(time.Second, s.config.ShutdownDrainDuration())
	failureDetectionTime := max(0, s.config.ShutdownFailHealthCheckDuration())

	s.logger.Info("ShutdownHandler: Updating gRPC health status to ShuttingDown")
	s.healthServer.Shutdown()
	s.membershipMonitor.SetDraining(true)

	s.logger.Info("ShutdownHandler: Waiting for others to discover I am unhealthy")
	time.Sleep(failureDetectionTime)

	s.handler.Stop()
	s.operatorHandler.Stop()
	s.adminHandler.Stop()
	s.versionChecker.Stop()
	s.visibilityManager.Close()

	s.logger.Info("ShutdownHandler: Draining traffic")
	// Gracefully stop gRPC server and HTTP API server concurrently
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.AfterFunc(requestDrainTime, func() {
			s.logger.Info("ShutdownHandler: Drain time expired, stopping all traffic")
			s.server.Stop()
		})
		s.server.GracefulStop()
		t.Stop()
	}()
	if s.httpAPIServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.httpAPIServer.GracefulStop(requestDrainTime)
		}()
	}
	wg.Wait()

	if s.metricsHandler != nil {
		s.metricsHandler.Stop(s.logger)
	}

	s.logger.Info("frontend stopped")
}

func (s *Service) GetFaultInjection() *client.FaultInjectionDataStoreFactory {
	return s.faultInjectionDataStoreFactory
}

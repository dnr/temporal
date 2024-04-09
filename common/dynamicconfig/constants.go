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

package dynamicconfig

import (
	"os"
	"time"

	"go.temporal.io/server/common"
	"go.temporal.io/server/common/namespace"
)

var (
	// keys for admin

	AdminEnableListHistoryTasks = &BoolGlobalSetting{
		Key:         "admin.enableListHistoryTasks",
		Default:     true,
		Description: `AdminEnableListHistoryTasks is the key for enabling listing history tasks`,
	}
	AdminMatchingNamespaceToPartitionDispatchRate = &FloatNamespaceSetting{
		Key:         "admin.matchingNamespaceToPartitionDispatchRate",
		Default:     10000,
		Description: `AdminMatchingNamespaceToPartitionDispatchRate is the max qps of any task queue partition for a given namespace`,
	}
	AdminMatchingNamespaceTaskqueueToPartitionDispatchRate = &FloatTaskQueueInfoSetting{
		Key:         "admin.matchingNamespaceTaskqueueToPartitionDispatchRate",
		Default:     1000,
		Description: `AdminMatchingNamespaceTaskqueueToPartitionDispatchRate is the max qps of a task queue partition for a given namespace & task queue`,
	}

	// keys for system

	VisibilityPersistenceMaxReadQPS = &IntGlobalSetting{
		Key:         "system.visibilityPersistenceMaxReadQPS",
		Default:     9000,
		Description: `VisibilityPersistenceMaxReadQPS is the max QPC system host can query visibility DB for read.`,
	}
	VisibilityPersistenceMaxWriteQPS = &IntGlobalSetting{
		Key:         "system.visibilityPersistenceMaxWriteQPS",
		Default:     9000,
		Description: `VisibilityPersistenceMaxWriteQPS is the max QPC system host can query visibility DB for write.`,
	}
	// FIXME
	// // EnableReadFromSecondaryVisibility is the config to enable read from secondary visibility
	// EnableReadFromSecondaryVisibility = "system.enableReadFromSecondaryVisibility"
	// // SecondaryVisibilityWritingMode is key for how to write to secondary visibility
	// SecondaryVisibilityWritingMode = "system.secondaryVisibilityWritingMode"
	VisibilityDisableOrderByClause = &BoolNamespaceSetting{
		Key:         "system.visibilityDisableOrderByClause",
		Default:     true,
		Description: `VisibilityDisableOrderByClause is the config to disable ORDERY BY clause for Elasticsearch`,
	}
	VisibilityEnableManualPagination = &BoolNamespaceSetting{
		Key:         "system.visibilityEnableManualPagination",
		Default:     true,
		Description: `VisibilityEnableManualPagination is the config to enable manual pagination for Elasticsearch`,
	}
	VisibilityAllowList = &BoolNamespaceSetting{
		Key:         "system.visibilityAllowList",
		Default:     true,
		Description: `VisibilityAllowList is the config to allow list of values for regular types`,
	}
	SuppressErrorSetSystemSearchAttribute = &BoolNamespaceSetting{
		Key:     "system.suppressErrorSetSystemSearchAttribute",
		Default: false,
		Description: `SuppressErrorSetSystemSearchAttribute suppresses errors when trying to set
values in system search attributes.`,
	}

	HistoryArchivalState = &StringGlobalSetting{
		Key:         "system.historyArchivalState",
		Default:     historyState,
		Description: `HistoryArchivalState is key for the state of history archival`,
	}
	EnableReadFromHistoryArchival = &BoolGlobalSetting{
		Key:         "system.enableReadFromHistoryArchival",
		Default:     historyReadEnabled,
		Description: `EnableReadFromHistoryArchival is key for enabling reading history from archival store`,
	}
	VisibilityArchivalState = &StringGlobalSetting{
		Key:         "system.visibilityArchivalState",
		Default:     visibilityState,
		Description: `VisibilityArchivalState is key for the state of visibility archival`,
	}
	EnableReadFromVisibilityArchival = &BoolGlobalSetting{
		Key:         "system.enableReadFromVisibilityArchival",
		Default:     visibilityReadEnabled,
		Description: `EnableReadFromVisibilityArchival is key for enabling reading visibility from archival store`,
	}
	EnableNamespaceNotActiveAutoForwarding = &BoolNamespaceSetting{
		Key:     "system.enableNamespaceNotActiveAutoForwarding",
		Default: true,
		Description: `EnableNamespaceNotActiveAutoForwarding whether enabling DC auto forwarding to active cluster
for signal / start / signal with start API if namespace is not active`,
	}
	TransactionSizeLimit = &IntGlobalSetting{
		Key:         "system.transactionSizeLimit",
		Default:     common.DefaultTransactionSizeLimit,
		Description: `TransactionSizeLimit is the largest allowed transaction size to persistence`,
	}
	DisallowQuery = &BoolNamespaceSetting{
		Key:         "system.disallowQuery",
		Default:     false,
		Description: `DisallowQuery is the key to disallow query for a namespace`,
	}
	// FIXME: unused?
	// // EnableAuthorization is the key to enable authorization for a namespace
	// EnableAuthorization = "system.enableAuthorization"
	EnableCrossNamespaceCommands = &BoolGlobalSetting{
		Key:         "system.enableCrossNamespaceCommands",
		Default:     true,
		Description: `EnableCrossNamespaceCommands is the key to enable commands for external namespaces`,
	}
	ClusterMetadataRefreshInterval = &DurationGlobalSetting{
		Key:         "system.clusterMetadataRefreshInterval",
		Default:     refreshInterval,
		Description: `ClusterMetadataRefreshInterval is config to manage cluster metadata table refresh interval`,
	}
	ForceSearchAttributesCacheRefreshOnRead = &BoolGlobalSetting{
		Key:     "system.forceSearchAttributesCacheRefreshOnRead",
		Default: false,
		Description: `ForceSearchAttributesCacheRefreshOnRead forces refreshing search attributes cache on a read operation, so we always
get the latest data from DB. This effectively bypasses cache value and is used to facilitate testing of changes in
search attributes. This should not be turned on in production.`,
	}
	EnableRingpopTLS = &BoolGlobalSetting{
		Key:     "system.enableRingpopTLS",
		Default: false,
		Description: `EnableRingpopTLS controls whether to use TLS for ringpop, using the same "internode" TLS
config as the other services.`,
	}
	RingpopApproximateMaxPropagationTime = &DurationGlobalSetting{
		Key:     "system.ringpopApproximateMaxPropagationTime",
		Default: 3 * time.Second,
		Description: `RingpopApproximateMaxPropagationTime is used for timing certain startup and shutdown processes.
(It is not and doesn't have to be a guarantee.)`,
	}
	EnableParentClosePolicyWorker = &BoolGlobalSetting{
		Key:         "system.enableParentClosePolicyWorker",
		Default:     true,
		Description: `EnableParentClosePolicyWorker decides whether or not enable system workers for processing parent close policy task`,
	}
	EnableStickyQuery = &BoolNamespaceSetting{
		Key:         "system.enableStickyQuery",
		Default:     true,
		Description: `EnableStickyQuery indicates if sticky query should be enabled per namespace`,
	}
	EnableActivityEagerExecution = &BoolNamespaceSetting{
		Key:         "system.enableActivityEagerExecution",
		Default:     false,
		Description: `EnableActivityEagerExecution indicates if activity eager execution is enabled per namespace`,
	}
	EnableEagerWorkflowStart = &BoolNamespaceSetting{
		Key:     "system.enableEagerWorkflowStart",
		Default: false,
		Description: `EnableEagerWorkflowStart toggles "eager workflow start" - returning the first workflow task inline in the
response to a StartWorkflowExecution request and skipping the trip through matching.`,
	}
	NamespaceCacheRefreshInterval = &DurationGlobalSetting{
		Key:         "system.namespaceCacheRefreshInterval",
		Default:     10 * time.Second,
		Description: `NamespaceCacheRefreshInterval is the key for namespace cache refresh interval dynamic config`,
	}
	PersistenceHealthSignalMetricsEnabled = &BoolGlobalSetting{
		Key:         "system.persistenceHealthSignalMetricsEnabled",
		Default:     true,
		Description: `PersistenceHealthSignalMetricsEnabled determines whether persistence shard RPS metrics are emitted`,
	}
	PersistenceHealthSignalAggregationEnabled = &BoolGlobalSetting{
		Key:         "system.persistenceHealthSignalAggregationEnabled",
		Default:     true,
		Description: `PersistenceHealthSignalAggregationEnabled determines whether persistence latency and error averages are tracked`,
	}
	PersistenceHealthSignalWindowSize = &DurationGlobalSetting{
		Key:         "system.persistenceHealthSignalWindowSize",
		Default:     10 * time.Second,
		Description: `PersistenceHealthSignalWindowSize is the time window size in seconds for aggregating persistence signals`,
	}
	PersistenceHealthSignalBufferSize = &IntGlobalSetting{
		Key:         "system.persistenceHealthSignalBufferSize",
		Default:     5000,
		Description: `PersistenceHealthSignalBufferSize is the maximum number of persistence signals to buffer in memory per signal key`,
	}
	ShardRPSWarnLimit = &IntGlobalSetting{
		Key:         "system.shardRPSWarnLimit",
		Default:     50,
		Description: `ShardRPSWarnLimit is the per-shard RPS limit for warning`,
	}
	ShardPerNsRPSWarnPercent = &Float64GlobalSetting{
		Key:     "system.shardPerNsRPSWarnPercent",
		Default: 0.8,
		Description: `ShardPerNsRPSWarnPercent is the per-shard per-namespace RPS limit for warning as a percentage of ShardRPSWarnLimit
these warning are not emitted if the value is set to 0 or less`,
	}
	OperatorRPSRatio = &Float64GlobalSetting{
		Key:     "system.operatorRPSRatio",
		Default: common.DefaultOperatorRPSRatio,
		Description: `OperatorRPSRatio is the percentage of the rate limit provided to priority rate limiters that should be used for
operator API calls (highest priority). Should be >0.0 and <= 1.0 (defaults to 20% if not specified)`,
	}

	// deadlock detector

	DeadlockDumpGoroutines = &BoolGlobalSetting{
		Key:         "system.deadlock.DumpGoroutines",
		Default:     true,
		Description: `Whether the deadlock detector should dump goroutines`,
	}
	DeadlockFailHealthCheck = &BoolGlobalSetting{
		Key:         "system.deadlock.FailHealthCheck",
		Default:     false,
		Description: `Whether the deadlock detector should cause the grpc server to fail health checks`,
	}
	DeadlockAbortProcess = &BoolGlobalSetting{
		Key:         "system.deadlock.AbortProcess",
		Default:     false,
		Description: `Whether the deadlock detector should abort the process`,
	}
	DeadlockInterval = &DurationGlobalSetting{
		Key:         "system.deadlock.Interval",
		Default:     30 * time.Second,
		Description: `How often the detector checks each root.`,
	}
	DeadlockMaxWorkersPerRoot = &IntGlobalSetting{
		Key:         "system.deadlock.MaxWorkersPerRoot",
		Default:     10,
		Description: `How many extra goroutines can be created per root.`,
	}

	// utf-8 validation

	ValidateUTF8SampleRPCRequest = &Float64GlobalSetting{
		Key:         "system.validateUTF8.sample.rpcRequest",
		Default:     0.0,
		Description: `Sample rate of utf-8 string validation for rpc requests`,
	}
	ValidateUTF8SampleRPCResponse = &Float64GlobalSetting{
		Key:         "system.validateUTF8.sample.rpcResponse",
		Default:     0.0,
		Description: `Sample rate of utf-8 string validation for rpc responses`,
	}
	ValidateUTF8SamplePersistence = &Float64GlobalSetting{
		Key:         "system.validateUTF8.sample.persistence",
		Default:     0.0,
		Description: `Sample rate of utf-8 string validation for persistence [de]serialization`,
	}
	ValidateUTF8FailRPCRequest = &BoolGlobalSetting{
		Key:         "system.validateUTF8.fail.rpcRequest",
		Default:     false,
		Description: `Whether to fail rpcs on utf-8 string validation errors`,
	}
	ValidateUTF8FailRPCResponse = &BoolGlobalSetting{
		Key:         "system.validateUTF8.fail.rpcResponse",
		Default:     false,
		Description: `Whether to fail rpcs on utf-8 string validation errors`,
	}
	ValidateUTF8FailPersistence = &BoolGlobalSetting{
		Key:         "system.validateUTF8.fail.persistence",
		Default:     false,
		Description: `Whether to fail persistence [de]serialization on utf-8 string validation errors`,
	}

	// keys for size limit

	BlobSizeLimitError = &IntNamespaceSetting{
		Key:         "limit.blobSize.error",
		Default:     2 * 1024 * 1024,
		Description: `BlobSizeLimitError is the per event blob size limit`,
	}
	BlobSizeLimitWarn = &IntNamespaceSetting{
		Key:         "limit.blobSize.warn",
		Default:     512 * 1024,
		Description: `BlobSizeLimitWarn is the per event blob size limit for warning`,
	}
	MemoSizeLimitError = &IntNamespaceSetting{
		Key:         "limit.memoSize.error",
		Default:     2 * 1024 * 1024,
		Description: `MemoSizeLimitError is the per event memo size limit`,
	}
	MemoSizeLimitWarn = &IntNamespaceSetting{
		Key:         "limit.memoSize.warn",
		Default:     2 * 1024,
		Description: `MemoSizeLimitWarn is the per event memo size limit for warning`,
	}
	NumPendingChildExecutionsLimitError = &IntNamespaceSetting{
		Key:     "limit.numPendingChildExecutions.error",
		Default: 2000,
		Description: `NumPendingChildExecutionsLimitError is the maximum number of pending child workflows a workflow can have before
StartChildWorkflowExecution commands will fail.`,
	}
	NumPendingActivitiesLimitError = &IntNamespaceSetting{
		Key:     "limit.numPendingActivities.error",
		Default: 2000,
		Description: `NumPendingActivitiesLimitError is the maximum number of pending activities a workflow can have before
ScheduleActivityTask will fail.`,
	}
	NumPendingSignalsLimitError = &IntNamespaceSetting{
		Key:     "limit.numPendingSignals.error",
		Default: 2000,
		Description: `NumPendingSignalsLimitError is the maximum number of pending signals a workflow can have before
SignalExternalWorkflowExecution commands from this workflow will fail.`,
	}
	NumPendingCancelRequestsLimitError = &IntNamespaceSetting{
		Key:     "limit.numPendingCancelRequests.error",
		Default: 2000,
		Description: `NumPendingCancelRequestsLimitError is the maximum number of pending requests to cancel other workflows a workflow can have before
RequestCancelExternalWorkflowExecution commands will fail.`,
	}
	HistorySizeLimitError = &IntNamespaceSetting{
		Key:         "limit.historySize.error",
		Default:     50 * 1024 * 1024,
		Description: `HistorySizeLimitError is the per workflow execution history size limit`,
	}
	HistorySizeLimitWarn = &IntNamespaceSetting{
		Key:         "limit.historySize.warn",
		Default:     10 * 1024 * 1024,
		Description: `HistorySizeLimitWarn is the per workflow execution history size limit for warning`,
	}
	HistorySizeSuggestContinueAsNew = &IntNamespaceSetting{
		Key:     "limit.historySize.suggestContinueAsNew",
		Default: 4 * 1024 * 1024,
		Description: `HistorySizeSuggestContinueAsNew is the workflow execution history size limit to suggest
continue-as-new (in workflow task started event)`,
	}
	HistoryCountLimitError = &IntNamespaceSetting{
		Key:         "limit.historyCount.error",
		Default:     50 * 1024,
		Description: `HistoryCountLimitError is the per workflow execution history event count limit`,
	}
	HistoryCountLimitWarn = &IntNamespaceSetting{
		Key:         "limit.historyCount.warn",
		Default:     10 * 1024,
		Description: `HistoryCountLimitWarn is the per workflow execution history event count limit for warning`,
	}
	MutableStateActivityFailureSizeLimitError = &IntNamespaceSetting{
		Key:     "limit.mutableStateActivityFailureSize.error",
		Default: 4 * 1024,
		Description: `MutableStateActivityFailureSizeLimitError is the per activity failure size limit for workflow mutable state.
If exceeded, failure will be truncated before being stored in mutable state.`,
	}
	MutableStateActivityFailureSizeLimitWarn = &IntNamespaceSetting{
		Key:         "limit.mutableStateActivityFailureSize.warn",
		Default:     2 * 1024,
		Description: `MutableStateActivityFailureSizeLimitWarn is the per activity failure size warning limit for workflow mutable state`,
	}
	MutableStateSizeLimitError = &IntGlobalSetting{
		Key:         "limit.mutableStateSize.error",
		Default:     8 * 1024 * 1024,
		Description: `MutableStateSizeLimitError is the per workflow execution mutable state size limit in bytes`,
	}
	MutableStateSizeLimitWarn = &IntGlobalSetting{
		Key:         "limit.mutableStateSize.warn",
		Default:     1 * 1024 * 1024,
		Description: `MutableStateSizeLimitWarn is the per workflow execution mutable state size limit in bytes for warning`,
	}
	HistoryCountSuggestContinueAsNew = &IntNamespaceSetting{
		Key:     "limit.historyCount.suggestContinueAsNew",
		Default: 4 * 1024,
		Description: `HistoryCountSuggestContinueAsNew is the workflow execution history event count limit to
suggest continue-as-new (in workflow task started event)`,
	}
	HistoryMaxPageSize = &IntNamespaceSetting{
		Key:         "limit.historyMaxPageSize",
		Default:     common.GetHistoryMaxPageSize,
		Description: `HistoryMaxPageSize is default max size for GetWorkflowExecutionHistory in one page`,
	}
	MaxIDLengthLimit = &IntGlobalSetting{
		Key:     "limit.maxIDLength",
		Default: 1000,
		Description: `MaxIDLengthLimit is the length limit for various IDs, including: Namespace, TaskQueue, WorkflowID, ActivityID, TimerID,
WorkflowType, ActivityType, SignalName, MarkerName, ErrorReason/FailureReason/CancelCause, Identity, RequestID`,
	}
	WorkerBuildIdSizeLimit = &IntGlobalSetting{
		Key:     "limit.workerBuildIdSize",
		Default: 255,
		Description: `WorkerBuildIdSizeLimit is the byte length limit for a worker build id as used in the rpc methods for updating
the version sets for a task queue.
Do not set this to a value higher than 255 for clusters using SQL based persistence due to predefined VARCHAR
column width.`,
	}
	VersionCompatibleSetLimitPerQueue = &IntNamespaceSetting{
		Key:     "limit.versionCompatibleSetLimitPerQueue",
		Default: 10,
		Description: `VersionCompatibleSetLimitPerQueue is the max number of compatible sets allowed in the versioning data for a task
queue. Update requests which would cause the versioning data to exceed this number will fail with a
FailedPrecondition error.`,
	}
	VersionBuildIdLimitPerQueue = &IntNamespaceSetting{
		Key:     "limit.versionBuildIdLimitPerQueue",
		Default: 100,
		Description: `VersionBuildIdLimitPerQueue is the max number of build IDs allowed to be defined in the versioning data for a
task queue. Update requests which would cause the versioning data to exceed this number will fail with a
FailedPrecondition error.`,
	}
	ReachabilityTaskQueueScanLimit = &IntGlobalSetting{
		Key:     "limit.reachabilityTaskQueueScan",
		Default: 20,
		Description: `ReachabilityTaskQueueScanLimit limits the number of task queues to scan when responding to a
GetWorkerTaskReachability query.`,
	}
	ReachabilityQueryBuildIdLimit = &IntGlobalSetting{
		Key:     "limit.reachabilityQueryBuildIds",
		Default: 5,
		Description: `ReachabilityQueryBuildIdLimit limits the number of build ids that can be requested in a single call to the
GetWorkerTaskReachability API.`,
	}
	ReachabilityQuerySetDurationSinceDefault = &DurationGlobalSetting{
		Key:     "frontend.reachabilityQuerySetDurationSinceDefault",
		Default: 5 * time.Minute,
		Description: `ReachabilityQuerySetDurationSinceDefault is the minimum period since a version set was demoted from being the
queue default before it is considered unreachable by new workflows.
This setting allows some propogation delay of versioning data for the reachability queries, which may happen for
the following reasons:
1. There are no workflows currently marked as open in the visibility store but a worker for the demoted version
is currently processing a task.
2. There are delays in the visibility task processor (which is asynchronous).
3. There's propagation delay of the versioning data between matching nodes.`,
	}
	TaskQueuesPerBuildIdLimit = &IntNamespaceSetting{
		Key:         "limit.taskQueuesPerBuildId",
		Default:     20,
		Description: `TaskQueuesPerBuildIdLimit limits the number of task queue names that can be mapped to a single build id.`,
	}

	NexusIncomingServiceNameMaxLength = &IntGlobalSetting{
		Key:         "limit.incomingServiceNameMaxLength",
		Default:     200,
		Description: `NexusIncomingServiceNameMaxLength is the maximum length of a Nexus incoming service name.`,
	}
	NexusIncomingServiceMaxSize = &IntGlobalSetting{
		Key:         "limit.incomingServiceMaxSize",
		Default:     4 * 1024,
		Description: `NexusIncomingServiceMaxSize is the maximum size of a Nexus incoming service in bytes.`,
	}
	NexusIncomingServiceListDefaultPageSize = &IntGlobalSetting{
		Key:         "limit.incomingServiceListDefaultPageSize",
		Default:     100,
		Description: `NexusIncomingServiceListDefaultPageSize is the default page size for listing Nexus incoming services.`,
	}
	NexusIncomingServiceListMaxPageSize = &IntGlobalSetting{
		Key:         "limit.incomingServiceListMaxPageSize",
		Default:     1000,
		Description: `NexusIncomingServiceListMaxPageSize is the maximum page size for listing Nexus incoming services.`,
	}
	NexusOutgoingServiceURLMaxLength = &IntGlobalSetting{
		Key:         "limit.outgoingServiceURLMaxLength",
		Default:     1000,
		Description: `NexusOutgoingServiceURLMaxLength is the maximum length of an outgoing service URL.`,
	}
	NexusOutgoingServiceNameMaxLength = &IntGlobalSetting{
		Key:         "limit.outgoingServiceNameMaxLength",
		Default:     200,
		Description: `NexusOutgoingServiceNameMaxLength is the maximum length of an outgoing service name.`,
	}
	NexusOutgoingServiceListDefaultPageSize = &IntGlobalSetting{
		Key:         "limit.outgoingServiceListDefaultPageSize",
		Default:     100,
		Description: `NexusOutgoingServiceListDefaultPageSize is the default page size for listing outgoing services.`,
	}
	NexusOutgoingServiceListMaxPageSize = &IntGlobalSetting{
		Key:         "limit.outgoingServiceListMaxPageSize",
		Default:     1000,
		Description: `NexusOutgoingServiceListMaxPageSize is the maximum page size for listing outgoing services.`,
	}

	RemovableBuildIdDurationSinceDefault = &DurationGlobalSetting{
		Key:     "worker.removableBuildIdDurationSinceDefault",
		Default: time.Hour,
		Description: `RemovableBuildIdDurationSinceDefault is the minimum duration since a build id was last default in its containing
set for it to be considered for removal, used by the build id scavenger.
This setting allows some propogation delay of versioning data, which may happen for the following reasons:
1. There are no workflows currently marked as open in the visibility store but a worker for the demoted version
is currently processing a task.
2. There are delays in the visibility task processor (which is asynchronous).
3. There's propagation delay of the versioning data between matching nodes.`,
	}
	BuildIdScavenengerVisibilityRPS = &Float64GlobalSetting{
		Key:         "worker.buildIdScavengerVisibilityRPS",
		Default:     1.0,
		Description: `BuildIdScavengerVisibilityRPS is the rate limit for visibility calls from the build id scavenger`,
	}

	// keys for frontend

	FrontendPersistenceMaxQPS = &IntGlobalSetting{
		Key:         "frontend.persistenceMaxQPS",
		Default:     2000,
		Description: `FrontendPersistenceMaxQPS is the max qps frontend host can query DB`,
	}
	FrontendPersistenceGlobalMaxQPS = &IntGlobalSetting{
		Key:         "frontend.persistenceGlobalMaxQPS",
		Default:     0,
		Description: `FrontendPersistenceGlobalMaxQPS is the max qps frontend cluster can query DB`,
	}
	FrontendPersistenceNamespaceMaxQPS = &IntNamespaceSetting{
		Key:         "frontend.persistenceNamespaceMaxQPS",
		Default:     0,
		Description: `FrontendPersistenceNamespaceMaxQPS is the max qps each namespace on frontend host can query DB`,
	}
	FrontendPersistenceGlobalNamespaceMaxQPS = &IntNamespaceSetting{
		Key:         "frontend.persistenceGlobalNamespaceMaxQPS",
		Default:     0,
		Description: `FrontendPersistenceNamespaceMaxQPS is the max qps each namespace in frontend cluster can query DB`,
	}
	FrontendEnablePersistencePriorityRateLimiting = &BoolGlobalSetting{
		Key:         "frontend.enablePersistencePriorityRateLimiting",
		Default:     true,
		Description: `FrontendEnablePersistencePriorityRateLimiting indicates if priority rate limiting is enabled in frontend persistence client`,
	}
	FrontendPersistenceDynamicRateLimitingParams = &MapGlobalSetting{
		Key:     "frontend.persistenceDynamicRateLimitingParams",
		Default: dynamicconfig.DefaultDynamicRateLimitingParams,
		Description: `FrontendPersistenceDynamicRateLimitingParams is a map that contains all adjustable dynamic rate limiting params
see DefaultDynamicRateLimitingParams for available options and defaults`,
	}
	FrontendVisibilityMaxPageSize = &IntNamespaceSetting{
		Key:         "frontend.visibilityMaxPageSize",
		Default:     1000,
		Description: `FrontendVisibilityMaxPageSize is default max size for ListWorkflowExecutions in one page`,
	}
	FrontendHistoryMaxPageSize = &IntNamespaceSetting{
		Key:         "frontend.historyMaxPageSize",
		Default:     common.GetHistoryMaxPageSize,
		Description: `FrontendHistoryMaxPageSize is default max size for GetWorkflowExecutionHistory in one page`,
	}
	FrontendRPS = &IntGlobalSetting{
		Key:         "frontend.rps",
		Default:     2400,
		Description: `FrontendRPS is workflow rate limit per second per-instance`,
	}
	FrontendGlobalRPS = &IntGlobalSetting{
		Key:         "frontend.globalRPS",
		Default:     0,
		Description: `FrontendGlobalRPS is workflow rate limit per second for the whole cluster`,
	}
	FrontendNamespaceReplicationInducingAPIsRPS = &IntGlobalSetting{
		Key:     "frontend.rps.namespaceReplicationInducingAPIs",
		Default: 20,
		Description: `FrontendNamespaceReplicationInducingAPIsRPS limits the per second request rate for namespace replication inducing
APIs (e.g. RegisterNamespace, UpdateNamespace, UpdateWorkerBuildIdCompatibility).
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	}
	FrontendMaxNamespaceRPSPerInstance = &IntNamespaceSetting{
		Key:         "frontend.namespaceRPS",
		Default:     2400,
		Description: `FrontendMaxNamespaceRPSPerInstance is workflow namespace rate limit per second`,
	}
	FrontendMaxNamespaceBurstRatioPerInstance = &FloatNamespaceSetting{
		Key:     "frontend.namespaceBurstRatio",
		Default: 2,
		Description: `FrontendMaxNamespaceBurstRatioPerInstance is workflow namespace burst limit as a ratio of namespace RPS. The RPS
used here will be the effective RPS from global and per-instance limits. The value must be 1 or higher.`,
	}
	FrontendMaxConcurrentLongRunningRequestsPerInstance = &IntNamespaceSetting{
		Key:     "frontend.namespaceCount",
		Default: 1200,
		Description: `FrontendMaxConcurrentLongRunningRequestsPerInstance limits concurrent long-running requests per-instance,
per-API. Example requests include long-poll requests, and 'Query' requests (which need to wait for WFTs). The
limit is applied individually to each API method. This value is ignored if
FrontendGlobalMaxConcurrentLongRunningRequests is greater than zero. Warning: setting this to zero will cause all
long-running requests to fail. The name 'frontend.namespaceCount' is kept for backwards compatibility with
existing deployments even though it is a bit of a misnomer. This does not limit the number of namespaces; it is a
per-_namespace_ limit on the _count_ of long-running requests. Requests are only throttled when the limit is
exceeded, not when it is only reached.`,
	}
	FrontendGlobalMaxConcurrentLongRunningRequests = &IntNamespaceSetting{
		Key:     "frontend.globalNamespaceCount",
		Default: 0,
		Description: `FrontendGlobalMaxConcurrentLongRunningRequests limits concurrent long-running requests across all frontend
instances in the cluster, for a given namespace, per-API method. If this is set to 0 (the default), then it is
ignored. The name 'frontend.globalNamespaceCount' is kept for consistency with the per-instance limit name,
'frontend.namespaceCount'.`,
	}
	FrontendMaxNamespaceVisibilityRPSPerInstance = &IntNamespaceSetting{
		Key:     "frontend.namespaceRPS.visibility",
		Default: 10,
		Description: `FrontendMaxNamespaceVisibilityRPSPerInstance is namespace rate limit per second for visibility APIs.
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	}
	FrontendMaxNamespaceNamespaceReplicationInducingAPIsRPSPerInstance = &IntNamespaceSetting{
		Key:     "frontend.namespaceRPS.namespaceReplicationInducingAPIs",
		Default: 1,
		Description: `FrontendMaxNamespaceNamespaceReplicationInducingAPIsRPSPerInstance is a per host/per namespace RPS limit for
namespace replication inducing APIs (e.g. RegisterNamespace, UpdateNamespace, UpdateWorkerBuildIdCompatibility).
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	}
	FrontendMaxNamespaceVisibilityBurstRatioPerInstance = &FloatNamespaceSetting{
		Key:     "frontend.namespaceBurstRatio.visibility",
		Default: 1,
		Description: `FrontendMaxNamespaceVisibilityBurstRatioPerInstance is namespace burst limit for visibility APIs as a ratio of
namespace visibility RPS. The RPS used here will be the effective RPS from global and per-instance limits. This
config is EXPERIMENTAL and may be changed or removed in a later release. The value must be 1 or higher.`,
	}
	FrontendMaxNamespaceNamespaceReplicationInducingAPIsBurstRatioPerInstance = &FloatNamespaceSetting{
		Key:     "frontend.namespaceBurstRatio.namespaceReplicationInducingAPIs",
		Default: 10,
		Description: `FrontendMaxNamespaceNamespaceReplicationInducingAPIsBurstRatioPerInstance is a per host/per namespace burst limit for
namespace replication inducing APIs (e.g. RegisterNamespace, UpdateNamespace, UpdateWorkerBuildIdCompatibility)
as a ratio of namespace ReplicationInducingAPIs RPS. The RPS used here will be the effective RPS from global and
per-instance limits. This config is EXPERIMENTAL and may be changed or removed in a later release. The value must
be 1 or higher.`,
	}
	FrontendGlobalNamespaceRPS = &IntNamespaceSetting{
		Key:     "frontend.globalNamespaceRPS",
		Default: 0,
		Description: `FrontendGlobalNamespaceRPS is workflow namespace rate limit per second for the whole cluster.
The limit is evenly distributed among available frontend service instances.
If this is set, it overwrites per instance limit "frontend.namespaceRPS".`,
	}
	InternalFrontendGlobalNamespaceRPS = &IntNamespaceSetting{
		Key:     "internal-frontend.globalNamespaceRPS",
		Default: 0,
		Description: `InternalFrontendGlobalNamespaceRPS is workflow namespace rate limit per second across
all internal-frontends.`,
	}
	FrontendGlobalNamespaceVisibilityRPS = &IntNamespaceSetting{
		Key:     "frontend.globalNamespaceRPS.visibility",
		Default: 0,
		Description: `FrontendGlobalNamespaceVisibilityRPS is workflow namespace rate limit per second for the whole cluster for visibility API.
The limit is evenly distributed among available frontend service instances.
If this is set, it overwrites per instance limit "frontend.namespaceRPS.visibility".
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	}
	FrontendGlobalNamespaceNamespaceReplicationInducingAPIsRPS = &IntNamespaceSetting{
		Key:     "frontend.globalNamespaceRPS.namespaceReplicationInducingAPIs",
		Default: 10,
		Description: `FrontendGlobalNamespaceNamespaceReplicationInducingAPIsRPS is a cluster global, per namespace RPS limit for
namespace replication inducing APIs (e.g. RegisterNamespace, UpdateNamespace, UpdateWorkerBuildIdCompatibility).
The limit is evenly distributed among available frontend service instances.
If this is set, it overwrites the per instance limit configured with
"frontend.namespaceRPS.namespaceReplicationInducingAPIs".
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	}
	InternalFrontendGlobalNamespaceVisibilityRPS = &IntNamespaceSetting{
		Key:     "internal-frontend.globalNamespaceRPS.visibility",
		Default: 0,
		Description: `InternalFrontendGlobalNamespaceVisibilityRPS is workflow namespace rate limit per second
across all internal-frontends.
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	}
	FrontendThrottledLogRPS = &IntGlobalSetting{
		Key:         "frontend.throttledLogRPS",
		Default:     20,
		Description: `FrontendThrottledLogRPS is the rate limit on number of log messages emitted per second for throttled logger`,
	}
	FrontendShutdownDrainDuration = &DurationGlobalSetting{
		Key:         "frontend.shutdownDrainDuration",
		Default:     0 * time.Second,
		Description: `FrontendShutdownDrainDuration is the duration of traffic drain during shutdown`,
	}
	FrontendShutdownFailHealthCheckDuration = &DurationGlobalSetting{
		Key:         "frontend.shutdownFailHealthCheckDuration",
		Default:     0 * time.Second,
		Description: `FrontendShutdownFailHealthCheckDuration is the duration of shutdown failure detection`,
	}
	FrontendMaxBadBinaries = &IntNamespaceSetting{
		Key:         "frontend.maxBadBinaries",
		Default:     namespace.MaxBadBinaries,
		Description: `FrontendMaxBadBinaries is the max number of bad binaries in namespace config`,
	}
	SendRawWorkflowHistory = &BoolNamespaceSetting{
		Key:         "frontend.sendRawWorkflowHistory",
		Default:     false,
		Description: `SendRawWorkflowHistory is whether to enable raw history retrieving`,
	}
	SearchAttributesNumberOfKeysLimit = &IntNamespaceSetting{
		Key:         "frontend.searchAttributesNumberOfKeysLimit",
		Default:     100,
		Description: `SearchAttributesNumberOfKeysLimit is the limit of number of keys`,
	}
	SearchAttributesSizeOfValueLimit = &IntNamespaceSetting{
		Key:         "frontend.searchAttributesSizeOfValueLimit",
		Default:     2 * 1024,
		Description: `SearchAttributesSizeOfValueLimit is the size limit of each value`,
	}
	SearchAttributesTotalSizeLimit = &IntNamespaceSetting{
		Key:         "frontend.searchAttributesTotalSizeLimit",
		Default:     40 * 1024,
		Description: `SearchAttributesTotalSizeLimit is the size limit of the whole map`,
	}
	VisibilityArchivalQueryMaxPageSize = &IntGlobalSetting{
		Key:         "frontend.visibilityArchivalQueryMaxPageSize",
		Default:     10000,
		Description: `VisibilityArchivalQueryMaxPageSize is the maximum page size for a visibility archival query`,
	}
	EnableServerVersionCheck = &BoolGlobalSetting{
		Key:         "frontend.enableServerVersionCheck",
		Default:     os.Getenv("TEMPORAL_VERSION_CHECK_DISABLED"),
		Description: `EnableServerVersionCheck is a flag that controls whether or not periodic version checking is enabled`,
	}
	EnableTokenNamespaceEnforcement = &BoolGlobalSetting{
		Key:         "frontend.enableTokenNamespaceEnforcement",
		Default:     true,
		Description: `EnableTokenNamespaceEnforcement enables enforcement that namespace in completion token matches namespace of the request`,
	}
	DisableListVisibilityByFilter = &BoolNamespaceSetting{
		Key:         "frontend.disableListVisibilityByFilter",
		Default:     false,
		Description: `DisableListVisibilityByFilter is config to disable list open/close workflow using filter`,
	}
	KeepAliveMinTime = &DurationGlobalSetting{
		Key:         "frontend.keepAliveMinTime",
		Default:     10 * time.Second,
		Description: `KeepAliveMinTime is the minimum amount of time a client should wait before sending a keepalive ping.`,
	}
	KeepAlivePermitWithoutStream = &BoolGlobalSetting{
		Key:     "frontend.keepAlivePermitWithoutStream",
		Default: true,
		Description: `KeepAlivePermitWithoutStream If true, server allows keepalive pings even when there are no active
streams(RPCs). If false, and client sends ping when there are no active
streams, server will send GOAWAY and close the connection.`,
	}
	KeepAliveMaxConnectionIdle = &DurationGlobalSetting{
		Key:     "frontend.keepAliveMaxConnectionIdle",
		Default: 2 * time.Minute,
		Description: `KeepAliveMaxConnectionIdle is a duration for the amount of time after which an
idle connection would be closed by sending a GoAway. Idleness duration is
defined since the most recent time the number of outstanding RPCs became
zero or the connection establishment.`,
	}
	KeepAliveMaxConnectionAge = &DurationGlobalSetting{
		Key:     "frontend.keepAliveMaxConnectionAge",
		Default: 5 * time.Minute,
		Description: `KeepAliveMaxConnectionAge is a duration for the maximum amount of time a
connection may exist before it will be closed by sending a GoAway. A
random jitter of +/-10% will be added to MaxConnectionAge to spread out
connection storms.`,
	}
	KeepAliveMaxConnectionAgeGrace = &DurationGlobalSetting{
		Key:     "frontend.keepAliveMaxConnectionAgeGrace",
		Default: 70 * time.Second,
		Description: `KeepAliveMaxConnectionAgeGrace is an additive period after MaxConnectionAge after
which the connection will be forcibly closed.`,
	}
	KeepAliveTime = &DurationGlobalSetting{
		Key:     "frontend.keepAliveTime",
		Default: 1 * time.Minute,
		Description: `KeepAliveTime After a duration of this time if the server doesn't see any activity it
pings the client to see if the transport is still alive.
If set below 1s, a minimum value of 1s will be used instead.`,
	}
	KeepAliveTimeout = &DurationGlobalSetting{
		Key:     "frontend.keepAliveTimeout",
		Default: 10 * time.Second,
		Description: `KeepAliveTimeout After having pinged for keepalive check, the server waits for a duration
of Timeout and if no activity is seen even after that the connection is closed.`,
	}
	FrontendEnableSchedules = &BoolNamespaceSetting{
		Key:         "frontend.enableSchedules",
		Default:     true,
		Description: `FrontendEnableSchedules enables schedule-related RPCs in the frontend`,
	}
	FrontendEnableNexusAPIs = &BoolGlobalSetting{
		Key:         "frontend.enableNexusAPIs",
		Default:     false,
		Description: `FrontendEnableNexusAPIs enables serving Nexus HTTP requests in the frontend.`,
	}
	FrontendRefreshNexusIncomingServicesLongPollTimeout = &DurationGlobalSetting{
		Key:         "frontend.refreshNexusIncomingServicesLongPollTimeout",
		Default:     5 * time.Minute,
		Description: `FrontendRefreshNexusIncomingServicesLongPollTimeout is the maximum duration of background long poll requests to update Nexus incoming services.`,
	}
	// // FrontendRefreshNexusIncomingServicesMinWait is the minimum wait time between background long poll requests to update Nexus incoming services.
	// FrontendRefreshNexusIncomingServicesMinWait = "frontend.refreshNexusIncomingServicesMinWait"
	FrontendEnableCallbackAttachment = &BoolNamespaceSetting{
		Key:         "frontend.enableCallbackAttachment",
		Default:     false,
		Description: `FrontendEnableCallbackAttachment enables attaching callbacks to workflows.`,
	}
	FrontendMaxConcurrentBatchOperationPerNamespace = &IntNamespaceSetting{
		Key:         "frontend.MaxConcurrentBatchOperationPerNamespace",
		Default:     1,
		Description: `FrontendMaxConcurrentBatchOperationPerNamespace is the max concurrent batch operation job count per namespace`,
	}
	FrontendMaxExecutionCountBatchOperationPerNamespace = &IntNamespaceSetting{
		Key:         "frontend.MaxExecutionCountBatchOperationPerNamespace",
		Default:     1000,
		Description: `FrontendMaxExecutionCountBatchOperationPerNamespace is the max execution count batch operation supports per namespace`,
	}
	FrontendEnableBatcher = &BoolNamespaceSetting{
		Key:         "frontend.enableBatcher",
		Default:     true,
		Description: `FrontendEnableBatcher enables batcher-related RPCs in the frontend`,
	}
	FrontendAccessHistoryFraction = &Float64GlobalSetting{
		Key:     "frontend.accessHistoryFraction",
		Default: 0.0,
		Description: `FrontendAccessHistoryFraction (0.0~1.0) is the fraction of history operations that are sent to the history
service using the new RPCs. The remaining access history via the existing implementation.
TODO: remove once migration completes.`,
	}
	FrontendAdminDeleteAccessHistoryFraction = &Float64GlobalSetting{
		Key:     "frontend.adminDeleteAccessHistoryFraction",
		Default: 0.0,
		Description: `FrontendAdminDeleteAccessHistoryFraction (0.0~1.0) is the fraction of admin DeleteWorkflowExecution requests
that are sent to the history service using the new RPCs. The remaining access history via the existing implementation.
TODO: remove once migration completes.`,
	}

	FrontendEnableUpdateWorkflowExecution = &BoolNamespaceSetting{
		Key:     "frontend.enableUpdateWorkflowExecution",
		Default: false,
		Description: `FrontendEnableUpdateWorkflowExecution enables UpdateWorkflowExecution API in the frontend.
The UpdateWorkflowExecution API has gone through rigorous testing efforts but this config's default is 'false' until the
feature gets more time in production.`,
	}

	FrontendEnableExecuteMultiOperation = &BoolNamespaceSetting{
		Key:     "frontend.enableExecuteMultiOperation",
		Default: false,
		Description: `FrontendEnableExecuteMultiOperation enables the ExecuteMultiOperation API in the frontend.
The API is under active development.`,
	}

	FrontendEnableUpdateWorkflowExecutionAsyncAccepted = &BoolNamespaceSetting{
		Key:     "frontend.enableUpdateWorkflowExecutionAsyncAccepted",
		Default: false,
		Description: `FrontendEnableUpdateWorkflowExecutionAsyncAccepted enables the form of
asynchronous workflow execution update that waits on the "Accepted"
lifecycle stage. Default value is 'false'.`,
	}

	EnableWorkflowIdConflictPolicy = &BoolNamespaceSetting{
		Key:         "frontend.enableWorkflowIdConflictPolicy",
		Default:     false,
		Description: `EnableWorkflowIdConflictPolicy enables the 'WorkflowIdConflictPolicy' option for Start and Signal-with-Start`,
	}

	FrontendEnableWorkerVersioningDataAPIs = &BoolNamespaceSetting{
		Key:         "frontend.workerVersioningDataAPIs",
		Default:     false,
		Description: `FrontendEnableWorkerVersioningDataAPIs enables worker versioning data read / write APIs.`,
	}
	FrontendEnableWorkerVersioningWorkflowAPIs = &BoolNamespaceSetting{
		Key:         "frontend.workerVersioningWorkflowAPIs",
		Default:     false,
		Description: `FrontendEnableWorkerVersioningWorkflowAPIs enables worker versioning in workflow progress APIs.`,
	}

	DeleteNamespaceDeleteActivityRPS = &IntGlobalSetting{
		Key:     "frontend.deleteNamespaceDeleteActivityRPS",
		Default: 100,
		Description: `DeleteNamespaceDeleteActivityRPS is an RPS per every parallel delete executions activity.
Total RPS is equal to DeleteNamespaceDeleteActivityRPS * DeleteNamespaceConcurrentDeleteExecutionsActivities.
Default value is 100.`,
	}
	DeleteNamespacePageSize = &IntGlobalSetting{
		Key:     "frontend.deleteNamespaceDeletePageSize",
		Default: 1000,
		Description: `DeleteNamespacePageSize is a page size to read executions from visibility for delete executions activity.
Default value is 1000.`,
	}
	DeleteNamespacePagesPerExecution = &IntGlobalSetting{
		Key:     "frontend.deleteNamespacePagesPerExecution",
		Default: 256,
		Description: `DeleteNamespacePagesPerExecution is a number of pages before returning ContinueAsNew from delete executions activity.
Default value is 256.`,
	}
	DeleteNamespaceConcurrentDeleteExecutionsActivities = &IntGlobalSetting{
		Key:     "frontend.deleteNamespaceConcurrentDeleteExecutionsActivities",
		Default: 4,
		Description: `DeleteNamespaceConcurrentDeleteExecutionsActivities is a number of concurrent delete executions activities.
Must be not greater than 256 and number of worker cores in the cluster.
Default is 4.`,
	}
	DeleteNamespaceNamespaceDeleteDelay = &DurationGlobalSetting{
		Key:     "frontend.deleteNamespaceNamespaceDeleteDelay",
		Default: 0 * time.Hour,
		Description: `DeleteNamespaceNamespaceDeleteDelay is a duration for how long namespace stays in database
after all namespace resources (i.e. workflow executions) are deleted.
Default is 0, means, namespace will be deleted immediately.`,
	}

	// keys for matching

	MatchingRPS = &IntGlobalSetting{
		Key:         "matching.rps",
		Default:     1200,
		Description: `MatchingRPS is request rate per second for each matching host`,
	}
	MatchingPersistenceMaxQPS = &IntGlobalSetting{
		Key:         "matching.persistenceMaxQPS",
		Default:     3000,
		Description: `MatchingPersistenceMaxQPS is the max qps matching host can query DB`,
	}
	MatchingPersistenceGlobalMaxQPS = &IntGlobalSetting{
		Key:         "matching.persistenceGlobalMaxQPS",
		Default:     0,
		Description: `MatchingPersistenceGlobalMaxQPS is the max qps matching cluster can query DB`,
	}
	MatchingPersistenceNamespaceMaxQPS = &IntNamespaceSetting{
		Key:         "matching.persistenceNamespaceMaxQPS",
		Default:     0,
		Description: `MatchingPersistenceNamespaceMaxQPS is the max qps each namespace on matching host can query DB`,
	}
	MatchingPersistenceGlobalNamespaceMaxQPS = &IntNamespaceSetting{
		Key:         "matching.persistenceGlobalNamespaceMaxQPS",
		Default:     0,
		Description: `MatchingPersistenceNamespaceMaxQPS is the max qps each namespace in matching cluster can query DB`,
	}
	MatchingEnablePersistencePriorityRateLimiting = &BoolGlobalSetting{
		Key:         "matching.enablePersistencePriorityRateLimiting",
		Default:     true,
		Description: `MatchingEnablePersistencePriorityRateLimiting indicates if priority rate limiting is enabled in matching persistence client`,
	}
	MatchingPersistenceDynamicRateLimitingParams = &MapGlobalSetting{
		Key:     "matching.persistenceDynamicRateLimitingParams",
		Default: dynamicconfig.DefaultDynamicRateLimitingParams,
		Description: `MatchingPersistenceDynamicRateLimitingParams is a map that contains all adjustable dynamic rate limiting params
see DefaultDynamicRateLimitingParams for available options and defaults`,
	}
	MatchingMinTaskThrottlingBurstSize = &IntTaskQueueInfoSetting{
		Key:         "matching.minTaskThrottlingBurstSize",
		Default:     1,
		Description: `MatchingMinTaskThrottlingBurstSize is the minimum burst size for task queue throttling`,
	}
	MatchingGetTasksBatchSize = &IntTaskQueueInfoSetting{
		Key:         "matching.getTasksBatchSize",
		Default:     1000,
		Description: `MatchingGetTasksBatchSize is the maximum batch size to fetch from the task buffer`,
	}
	MatchingLongPollExpirationInterval = &DurationTaskQueueInfoSetting{
		Key:         "matching.longPollExpirationInterval",
		Default:     time.Minute,
		Description: `MatchingLongPollExpirationInterval is the long poll expiration interval in the matching service`,
	}
	MatchingSyncMatchWaitDuration = &DurationTaskQueueInfoSetting{
		Key:         "matching.syncMatchWaitDuration",
		Default:     200 * time.Millisecond,
		Description: `MatchingSyncMatchWaitDuration is to wait time for sync match`,
	}
	MatchingHistoryMaxPageSize = &IntNamespaceSetting{
		Key:         "matching.historyMaxPageSize",
		Default:     common.GetHistoryMaxPageSize,
		Description: `MatchingHistoryMaxPageSize is the maximum page size of history events returned on PollWorkflowTaskQueue requests`,
	}
	MatchingLoadUserData = &BoolTaskQueueInfoSetting{
		Key:     "matching.loadUserData",
		Default: true,
		Description: `MatchingLoadUserData can be used to entirely disable loading user data from persistence (and the inter node RPCs
that propoagate it). When turned off, features that rely on user data (e.g. worker versioning) will essentially
be disabled. When disabled, matching will drop tasks for versioned workflows and activities to avoid breaking
versioning semantics. Operator intervention will be required to reschedule the dropped tasks.`,
	}
	MatchingUpdateAckInterval = &DurationTaskQueueInfoSetting{
		Key:         "matching.updateAckInterval",
		Default:     defaultUpdateAckInterval,
		Description: `MatchingUpdateAckInterval is the interval for update ack`,
	}
	MatchingMaxTaskQueueIdleTime = &DurationTaskQueueInfoSetting{
		Key:     "matching.maxTaskQueueIdleTime",
		Default: 5 * time.Minute,
		Description: `MatchingMaxTaskQueueIdleTime is the time after which an idle task queue will be unloaded.
Note: this should be greater than matching.longPollExpirationInterval and matching.getUserDataLongPollTimeout.`,
	}
	MatchingOutstandingTaskAppendsThreshold = &IntTaskQueueInfoSetting{
		Key:         "matching.outstandingTaskAppendsThreshold",
		Default:     250,
		Description: `MatchingOutstandingTaskAppendsThreshold is the threshold for outstanding task appends`,
	}
	MatchingMaxTaskBatchSize = &IntTaskQueueInfoSetting{
		Key:         "matching.maxTaskBatchSize",
		Default:     100,
		Description: `MatchingMaxTaskBatchSize is max batch size for task writer`,
	}
	MatchingMaxTaskDeleteBatchSize = &IntTaskQueueInfoSetting{
		Key:         "matching.maxTaskDeleteBatchSize",
		Default:     100,
		Description: `MatchingMaxTaskDeleteBatchSize is the max batch size for range deletion of tasks`,
	}
	MatchingThrottledLogRPS = &IntGlobalSetting{
		Key:         "matching.throttledLogRPS",
		Default:     20,
		Description: `MatchingThrottledLogRPS is the rate limit on number of log messages emitted per second for throttled logger`,
	}
	MatchingNumTaskqueueWritePartitions = &IntNamespaceSetting{
		Key:                "matching.numTaskqueueWritePartitions",
		ConstrainedDefault: asdf,
		Description:        `MatchingNumTaskqueueWritePartitions is the number of write partitions for a task queue`,
	}
	MatchingNumTaskqueueReadPartitions = &IntNamespaceSetting{
		Key:                "matching.numTaskqueueReadPartitions",
		ConstrainedDefault: asdf,
		Description:        `MatchingNumTaskqueueReadPartitions is the number of read partitions for a task queue`,
	}
	MatchingNumTaskqueueReadPartitions   = "matching.numTaskqueueReadPartitions"
	MatchingForwarderMaxOutstandingPolls = &IntTaskQueueInfoSetting{
		Key:         "matching.forwarderMaxOutstandingPolls",
		Default:     1,
		Description: `MatchingForwarderMaxOutstandingPolls is the max number of inflight polls from the forwarder`,
	}
	MatchingForwarderMaxOutstandingTasks = &IntTaskQueueInfoSetting{
		Key:         "matching.forwarderMaxOutstandingTasks",
		Default:     1,
		Description: `MatchingForwarderMaxOutstandingTasks is the max number of inflight addTask/queryTask from the forwarder`,
	}
	MatchingForwarderMaxRatePerSecond = &IntTaskQueueInfoSetting{
		Key:         "matching.forwarderMaxRatePerSecond",
		Default:     10,
		Description: `MatchingForwarderMaxRatePerSecond is the max rate at which add/query can be forwarded`,
	}
	MatchingForwarderMaxChildrenPerNode = &IntTaskQueueInfoSetting{
		Key:         "matching.forwarderMaxChildrenPerNode",
		Default:     20,
		Description: `MatchingForwarderMaxChildrenPerNode is the max number of children per node in the task queue partition tree`,
	}
	MatchingAlignMembershipChange = &DurationGlobalSetting{
		Key:     "matching.alignMembershipChange",
		Default: 0 * time.Second,
		Description: `MatchingAlignMembershipChange is a duration to align matching's membership changes to.
This can help reduce effects of task queue movement.`,
	}
	MatchingShutdownDrainDuration = &DurationGlobalSetting{
		Key:         "matching.shutdownDrainDuration",
		Default:     0 * time.Second,
		Description: `MatchingShutdownDrainDuration is the duration of traffic drain during shutdown`,
	}
	MatchingGetUserDataLongPollTimeout = &DurationGlobalSetting{
		Key:         "matching.getUserDataLongPollTimeout",
		Default:     5*time.Minute - 10*time.Second,
		Description: `MatchingGetUserDataLongPollTimeout is the max length of long polls for GetUserData calls between partitions.`,
	}
	MatchingBacklogNegligibleAge = &DurationTaskQueueInfoSetting{
		Key:     "matching.backlogNegligibleAge",
		Default: 24 * 365 * 10 * time.Hour,
		Description: `MatchingBacklogNegligibleAge if the head of backlog gets older than this we stop sync match and
forwarding to ensure more equal dispatch order among partitions.`,
	}
	MatchingMaxWaitForPollerBeforeFwd = &DurationTaskQueueInfoSetting{
		Key:     "matching.maxWaitForPollerBeforeFwd",
		Default: 200 * time.Millisecond,
		Description: `MatchingMaxWaitForPollerBeforeFwd in presence of a non-negligible backlog, we resume forwarding tasks if the
duration since last poll exceeds this threshold.`,
	}
	QueryPollerUnavailableWindow = &DurationGlobalSetting{
		Key:         "matching.queryPollerUnavailableWindow",
		Default:     20 * time.Second,
		Description: `QueryPollerUnavailableWindow WF Queries are rejected after a while if no poller has been seen within the window`,
	}
	MatchingListNexusIncomingServicesLongPollTimeout = &DurationGlobalSetting{
		Key:         "matching.listNexusIncomingServicesLongPollTimeout",
		Default:     5*time.Minute - 10*time.Second,
		Description: `MatchingListNexusIncomingServicesLongPollTimeout is the max length of long polls for ListNexusIncomingServices calls.`,
	}
	MatchingMembershipUnloadDelay = &DurationGlobalSetting{
		Key:     "matching.membershipUnloadDelay",
		Default: 500 * time.Millisecond,
		Description: `MatchingMembershipUnloadDelay is how long to wait to re-confirm loss of ownership before unloading a task queue.
Set to zero to disable proactive unload.`,
	}
	MatchingQueryWorkflowTaskTimeoutLogRate = &FloatTaskQueueInfoSetting{
		Key:     "matching.queryWorkflowTaskTimeoutLogRate",
		Default: 0.0,
		Description: `MatchingQueryWorkflowTaskTimeoutLogRate defines the sampling rate for logs when a query workflow task times out. Since
these log lines can be noisy, we want to be able to turn on and sample selectively for each affected namespace.`,
	}

	// for matching testing only:

	TestMatchingDisableSyncMatch = &BoolGlobalSetting{
		Key:         "test.matching.disableSyncMatch",
		Default:     false,
		Description: `TestMatchingDisableSyncMatch forces tasks to go through the db once`,
	}
	TestMatchingLBForceReadPartition = &IntGlobalSetting{
		Key:         "test.matching.lbForceReadPartition",
		Default:     -1,
		Description: `TestMatchingLBForceReadPartition forces polls to go to a specific partition`,
	}
	TestMatchingLBForceWritePartition = &IntGlobalSetting{
		Key:         "test.matching.lbForceWritePartition",
		Default:     -1,
		Description: `TestMatchingLBForceWritePartition forces adds to go to a specific partition`,
	}

	// keys for history

	EnableReplicationStream = &BoolGlobalSetting{
		Key:         "history.enableReplicationStream",
		Default:     false,
		Description: `EnableReplicationStream turn on replication stream`,
	}
	EnableHistoryReplicationDLQV2 = &BoolGlobalSetting{
		Key:     "history.enableHistoryReplicationDLQV2",
		Default: false,
		Description: `EnableHistoryReplicationDLQV2 switches to the DLQ v2 implementation for history replication. See details in
[go.temporal.io/server/common/persistence.QueueV2]. This feature is currently in development. Do NOT use it in
production.`,
	}

	HistoryRPS = &IntGlobalSetting{
		Key:         "history.rps",
		Default:     3000,
		Description: `HistoryRPS is request rate per second for each history host`,
	}
	HistoryPersistenceMaxQPS = &IntGlobalSetting{
		Key:         "history.persistenceMaxQPS",
		Default:     9000,
		Description: `HistoryPersistenceMaxQPS is the max qps history host can query DB`,
	}
	HistoryPersistenceGlobalMaxQPS = &IntGlobalSetting{
		Key:         "history.persistenceGlobalMaxQPS",
		Default:     0,
		Description: `HistoryPersistenceGlobalMaxQPS is the max qps history cluster can query DB`,
	}
	HistoryPersistenceNamespaceMaxQPS = &IntNamespaceSetting{
		Key:     "history.persistenceNamespaceMaxQPS",
		Default: 0,
		Description: `HistoryPersistenceNamespaceMaxQPS is the max qps each namespace on history host can query DB
If value less or equal to 0, will fall back to HistoryPersistenceMaxQPS`,
	}
	HistoryPersistenceGlobalNamespaceMaxQPS = &IntNamespaceSetting{
		Key:         "history.persistenceGlobalNamespaceMaxQPS",
		Default:     0,
		Description: `HistoryPersistenceNamespaceMaxQPS is the max qps each namespace in history cluster can query DB`,
	}
	HistoryPersistencePerShardNamespaceMaxQPS = &IntNamespaceSetting{
		Key:         "history.persistencePerShardNamespaceMaxQPS",
		Default:     0,
		Description: `HistoryPersistencePerShardNamespaceMaxQPS is the max qps each namespace on a shard can query DB`,
	}
	HistoryEnablePersistencePriorityRateLimiting = &BoolGlobalSetting{
		Key:         "history.enablePersistencePriorityRateLimiting",
		Default:     true,
		Description: `HistoryEnablePersistencePriorityRateLimiting indicates if priority rate limiting is enabled in history persistence client`,
	}
	HistoryPersistenceDynamicRateLimitingParams = &MapGlobalSetting{
		Key:     "history.persistenceDynamicRateLimitingParams",
		Default: dynamicconfig.DefaultDynamicRateLimitingParams,
		Description: `HistoryPersistenceDynamicRateLimitingParams is a map that contains all adjustable dynamic rate limiting params
see DefaultDynamicRateLimitingParams for available options and defaults`,
	}
	HistoryLongPollExpirationInterval = &DurationNamespaceSetting{
		Key:         "history.longPollExpirationInterval",
		Default:     time.Second * 20,
		Description: `HistoryLongPollExpirationInterval is the long poll expiration interval in the history service`,
	}
	HistoryCacheSizeBasedLimit = &BoolGlobalSetting{
		Key:     "history.cacheSizeBasedLimit",
		Default: false,
		Description: `HistoryCacheSizeBasedLimit if true, size of the history cache will be limited by HistoryCacheMaxSizeBytes
and HistoryCacheHostLevelMaxSizeBytes. Otherwise, entry count in the history cache will be limited by
HistoryCacheMaxSize and HistoryCacheHostLevelMaxSize.`,
	}
	HistoryCacheInitialSize = &IntGlobalSetting{
		Key:         "history.cacheInitialSize",
		Default:     128,
		Description: `HistoryCacheInitialSize is initial size of history cache`,
	}
	HistoryCacheMaxSize = &IntGlobalSetting{
		Key:         "history.cacheMaxSize",
		Default:     512,
		Description: `HistoryCacheMaxSize is the maximum number of entries in the shard level history cache`,
	}
	HistoryCacheMaxSizeBytes = &IntGlobalSetting{
		Key:     "history.cacheMaxSizeBytes",
		Default: 512 * 4 * 1024,
		Description: `HistoryCacheMaxSizeBytes is the maximum size of the shard level history cache in bytes. This is only used if
HistoryCacheSizeBasedLimit is set to true.`,
	}
	HistoryCacheTTL = &DurationGlobalSetting{
		Key:         "history.cacheTTL",
		Default:     time.Hour,
		Description: `HistoryCacheTTL is TTL of history cache`,
	}
	HistoryCacheNonUserContextLockTimeout = &DurationGlobalSetting{
		Key:     "history.cacheNonUserContextLockTimeout",
		Default: 500 * time.Millisecond,
		Description: `HistoryCacheNonUserContextLockTimeout controls how long non-user call (callerType != API or Operator)
will wait on workflow lock acquisition. Requires service restart to take effect.`,
	}
	EnableHostHistoryCache = &BoolGlobalSetting{
		Key:         "history.enableHostHistoryCache",
		Default:     false,
		Description: `EnableHostHistoryCache controls if the history cache is host level`,
	}
	HistoryCacheHostLevelMaxSize = &IntGlobalSetting{
		Key:         "history.hostLevelCacheMaxSize",
		Default:     256000,
		Description: `HistoryCacheHostLevelMaxSize is the maximum number of entries in the host level history cache`,
	}
	HistoryCacheHostLevelMaxSizeBytes = &IntGlobalSetting{
		Key:     "history.hostLevelCacheMaxSizeBytes",
		Default: 256000 * 4 * 1024,
		Description: `HistoryCacheHostLevelMaxSizeBytes is the maximum size of the host level history cache. This is only used if
HistoryCacheSizeBasedLimit is set to true.`,
	}
	EnableMutableStateTransitionHistory = &BoolGlobalSetting{
		Key:     "history.enableMutableStateTransitionHistory",
		Default: false,
		Description: `EnableMutableStateTransitionHistory controls whether to record state transition history in mutable state records.
The feature is used in the hierarchical state machine framework and is considered unstable as the structure may
change with the pending replication design.`,
	}
	HistoryStartupMembershipJoinDelay = &DurationGlobalSetting{
		Key:     "history.startupMembershipJoinDelay",
		Default: 0 * time.Second,
		Description: `HistoryStartupMembershipJoinDelay is the duration a history instance waits
before joining membership after starting.`,
	}
	HistoryShutdownDrainDuration = &DurationGlobalSetting{
		Key:         "history.shutdownDrainDuration",
		Default:     0 * time.Second,
		Description: `HistoryShutdownDrainDuration is the duration of traffic drain during shutdown`,
	}
	XDCCacheMaxSizeBytes = &IntGlobalSetting{
		Key:         "history.xdcCacheMaxSizeBytes",
		Default:     8 * 1024 * 1024,
		Description: `XDCCacheMaxSizeBytes is max size of events cache in bytes`,
	}
	EventsCacheMaxSizeBytes = &IntGlobalSetting{
		Key:         "history.eventsCacheMaxSizeBytes",
		Default:     512 * 1024,
		Description: `EventsCacheMaxSizeBytes is max size of the shard level events cache in bytes`,
	}
	EventsHostLevelCacheMaxSizeBytes = &IntGlobalSetting{
		Key:         "history.eventsHostLevelCacheMaxSizeBytes",
		Default:     512 * 512 * 1024,
		Description: `EventsHostLevelCacheMaxSizeBytes is max size of the host level events cache in bytes`,
	}
	EventsCacheTTL = &DurationGlobalSetting{
		Key:         "history.eventsCacheTTL",
		Default:     time.Hour,
		Description: `EventsCacheTTL is TTL of events cache`,
	}
	EnableHostLevelEventsCache = &BoolGlobalSetting{
		Key:         "history.enableHostLevelEventsCache",
		Default:     false,
		Description: `EnableHostLevelEventsCache controls if the events cache is host level`,
	}
	AcquireShardInterval = &DurationGlobalSetting{
		Key:         "history.acquireShardInterval",
		Default:     time.Minute,
		Description: `AcquireShardInterval is interval that timer used to acquire shard`,
	}
	AcquireShardConcurrency = &IntGlobalSetting{
		Key:         "history.acquireShardConcurrency",
		Default:     10,
		Description: `AcquireShardConcurrency is number of goroutines that can be used to acquire shards in the shard controller.`,
	}
	ShardLingerOwnershipCheckQPS = &IntGlobalSetting{
		Key:     "history.shardLingerOwnershipCheckQPS",
		Default: 4,
		Description: `ShardLingerOwnershipCheckQPS is the frequency to perform shard ownership
checks while a shard is lingering.`,
	}
	ShardLingerTimeLimit = &DurationGlobalSetting{
		Key:     "history.shardLingerTimeLimit",
		Default: 0,
		Description: `ShardLingerTimeLimit configures if and for how long the shard controller
will temporarily delay closing shards after a membership update, awaiting a
shard ownership lost error from persistence. Not recommended with
persistence layers that are missing AssertShardOwnership support.
If set to zero, shards will not delay closing.`,
	}
	ShardOwnershipAssertionEnabled = &BoolGlobalSetting{
		Key:     "history.shardOwnershipAssertionEnabled",
		Default: false,
		Description: `ShardOwnershipAssertionEnabled configures if the shard ownership is asserted
for API requests when a NotFound or NamespaceNotFound error is returned from
persistence.
NOTE: Shard ownership assertion is not implemented by any persistence implementation
in this codebase, because assertion is not needed for persistence implementation
that guarantees read after write consistency. As a result, even if this config is
enabled, it's a no-op.`,
	}
	HistoryClientOwnershipCachingEnabled = &BoolGlobalSetting{
		Key:     "history.clientOwnershipCachingEnabled",
		Default: false,
		Description: `HistoryClientOwnershipCachingEnabled configures if history clients try to cache
shard ownership information, instead of checking membership for each request.
Only inspected when an instance first creates a history client, so changes
to this require a restart to take effect.`,
	}
	ShardIOConcurrency = &IntGlobalSetting{
		Key:         "history.shardIOConcurrency",
		Default:     1,
		Description: `ShardIOConcurrency controls the concurrency of persistence operations in shard context`,
	}
	StandbyClusterDelay = &DurationGlobalSetting{
		Key:         "history.standbyClusterDelay",
		Default:     5 * time.Minute,
		Description: `StandbyClusterDelay is the artificial delay added to standby cluster's view of active cluster's time`,
	}
	StandbyTaskMissingEventsResendDelay = &DurationTaskTypeSetting{
		Key:     "history.standbyTaskMissingEventsResendDelay",
		Default: 10 * time.Minute,
		Description: `StandbyTaskMissingEventsResendDelay is the amount of time standby cluster's will wait (if events are missing)
before calling remote for missing events`,
	}
	StandbyTaskMissingEventsDiscardDelay = &DurationTaskTypeSetting{
		Key:     "history.standbyTaskMissingEventsDiscardDelay",
		Default: 15 * time.Minute,
		Description: `StandbyTaskMissingEventsDiscardDelay is the amount of time standby cluster's will wait (if events are missing)
before discarding the task`,
	}
	QueuePendingTaskCriticalCount = &IntGlobalSetting{
		Key:     "history.queuePendingTaskCriticalCount",
		Default: 9000,
		Description: `QueuePendingTaskCriticalCount is the max number of pending task in one queue
before triggering queue slice splitting and unloading`,
	}
	QueueReaderStuckCriticalAttempts = &IntGlobalSetting{
		Key:     "history.queueReaderStuckCriticalAttempts",
		Default: 3,
		Description: `QueueReaderStuckCriticalAttempts is the max number of task loading attempts for a certain task range
before that task range is split into a separate slice to unblock loading for later range.
currently only work for scheduled queues and the task range is 1s.`,
	}
	QueueCriticalSlicesCount = &IntGlobalSetting{
		Key:     "history.queueCriticalSlicesCount",
		Default: 50,
		Description: `QueueCriticalSlicesCount is the max number of slices in one queue
before force compacting slices`,
	}
	QueuePendingTaskMaxCount = &IntGlobalSetting{
		Key:     "history.queuePendingTasksMaxCount",
		Default: 10000,
		Description: `QueuePendingTaskMaxCount is the max number of task pending tasks in one queue before stop
loading new tasks into memory. While QueuePendingTaskCriticalCount won't stop task loading
for the entire queue but only trigger a queue action to unload tasks. Ideally this max count
limit should not be hit and task unloading should happen once critical count is exceeded. But
since queue action is async, we need this hard limit.`,
	}
	ContinueAsNewMinInterval = &DurationNamespaceSetting{
		Key:     "history.continueAsNewMinInterval",
		Default: time.Second,
		Description: `ContinueAsNewMinInterval is the minimal interval between continue_as_new executions.
This is needed to prevent tight loop continue_as_new spin. Default is 1s.`,
	}

	TaskSchedulerEnableRateLimiter = &BoolGlobalSetting{
		Key:         "history.taskSchedulerEnableRateLimiter",
		Default:     false,
		Description: `TaskSchedulerEnableRateLimiter indicates if task scheduler rate limiter should be enabled`,
	}
	TaskSchedulerEnableRateLimiterShadowMode = &BoolGlobalSetting{
		Key:     "history.taskSchedulerEnableRateLimiterShadowMode",
		Default: true,
		Description: `TaskSchedulerEnableRateLimiterShadowMode indicates if task scheduler rate limiter should run in shadow mode
i.e. through rate limiter and emit metrics but do not actually block/throttle task scheduling`,
	}
	TaskSchedulerRateLimiterStartupDelay = &DurationGlobalSetting{
		Key:         "history.taskSchedulerRateLimiterStartupDelay",
		Default:     5 * time.Second,
		Description: `TaskSchedulerRateLimiterStartupDelay is the duration to wait after startup before enforcing task scheduler rate limiting`,
	}
	TaskSchedulerGlobalMaxQPS = &IntGlobalSetting{
		Key:     "history.taskSchedulerGlobalMaxQPS",
		Default: 0,
		Description: `TaskSchedulerGlobalMaxQPS is the max qps all task schedulers in the cluster can schedule tasks
If value less or equal to 0, will fall back to TaskSchedulerMaxQPS`,
	}
	TaskSchedulerMaxQPS = &IntGlobalSetting{
		Key:     "history.taskSchedulerMaxQPS",
		Default: 0,
		Description: `TaskSchedulerMaxQPS is the max qps task schedulers on a host can schedule tasks
If value less or equal to 0, will fall back to HistoryPersistenceMaxQPS`,
	}
	TaskSchedulerGlobalNamespaceMaxQPS = &IntNamespaceSetting{
		Key:     "history.taskSchedulerGlobalNamespaceMaxQPS",
		Default: 0,
		Description: `TaskSchedulerGlobalNamespaceMaxQPS is the max qps all task schedulers in the cluster can schedule tasks for a certain namespace
If value less or equal to 0, will fall back to TaskSchedulerNamespaceMaxQPS`,
	}
	TaskSchedulerNamespaceMaxQPS = &IntNamespaceSetting{
		Key:     "history.taskSchedulerNamespaceMaxQPS",
		Default: 0,
		Description: `TaskSchedulerNamespaceMaxQPS is the max qps task schedulers on a host can schedule tasks for a certain namespace
If value less or equal to 0, will fall back to HistoryPersistenceNamespaceMaxQPS`,
	}

	TimerTaskBatchSize = &IntGlobalSetting{
		Key:         "history.timerTaskBatchSize",
		Default:     100,
		Description: `TimerTaskBatchSize is batch size for timer processor to process tasks`,
	}
	TimerProcessorSchedulerWorkerCount = &IntGlobalSetting{
		Key:         "history.timerProcessorSchedulerWorkerCount",
		Default:     512,
		Description: `TimerProcessorSchedulerWorkerCount is the number of workers in the host level task scheduler for timer processor`,
	}
	TimerProcessorSchedulerActiveRoundRobinWeights = &MapNamespaceSetting{
		Key:         "history.timerProcessorSchedulerActiveRoundRobinWeights",
		Default:     ConvertWeightsToDynamicConfigValue(DefaultActiveTaskPriorityWeight),
		Description: `TimerProcessorSchedulerActiveRoundRobinWeights is the priority round robin weights used by timer task scheduler for active namespaces`,
	}
	TimerProcessorSchedulerStandbyRoundRobinWeights = &MapNamespaceSetting{
		Key:         "history.timerProcessorSchedulerStandbyRoundRobinWeights",
		Default:     ConvertWeightsToDynamicConfigValue(DefaultStandbyTaskPriorityWeight),
		Description: `TimerProcessorSchedulerStandbyRoundRobinWeights is the priority round robin weights used by timer task scheduler for standby namespaces`,
	}
	TimerProcessorUpdateAckInterval = &DurationGlobalSetting{
		Key:         "history.timerProcessorUpdateAckInterval",
		Default:     30 * time.Second,
		Description: `TimerProcessorUpdateAckInterval is update interval for timer processor`,
	}
	TimerProcessorUpdateAckIntervalJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.timerProcessorUpdateAckIntervalJitterCoefficient",
		Default:     0.15,
		Description: `TimerProcessorUpdateAckIntervalJitterCoefficient is the update interval jitter coefficient`,
	}
	TimerProcessorMaxPollRPS = &IntGlobalSetting{
		Key:         "history.timerProcessorMaxPollRPS",
		Default:     20,
		Description: `TimerProcessorMaxPollRPS is max poll rate per second for timer processor`,
	}
	TimerProcessorMaxPollHostRPS = &IntGlobalSetting{
		Key:         "history.timerProcessorMaxPollHostRPS",
		Default:     0,
		Description: `TimerProcessorMaxPollHostRPS is max poll rate per second for all timer processor on a host`,
	}
	TimerProcessorMaxPollInterval = &DurationGlobalSetting{
		Key:         "history.timerProcessorMaxPollInterval",
		Default:     5 * time.Minute,
		Description: `TimerProcessorMaxPollInterval is max poll interval for timer processor`,
	}
	TimerProcessorMaxPollIntervalJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.timerProcessorMaxPollIntervalJitterCoefficient",
		Default:     0.15,
		Description: `TimerProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	}
	TimerProcessorPollBackoffInterval = &DurationGlobalSetting{
		Key:         "history.timerProcessorPollBackoffInterval",
		Default:     5 * time.Second,
		Description: `TimerProcessorPollBackoffInterval is the poll backoff interval if task redispatcher's size exceeds limit for timer processor`,
	}
	TimerProcessorMaxTimeShift = &DurationGlobalSetting{
		Key:         "history.timerProcessorMaxTimeShift",
		Default:     1 * time.Second,
		Description: `TimerProcessorMaxTimeShift is the max shift timer processor can have`,
	}
	TimerQueueMaxReaderCount = &IntGlobalSetting{
		Key:         "history.timerQueueMaxReaderCount",
		Default:     2,
		Description: `TimerQueueMaxReaderCount is the max number of readers in one multi-cursor timer queue`,
	}
	RetentionTimerJitterDuration = &DurationGlobalSetting{
		Key:         "history.retentionTimerJitterDuration",
		Default:     30 * time.Minute,
		Description: `RetentionTimerJitterDuration is a time duration jitter to distribute timer from T0 to T0 + jitter duration`,
	}

	MemoryTimerProcessorSchedulerWorkerCount = &IntGlobalSetting{
		Key:         "history.memoryTimerProcessorSchedulerWorkerCount",
		Default:     64,
		Description: `MemoryTimerProcessorSchedulerWorkerCount is the number of workers in the task scheduler for in memory timer processor.`,
	}

	TransferTaskBatchSize = &IntGlobalSetting{
		Key:         "history.transferTaskBatchSize",
		Default:     100,
		Description: `TransferTaskBatchSize is batch size for transferQueueProcessor`,
	}
	TransferProcessorMaxPollRPS = &IntGlobalSetting{
		Key:         "history.transferProcessorMaxPollRPS",
		Default:     20,
		Description: `TransferProcessorMaxPollRPS is max poll rate per second for transferQueueProcessor`,
	}
	TransferProcessorMaxPollHostRPS = &IntGlobalSetting{
		Key:         "history.transferProcessorMaxPollHostRPS",
		Default:     0,
		Description: `TransferProcessorMaxPollHostRPS is max poll rate per second for all transferQueueProcessor on a host`,
	}
	TransferProcessorSchedulerWorkerCount = &IntGlobalSetting{
		Key:         "history.transferProcessorSchedulerWorkerCount",
		Default:     512,
		Description: `TransferProcessorSchedulerWorkerCount is the number of workers in the host level task scheduler for transferQueueProcessor`,
	}
	TransferProcessorSchedulerActiveRoundRobinWeights = &MapNamespaceSetting{
		Key:         "history.transferProcessorSchedulerActiveRoundRobinWeights",
		Default:     ConvertWeightsToDynamicConfigValue(DefaultActiveTaskPriorityWeight),
		Description: `TransferProcessorSchedulerActiveRoundRobinWeights is the priority round robin weights used by transfer task scheduler for active namespaces`,
	}
	TransferProcessorSchedulerStandbyRoundRobinWeights = &MapNamespaceSetting{
		Key:         "history.transferProcessorSchedulerStandbyRoundRobinWeights",
		Default:     ConvertWeightsToDynamicConfigValue(DefaultStandbyTaskPriorityWeight),
		Description: `TransferProcessorSchedulerStandbyRoundRobinWeights is the priority round robin weights used by transfer task scheduler for standby namespaces`,
	}
	TransferProcessorMaxPollInterval = &DurationGlobalSetting{
		Key:         "history.transferProcessorMaxPollInterval",
		Default:     1 * time.Minute,
		Description: `TransferProcessorMaxPollInterval max poll interval for transferQueueProcessor`,
	}
	TransferProcessorMaxPollIntervalJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.transferProcessorMaxPollIntervalJitterCoefficient",
		Default:     0.15,
		Description: `TransferProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	}
	TransferProcessorUpdateAckInterval = &DurationGlobalSetting{
		Key:         "history.transferProcessorUpdateAckInterval",
		Default:     30 * time.Second,
		Description: `TransferProcessorUpdateAckInterval is update interval for transferQueueProcessor`,
	}
	TransferProcessorUpdateAckIntervalJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.transferProcessorUpdateAckIntervalJitterCoefficient",
		Default:     0.15,
		Description: `TransferProcessorUpdateAckIntervalJitterCoefficient is the update interval jitter coefficient`,
	}
	TransferProcessorPollBackoffInterval = &DurationGlobalSetting{
		Key:         "history.transferProcessorPollBackoffInterval",
		Default:     5 * time.Second,
		Description: `TransferProcessorPollBackoffInterval is the poll backoff interval if task redispatcher's size exceeds limit for transferQueueProcessor`,
	}
	TransferProcessorEnsureCloseBeforeDelete = &BoolGlobalSetting{
		Key:         "history.transferProcessorEnsureCloseBeforeDelete",
		Default:     true,
		Description: `TransferProcessorEnsureCloseBeforeDelete means we ensure the execution is closed before we delete it`,
	}
	TransferQueueMaxReaderCount = &IntGlobalSetting{
		Key:         "history.transferQueueMaxReaderCount",
		Default:     2,
		Description: `TransferQueueMaxReaderCount is the max number of readers in one multi-cursor transfer queue`,
	}

	OutboundProcessorEnabled = &BoolGlobalSetting{
		Key:         "history.outboundProcessorEnabled",
		Default:     false,
		Description: `OutboundProcessorEnabled enables starting the outbound queue processor.`,
	}
	OutboundTaskBatchSize = &IntGlobalSetting{
		Key:         "history.outboundTaskBatchSize",
		Default:     100,
		Description: `OutboundTaskBatchSize is batch size for outboundQueueFactory`,
	}
	OutboundProcessorMaxPollRPS = &IntGlobalSetting{
		Key:         "history.outboundProcessorMaxPollRPS",
		Default:     20,
		Description: `OutboundProcessorMaxPollRPS is max poll rate per second for outboundQueueFactory`,
	}
	OutboundProcessorMaxPollHostRPS = &IntGlobalSetting{
		Key:         "history.outboundProcessorMaxPollHostRPS",
		Default:     0,
		Description: `OutboundProcessorMaxPollHostRPS is max poll rate per second for all outboundQueueFactory on a host`,
	}
	// FIXME: unused?
	// // OutboundProcessorUpdateShardTaskCount is update shard count for outboundQueueFactory
	// OutboundProcessorUpdateShardTaskCount = "history.outboundProcessorUpdateShardTaskCount"
	OutboundProcessorMaxPollInterval = &DurationGlobalSetting{
		Key:         "history.outboundProcessorMaxPollInterval",
		Default:     1 * time.Minute,
		Description: `OutboundProcessorMaxPollInterval max poll interval for outboundQueueFactory`,
	}
	OutboundProcessorMaxPollIntervalJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.outboundProcessorMaxPollIntervalJitterCoefficient",
		Default:     0.15,
		Description: `OutboundProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	}
	OutboundProcessorUpdateAckInterval = &DurationGlobalSetting{
		Key:         "history.outboundProcessorUpdateAckInterval",
		Default:     30 * time.Second,
		Description: `OutboundProcessorUpdateAckInterval is update interval for outboundQueueFactory`,
	}
	OutboundProcessorUpdateAckIntervalJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.outboundProcessorUpdateAckIntervalJitterCoefficient",
		Default:     0.15,
		Description: `OutboundProcessorUpdateAckIntervalJitterCoefficient is the update interval jitter coefficient`,
	}
	OutboundProcessorPollBackoffInterval = &DurationGlobalSetting{
		Key:         "history.outboundProcessorPollBackoffInterval",
		Default:     5 * time.Second,
		Description: `OutboundProcessorPollBackoffInterval is the poll backoff interval if task redispatcher's size exceeds limit for outboundQueueFactory`,
	}
	OutboundQueueMaxReaderCount = &IntGlobalSetting{
		Key:         "history.outboundQueueMaxReaderCount",
		Default:     4,
		Description: `OutboundQueueMaxReaderCount is the max number of readers in one multi-cursor outbound queue`,
	}

	VisibilityTaskBatchSize = &IntGlobalSetting{
		Key:         "history.visibilityTaskBatchSize",
		Default:     100,
		Description: `VisibilityTaskBatchSize is batch size for visibilityQueueProcessor`,
	}
	VisibilityProcessorMaxPollRPS = &IntGlobalSetting{
		Key:         "history.visibilityProcessorMaxPollRPS",
		Default:     20,
		Description: `VisibilityProcessorMaxPollRPS is max poll rate per second for visibilityQueueProcessor`,
	}
	VisibilityProcessorMaxPollHostRPS = &IntGlobalSetting{
		Key:         "history.visibilityProcessorMaxPollHostRPS",
		Default:     0,
		Description: `VisibilityProcessorMaxPollHostRPS is max poll rate per second for all visibilityQueueProcessor on a host`,
	}
	VisibilityProcessorSchedulerWorkerCount = &IntGlobalSetting{
		Key:         "history.visibilityProcessorSchedulerWorkerCount",
		Default:     512,
		Description: `VisibilityProcessorSchedulerWorkerCount is the number of workers in the host level task scheduler for visibilityQueueProcessor`,
	}
	VisibilityProcessorSchedulerActiveRoundRobinWeights = &MapNamespaceSetting{
		Key:         "history.visibilityProcessorSchedulerActiveRoundRobinWeights",
		Default:     ConvertWeightsToDynamicConfigValue(DefaultActiveTaskPriorityWeight),
		Description: `VisibilityProcessorSchedulerActiveRoundRobinWeights is the priority round robin weights by visibility task scheduler for active namespaces`,
	}
	VisibilityProcessorSchedulerStandbyRoundRobinWeights = &MapNamespaceSetting{
		Key:         "history.visibilityProcessorSchedulerStandbyRoundRobinWeights",
		Default:     ConvertWeightsToDynamicConfigValue(DefaultStandbyTaskPriorityWeight),
		Description: `VisibilityProcessorSchedulerStandbyRoundRobinWeights is the priority round robin weights by visibility task scheduler for standby namespaces`,
	}
	VisibilityProcessorMaxPollInterval = &DurationGlobalSetting{
		Key:         "history.visibilityProcessorMaxPollInterval",
		Default:     1 * time.Minute,
		Description: `VisibilityProcessorMaxPollInterval max poll interval for visibilityQueueProcessor`,
	}
	VisibilityProcessorMaxPollIntervalJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.visibilityProcessorMaxPollIntervalJitterCoefficient",
		Default:     0.15,
		Description: `VisibilityProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	}
	VisibilityProcessorUpdateAckInterval = &DurationGlobalSetting{
		Key:         "history.visibilityProcessorUpdateAckInterval",
		Default:     30 * time.Second,
		Description: `VisibilityProcessorUpdateAckInterval is update interval for visibilityQueueProcessor`,
	}
	VisibilityProcessorUpdateAckIntervalJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.visibilityProcessorUpdateAckIntervalJitterCoefficient",
		Default:     0.15,
		Description: `VisibilityProcessorUpdateAckIntervalJitterCoefficient is the update interval jitter coefficient`,
	}
	VisibilityProcessorPollBackoffInterval = &DurationGlobalSetting{
		Key:         "history.visibilityProcessorPollBackoffInterval",
		Default:     5 * time.Second,
		Description: `VisibilityProcessorPollBackoffInterval is the poll backoff interval if task redispatcher's size exceeds limit for visibilityQueueProcessor`,
	}
	VisibilityProcessorEnsureCloseBeforeDelete = &BoolGlobalSetting{
		Key:         "history.visibilityProcessorEnsureCloseBeforeDelete",
		Default:     false,
		Description: `VisibilityProcessorEnsureCloseBeforeDelete means we ensure the visibility of an execution is closed before we delete its visibility records`,
	}
	VisibilityProcessorEnableCloseWorkflowCleanup = &BoolNamespaceSetting{
		Key:     "history.visibilityProcessorEnableCloseWorkflowCleanup",
		Default: false,
		Description: `VisibilityProcessorEnableCloseWorkflowCleanup to clean up the mutable state after visibility
close task has been processed. Must use Elasticsearch as visibility store, otherwise workflow
data (eg: search attributes) will be lost after workflow is closed.`,
	}
	VisibilityQueueMaxReaderCount = &IntGlobalSetting{
		Key:         "history.visibilityQueueMaxReaderCount",
		Default:     2,
		Description: `VisibilityQueueMaxReaderCount is the max number of readers in one multi-cursor visibility queue`,
	}

	ArchivalTaskBatchSize = &IntGlobalSetting{
		Key:         "history.archivalTaskBatchSize",
		Default:     100,
		Description: `ArchivalTaskBatchSize is batch size for archivalQueueProcessor`,
	}
	ArchivalProcessorMaxPollRPS = &IntGlobalSetting{
		Key:         "history.archivalProcessorMaxPollRPS",
		Default:     20,
		Description: `ArchivalProcessorMaxPollRPS is max poll rate per second for archivalQueueProcessor`,
	}
	ArchivalProcessorMaxPollHostRPS = &IntGlobalSetting{
		Key:         "history.archivalProcessorMaxPollHostRPS",
		Default:     0,
		Description: `ArchivalProcessorMaxPollHostRPS is max poll rate per second for all archivalQueueProcessor on a host`,
	}
	ArchivalProcessorSchedulerWorkerCount = &IntGlobalSetting{
		Key:     "history.archivalProcessorSchedulerWorkerCount",
		Default: 512,
		Description: `ArchivalProcessorSchedulerWorkerCount is the number of workers in the host level task scheduler for
archivalQueueProcessor`,
	}
	ArchivalProcessorMaxPollInterval = &DurationGlobalSetting{
		Key:         "history.archivalProcessorMaxPollInterval",
		Default:     5 * time.Minute,
		Description: `ArchivalProcessorMaxPollInterval max poll interval for archivalQueueProcessor`,
	}
	ArchivalProcessorMaxPollIntervalJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.archivalProcessorMaxPollIntervalJitterCoefficient",
		Default:     0.15,
		Description: `ArchivalProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	}
	ArchivalProcessorUpdateAckInterval = &DurationGlobalSetting{
		Key:         "history.archivalProcessorUpdateAckInterval",
		Default:     30 * time.Second,
		Description: `ArchivalProcessorUpdateAckInterval is update interval for archivalQueueProcessor`,
	}
	ArchivalProcessorUpdateAckIntervalJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.archivalProcessorUpdateAckIntervalJitterCoefficient",
		Default:     0.15,
		Description: `ArchivalProcessorUpdateAckIntervalJitterCoefficient is the update interval jitter coefficient`,
	}
	ArchivalProcessorPollBackoffInterval = &DurationGlobalSetting{
		Key:     "history.archivalProcessorPollBackoffInterval",
		Default: 5 * time.Second,
		Description: `ArchivalProcessorPollBackoffInterval is the poll backoff interval if task redispatcher's size exceeds limit for
archivalQueueProcessor`,
	}
	ArchivalProcessorArchiveDelay = &DurationGlobalSetting{
		Key:         "history.archivalProcessorArchiveDelay",
		Default:     5 * time.Minute,
		Description: `ArchivalProcessorArchiveDelay is the delay before archivalQueueProcessor starts to process archival tasks`,
	}
	ArchivalBackendMaxRPS = &Float64GlobalSetting{
		Key:         "history.archivalBackendMaxRPS",
		Default:     10000.0,
		Description: `ArchivalBackendMaxRPS is the maximum rate of requests per second to the archival backend`,
	}
	ArchivalQueueMaxReaderCount = &IntGlobalSetting{
		Key:         "history.archivalQueueMaxReaderCount",
		Default:     2,
		Description: `ArchivalQueueMaxReaderCount is the max number of readers in one multi-cursor archival queue`,
	}

	WorkflowExecutionMaxInFlightUpdates = &IntNamespaceSetting{
		Key:         "history.maxInFlightUpdates",
		Default:     10,
		Description: `WorkflowExecutionMaxInFlightUpdates is the max number of updates that can be in-flight (admitted but not yet completed) for any given workflow execution.`,
	}
	WorkflowExecutionMaxTotalUpdates = &IntNamespaceSetting{
		Key:         "history.maxTotalUpdates",
		Default:     2000,
		Description: `WorkflowExecutionMaxTotalUpdates is the max number of updates that any given workflow execution can receive.`,
	}

	ReplicatorTaskBatchSize = &IntGlobalSetting{
		Key:         "history.replicatorTaskBatchSize",
		Default:     25,
		Description: `ReplicatorTaskBatchSize is batch size for ReplicatorProcessor`,
	}
	ReplicatorMaxSkipTaskCount = &IntGlobalSetting{
		Key:         "history.replicatorMaxSkipTaskCount",
		Default:     250,
		Description: `ReplicatorMaxSkipTaskCount is maximum number of tasks that can be skipped during tasks pagination due to not meeting filtering conditions (e.g. missed namespace).`,
	}
	ReplicatorProcessorMaxPollInterval = &DurationGlobalSetting{
		Key:         "history.replicatorProcessorMaxPollInterval",
		Default:     1 * time.Minute,
		Description: `ReplicatorProcessorMaxPollInterval is max poll interval for ReplicatorProcessor`,
	}
	ReplicatorProcessorMaxPollIntervalJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.replicatorProcessorMaxPollIntervalJitterCoefficient",
		Default:     0.15,
		Description: `ReplicatorProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	}
	MaximumBufferedEventsBatch = &IntGlobalSetting{
		Key:         "history.maximumBufferedEventsBatch",
		Default:     100,
		Description: `MaximumBufferedEventsBatch is the maximum permissible number of buffered events for any given mutable state.`,
	}
	MaximumBufferedEventsSizeInBytes = &IntGlobalSetting{
		Key:     "history.maximumBufferedEventsSizeInBytes",
		Default: 2 * 1024 * 1024,
		Description: `MaximumBufferedEventsSizeInBytes is the maximum permissible size of all buffered events for any given mutable
state. The total size is determined by the sum of the size, in bytes, of each HistoryEvent proto.`,
	}
	MaximumSignalsPerExecution = &IntNamespaceSetting{
		Key:         "history.maximumSignalsPerExecution",
		Default:     10000,
		Description: `MaximumSignalsPerExecution is max number of signals supported by single execution`,
	}
	ShardUpdateMinInterval = &DurationGlobalSetting{
		Key:         "history.shardUpdateMinInterval",
		Default:     5 * time.Minute,
		Description: `ShardUpdateMinInterval is the minimal time interval which the shard info can be updated`,
	}
	ShardUpdateMinTasksCompleted = &IntGlobalSetting{
		Key:     "history.shardUpdateMinTasksCompleted",
		Default: 1000,
		Description: `ShardUpdateMinTasksCompleted is the minimum number of tasks which must be completed (across all queues) before the shard info can be updated.
Note that once history.shardUpdateMinInterval amount of time has passed we'll update the shard info regardless of the number of tasks completed.
When the this config is zero or lower we will only update shard info at most once every history.shardUpdateMinInterval.`,
	}
	ShardSyncMinInterval = &DurationGlobalSetting{
		Key:         "history.shardSyncMinInterval",
		Default:     5 * time.Minute,
		Description: `ShardSyncMinInterval is the minimal time interval which the shard info should be sync to remote`,
	}
	EmitShardLagLog = &BoolGlobalSetting{
		Key:         "history.emitShardLagLog",
		Default:     false,
		Description: `EmitShardLagLog whether emit the shard lag log`,
	}
	DefaultEventEncoding = &StringNamespaceSetting{
		Key:         "history.defaultEventEncoding",
		Default:     enumspb.ENCODING_TYPE_PROTO3.String(),
		Description: `DefaultEventEncoding is the encoding type for history events`,
	}
	DefaultActivityRetryPolicy = &MapNamespaceSetting{
		Key:     "history.defaultActivityRetryPolicy",
		Default: common.GetDefaultRetryPolicyConfigOptions(),
		Description: `DefaultActivityRetryPolicy represents the out-of-box retry policy for activities where
the user has not specified an explicit RetryPolicy`,
	}
	DefaultWorkflowRetryPolicy = &MapNamespaceSetting{
		Key:     "history.defaultWorkflowRetryPolicy",
		Default: common.GetDefaultRetryPolicyConfigOptions(),
		Description: `DefaultWorkflowRetryPolicy represents the out-of-box retry policy for unset fields
where the user has set an explicit RetryPolicy, but not specified all the fields`,
	}
	HistoryMaxAutoResetPoints = &IntNamespaceSetting{
		Key:         "history.historyMaxAutoResetPoints",
		Default:     DefaultHistoryMaxAutoResetPoints,
		Description: `HistoryMaxAutoResetPoints is the key for max number of auto reset points stored in mutableState`,
	}
	EnableParentClosePolicy = &BoolNamespaceSetting{
		Key:         "history.enableParentClosePolicy",
		Default:     true,
		Description: `EnableParentClosePolicy whether to  ParentClosePolicy`,
	}
	ParentClosePolicyThreshold = &IntNamespaceSetting{
		Key:     "history.parentClosePolicyThreshold",
		Default: 10,
		Description: `ParentClosePolicyThreshold decides that parent close policy will be processed by sys workers(if enabled) if
the number of children greater than or equal to this threshold`,
	}
	NumParentClosePolicySystemWorkflows = &IntGlobalSetting{
		Key:         "history.numParentClosePolicySystemWorkflows",
		Default:     10,
		Description: `NumParentClosePolicySystemWorkflows is key for number of parentClosePolicy system workflows running in total`,
	}
	HistoryThrottledLogRPS = &IntGlobalSetting{
		Key:         "history.throttledLogRPS",
		Default:     4,
		Description: `HistoryThrottledLogRPS is the rate limit on number of log messages emitted per second for throttled logger`,
	}
	WorkflowTaskHeartbeatTimeout = &DurationNamespaceSetting{
		Key:         "history.workflowTaskHeartbeatTimeout",
		Default:     time.Minute * 30,
		Description: `WorkflowTaskHeartbeatTimeout for workflow task heartbeat`,
	}
	WorkflowTaskCriticalAttempts = &IntGlobalSetting{
		Key:         "history.workflowTaskCriticalAttempt",
		Default:     10,
		Description: `WorkflowTaskCriticalAttempts is the number of attempts for a workflow task that's regarded as critical`,
	}
	WorkflowTaskRetryMaxInterval = &DurationGlobalSetting{
		Key:         "history.workflowTaskRetryMaxInterval",
		Default:     time.Minute * 10,
		Description: `WorkflowTaskRetryMaxInterval is the maximum interval added to a workflow task's startToClose timeout for slowing down retry`,
	}
	DefaultWorkflowTaskTimeout = &DurationNamespaceSetting{
		Key:         "history.defaultWorkflowTaskTimeout",
		Default:     common.DefaultWorkflowTaskTimeout,
		Description: `DefaultWorkflowTaskTimeout for a workflow task`,
	}
	SkipReapplicationByNamespaceID = &BoolNamespaceIDSetting{
		Key:         "history.SkipReapplicationByNamespaceID",
		Default:     false,
		Description: `SkipReapplicationByNamespaceID is whether skipping a event re-application for a namespace`,
	}
	StandbyTaskReReplicationContextTimeout = &DurationNamespaceIDSetting{
		Key:         "history.standbyTaskReReplicationContextTimeout",
		Default:     30 * time.Second,
		Description: `StandbyTaskReReplicationContextTimeout is the context timeout for standby task re-replication`,
	}
	MaxBufferedQueryCount = &IntGlobalSetting{
		Key:         "history.MaxBufferedQueryCount",
		Default:     1,
		Description: `MaxBufferedQueryCount indicates max buffer query count`,
	}
	MutableStateChecksumGenProbability = &IntNamespaceSetting{
		Key:         "history.mutableStateChecksumGenProbability",
		Default:     0,
		Description: `MutableStateChecksumGenProbability is the probability [0-100] that checksum will be generated for mutable state`,
	}
	MutableStateChecksumVerifyProbability = &IntNamespaceSetting{
		Key:         "history.mutableStateChecksumVerifyProbability",
		Default:     0,
		Description: `MutableStateChecksumVerifyProbability is the probability [0-100] that checksum will be verified for mutable state`,
	}
	MutableStateChecksumInvalidateBefore = &Float64GlobalSetting{
		Key:         "history.mutableStateChecksumInvalidateBefore",
		Default:     0,
		Description: `MutableStateChecksumInvalidateBefore is the epoch timestamp before which all checksums are to be discarded`,
	}

	ReplicationTaskFetcherParallelism = &IntGlobalSetting{
		Key:         "history.ReplicationTaskFetcherParallelism",
		Default:     4,
		Description: `ReplicationTaskFetcherParallelism determines how many go routines we spin up for fetching tasks`,
	}
	ReplicationTaskFetcherAggregationInterval = &DurationGlobalSetting{
		Key:         "history.ReplicationTaskFetcherAggregationInterval",
		Default:     2 * time.Second,
		Description: `ReplicationTaskFetcherAggregationInterval determines how frequently the fetch requests are sent`,
	}
	ReplicationTaskFetcherTimerJitterCoefficient = &Float64GlobalSetting{
		Key:         "history.ReplicationTaskFetcherTimerJitterCoefficient",
		Default:     0.15,
		Description: `ReplicationTaskFetcherTimerJitterCoefficient is the jitter for fetcher timer`,
	}
	ReplicationTaskFetcherErrorRetryWait = &DurationGlobalSetting{
		Key:         "history.ReplicationTaskFetcherErrorRetryWait",
		Default:     time.Second,
		Description: `ReplicationTaskFetcherErrorRetryWait is the wait time when fetcher encounters error`,
	}
	ReplicationTaskProcessorErrorRetryWait = &DurationShardIDSetting{
		Key:         "history.ReplicationTaskProcessorErrorRetryWait",
		Default:     1 * time.Second,
		Description: `ReplicationTaskProcessorErrorRetryWait is the initial retry wait when we see errors in applying replication tasks`,
	}
	ReplicationTaskProcessorErrorRetryBackoffCoefficient = &Float64ShardIDSetting{
		Key:         "history.ReplicationTaskProcessorErrorRetryBackoffCoefficient",
		Default:     1.2,
		Description: `ReplicationTaskProcessorErrorRetryBackoffCoefficient is the retry wait backoff time coefficient`,
	}
	ReplicationTaskProcessorErrorRetryMaxInterval = &DurationShardIDSetting{
		Key:         "history.ReplicationTaskProcessorErrorRetryMaxInterval",
		Default:     5 * time.Second,
		Description: `ReplicationTaskProcessorErrorRetryMaxInterval is the retry wait backoff max duration`,
	}
	ReplicationTaskProcessorErrorRetryMaxAttempts = &IntShardIDSetting{
		Key:         "history.ReplicationTaskProcessorErrorRetryMaxAttempts",
		Default:     80,
		Description: `ReplicationTaskProcessorErrorRetryMaxAttempts is the max retry attempts for applying replication tasks`,
	}
	ReplicationTaskProcessorErrorRetryExpiration = &DurationShardIDSetting{
		Key:         "history.ReplicationTaskProcessorErrorRetryExpiration",
		Default:     5 * time.Minute,
		Description: `ReplicationTaskProcessorErrorRetryExpiration is the max retry duration for applying replication tasks`,
	}
	ReplicationTaskProcessorNoTaskInitialWait = &DurationShardIDSetting{
		Key:         "history.ReplicationTaskProcessorNoTaskInitialWait",
		Default:     2 * time.Second,
		Description: `ReplicationTaskProcessorNoTaskInitialWait is the wait time when not ask is returned`,
	}
	ReplicationTaskProcessorCleanupInterval = &DurationShardIDSetting{
		Key:         "history.ReplicationTaskProcessorCleanupInterval",
		Default:     1 * time.Minute,
		Description: `ReplicationTaskProcessorCleanupInterval determines how frequently the cleanup replication queue`,
	}
	ReplicationTaskProcessorCleanupJitterCoefficient = &Float64ShardIDSetting{
		Key:         "history.ReplicationTaskProcessorCleanupJitterCoefficient",
		Default:     0.15,
		Description: `ReplicationTaskProcessorCleanupJitterCoefficient is the jitter for cleanup timer`,
	}
	// FIXME: unused?
	// // ReplicationTaskProcessorStartWait is the wait time before each task processing batch
	// ReplicationTaskProcessorStartWait = "history.ReplicationTaskProcessorStartWait"
	ReplicationTaskProcessorHostQPS = &Float64GlobalSetting{
		Key:         "history.ReplicationTaskProcessorHostQPS",
		Default:     1500,
		Description: `ReplicationTaskProcessorHostQPS is the qps of task processing rate limiter on host level`,
	}
	ReplicationTaskProcessorShardQPS = &Float64GlobalSetting{
		Key:         "history.ReplicationTaskProcessorShardQPS",
		Default:     30,
		Description: `ReplicationTaskProcessorShardQPS is the qps of task processing rate limiter on shard level`,
	}
	ReplicationEnableDLQMetrics = &BoolGlobalSetting{
		Key:         "history.ReplicationEnableDLQMetrics",
		Default:     true,
		Description: `ReplicationEnableDLQMetrics is the flag to emit DLQ metrics`,
	}
	ReplicationEnableUpdateWithNewTaskMerge = &BoolGlobalSetting{
		Key:     "history.ReplicationEnableUpdateWithNewTaskMerge",
		Default: false,
		Description: `ReplicationEnableUpdateWithNewTaskMerge is the flag controlling whether replication task merging logic
should be enabled for non continuedAsNew workflow UpdateWithNew case.`,
	}
	HistoryTaskDLQEnabled = &BoolGlobalSetting{
		Key:     "history.TaskDLQEnabled",
		Default: true,
		Description: `HistoryTaskDLQEnabled enables the history task DLQ. This applies to internal tasks like transfer and timer tasks.
Do not turn this on if you aren't using Cassandra as the history task DLQ is not implemented for other databases.`,
	}
	HistoryTaskDLQUnexpectedErrorAttempts = &IntGlobalSetting{
		Key:         "history.TaskDLQUnexpectedErrorAttempts",
		Default:     100,
		Description: `HistoryTaskDLQUnexpectedErrorAttempts is the number of task execution attempts before sending the task to DLQ.`,
	}
	HistoryTaskDLQInternalErrors = &BoolGlobalSetting{
		Key:     "history.TaskDLQInternalErrors",
		Default: false,
		Description: `HistoryTaskDLQInternalErrors causes history task processing to send tasks failing with serviceerror.Internal to
the dlq (or will drop them if not enabled)`,
	}
	HistoryTaskDLQErrorPattern = &StringGlobalSetting{
		Key:     "history.TaskDLQErrorPattern",
		Default: "",
		Description: `HistoryTaskDLQErrorPattern specifies a regular expression. If a task processing error matches with this regex,
that task will be sent to DLQ.`,
	}

	ReplicationStreamSyncStatusDuration = &DurationGlobalSetting{
		Key:         "history.ReplicationStreamSyncStatusDuration",
		Default:     1 * time.Second,
		Description: `ReplicationStreamSyncStatusDuration sync replication status duration`,
	}
	// FIXME: unused?
	// // ReplicationStreamMinReconnectDuration minimal replication stream reconnection duration
	// ReplicationStreamMinReconnectDuration = "history.ReplicationStreamMinReconnectDuration"
	ReplicationProcessorSchedulerQueueSize = &IntGlobalSetting{
		Key:         "history.ReplicationProcessorSchedulerQueueSize",
		Default:     128,
		Description: `ReplicationProcessorSchedulerQueueSize is the replication task executor queue size`,
	}
	ReplicationProcessorSchedulerWorkerCount = &IntGlobalSetting{
		Key:         "history.ReplicationProcessorSchedulerWorkerCount",
		Default:     512,
		Description: `ReplicationProcessorSchedulerWorkerCount is the replication task executor worker count`,
	}
	EnableEagerNamespaceRefresher = &BoolGlobalSetting{
		Key:         "history.EnableEagerNamespaceRefresher",
		Default:     false,
		Description: `EnableEagerNamespaceRefresher is a feature flag for eagerly refresh namespace during processing replication task`,
	}
	EnableReplicationTaskBatching = &BoolGlobalSetting{
		Key:         "history.EnableReplicationTaskBatching",
		Default:     false,
		Description: `EnableReplicationTaskBatching is a feature flag for batching replicate history event task`,
	}
	EnableReplicateLocalGeneratedEvents = &BoolGlobalSetting{
		Key:         "history.EnableReplicateLocalGeneratedEvents",
		Default:     false,
		Description: `EnableReplicateLocalGeneratedEvents is a feature flag for replicating locally generated events`,
	}

	// keys for worker

	WorkerPersistenceMaxQPS = &IntGlobalSetting{
		Key:         "worker.persistenceMaxQPS",
		Default:     500,
		Description: `WorkerPersistenceMaxQPS is the max qps worker host can query DB`,
	}
	WorkerPersistenceGlobalMaxQPS = &IntGlobalSetting{
		Key:         "worker.persistenceGlobalMaxQPS",
		Default:     0,
		Description: `WorkerPersistenceGlobalMaxQPS is the max qps worker cluster can query DB`,
	}
	WorkerPersistenceNamespaceMaxQPS = &IntNamespaceSetting{
		Key:         "worker.persistenceNamespaceMaxQPS",
		Default:     0,
		Description: `WorkerPersistenceNamespaceMaxQPS is the max qps each namespace on worker host can query DB`,
	}
	WorkerPersistenceGlobalNamespaceMaxQPS = &IntNamespaceSetting{
		Key:         "worker.persistenceGlobalNamespaceMaxQPS",
		Default:     0,
		Description: `WorkerPersistenceNamespaceMaxQPS is the max qps each namespace in worker cluster can query DB`,
	}
	WorkerEnablePersistencePriorityRateLimiting = &BoolGlobalSetting{
		Key:         "worker.enablePersistencePriorityRateLimiting",
		Default:     true,
		Description: `WorkerEnablePersistencePriorityRateLimiting indicates if priority rate limiting is enabled in worker persistence client`,
	}
	WorkerPersistenceDynamicRateLimitingParams = &MapGlobalSetting{
		Key:     "worker.persistenceDynamicRateLimitingParams",
		Default: dynamicconfig.DefaultDynamicRateLimitingParams,
		Description: `WorkerPersistenceDynamicRateLimitingParams is a map that contains all adjustable dynamic rate limiting params
see DefaultDynamicRateLimitingParams for available options and defaults`,
	}
	WorkerIndexerConcurrency = &IntGlobalSetting{
		Key:         "worker.indexerConcurrency",
		Default:     100,
		Description: `WorkerIndexerConcurrency is the max concurrent messages to be processed at any given time`,
	}
	WorkerESProcessorNumOfWorkers = &IntGlobalSetting{
		Key:         "worker.ESProcessorNumOfWorkers",
		Default:     2,
		Description: `WorkerESProcessorNumOfWorkers is num of workers for esProcessor`,
	}
	WorkerESProcessorBulkActions = &IntGlobalSetting{
		Key:         "worker.ESProcessorBulkActions",
		Default:     500,
		Description: `WorkerESProcessorBulkActions is max number of requests in bulk for esProcessor`,
	}
	WorkerESProcessorBulkSize = &IntGlobalSetting{
		Key:         "worker.ESProcessorBulkSize",
		Default:     16 * 1024 * 1024,
		Description: `WorkerESProcessorBulkSize is max total size of bulk in bytes for esProcessor`,
	}
	WorkerESProcessorFlushInterval = &DurationGlobalSetting{
		Key:         "worker.ESProcessorFlushInterval",
		Default:     1 * time.Second,
		Description: `WorkerESProcessorFlushInterval is flush interval for esProcessor`,
	}
	WorkerESProcessorAckTimeout = &DurationGlobalSetting{
		Key:     "worker.ESProcessorAckTimeout",
		Default: 30 * time.Second,
		Description: `WorkerESProcessorAckTimeout is the timeout that store will wait to get ack signal from ES processor.
Should be at least WorkerESProcessorFlushInterval+<time to process request>.`,
	}
	WorkerThrottledLogRPS = &IntGlobalSetting{
		Key:         "worker.throttledLogRPS",
		Default:     20,
		Description: `WorkerThrottledLogRPS is the rate limit on number of log messages emitted per second for throttled logger`,
	}
	WorkerScannerMaxConcurrentActivityExecutionSize = &IntGlobalSetting{
		Key:         "worker.ScannerMaxConcurrentActivityExecutionSize",
		Default:     10,
		Description: `WorkerScannerMaxConcurrentActivityExecutionSize indicates worker scanner max concurrent activity execution size`,
	}
	WorkerScannerMaxConcurrentWorkflowTaskExecutionSize = &IntGlobalSetting{
		Key:         "worker.ScannerMaxConcurrentWorkflowTaskExecutionSize",
		Default:     10,
		Description: `WorkerScannerMaxConcurrentWorkflowTaskExecutionSize indicates worker scanner max concurrent workflow execution size`,
	}
	WorkerScannerMaxConcurrentActivityTaskPollers = &IntGlobalSetting{
		Key:         "worker.ScannerMaxConcurrentActivityTaskPollers",
		Default:     8,
		Description: `WorkerScannerMaxConcurrentActivityTaskPollers indicates worker scanner max concurrent activity pollers`,
	}
	WorkerScannerMaxConcurrentWorkflowTaskPollers = &IntGlobalSetting{
		Key:         "worker.ScannerMaxConcurrentWorkflowTaskPollers",
		Default:     8,
		Description: `WorkerScannerMaxConcurrentWorkflowTaskPollers indicates worker scanner max concurrent workflow pollers`,
	}
	ScannerPersistenceMaxQPS = &IntGlobalSetting{
		Key:         "worker.scannerPersistenceMaxQPS",
		Default:     100,
		Description: `ScannerPersistenceMaxQPS is the maximum rate of persistence calls from worker.Scanner`,
	}
	ExecutionScannerPerHostQPS = &IntGlobalSetting{
		Key:         "worker.executionScannerPerHostQPS",
		Default:     10,
		Description: `ExecutionScannerPerHostQPS is the maximum rate of calls per host from executions.Scanner`,
	}
	ExecutionScannerPerShardQPS = &IntGlobalSetting{
		Key:         "worker.executionScannerPerShardQPS",
		Default:     1,
		Description: `ExecutionScannerPerShardQPS is the maximum rate of calls per shard from executions.Scanner`,
	}
	ExecutionDataDurationBuffer = &DurationGlobalSetting{
		Key:         "worker.executionDataDurationBuffer",
		Default:     time.Hour * 24 * 90,
		Description: `ExecutionDataDurationBuffer is the data TTL duration buffer of execution data`,
	}
	ExecutionScannerWorkerCount = &IntGlobalSetting{
		Key:         "worker.executionScannerWorkerCount",
		Default:     8,
		Description: `ExecutionScannerWorkerCount is the execution scavenger worker count`,
	}
	ExecutionScannerHistoryEventIdValidator = &BoolGlobalSetting{
		Key:         "worker.executionEnableHistoryEventIdValidator",
		Default:     true,
		Description: `ExecutionScannerHistoryEventIdValidator is the flag to enable history event id validator`,
	}
	TaskQueueScannerEnabled = &BoolGlobalSetting{
		Key:         "worker.taskQueueScannerEnabled",
		Default:     true,
		Description: `TaskQueueScannerEnabled indicates if task queue scanner should be started as part of worker.Scanner`,
	}
	BuildIdScavengerEnabled = &BoolGlobalSetting{
		Key:         "worker.buildIdScavengerEnabled",
		Default:     false,
		Description: `BuildIdScavengerEnabled indicates if the build id scavenger should be started as part of worker.Scanner`,
	}
	HistoryScannerEnabled = &BoolGlobalSetting{
		Key:         "worker.historyScannerEnabled",
		Default:     true,
		Description: `HistoryScannerEnabled indicates if history scanner should be started as part of worker.Scanner`,
	}
	ExecutionsScannerEnabled = &BoolGlobalSetting{
		Key:         "worker.executionsScannerEnabled",
		Default:     false,
		Description: `ExecutionsScannerEnabled indicates if executions scanner should be started as part of worker.Scanner`,
	}
	HistoryScannerDataMinAge = &DurationGlobalSetting{
		Key:         "worker.historyScannerDataMinAge",
		Default:     60 * 24 * time.Hour,
		Description: `HistoryScannerDataMinAge indicates the history scanner cleanup minimum age.`,
	}
	HistoryScannerVerifyRetention = &BoolGlobalSetting{
		Key:     "worker.historyScannerVerifyRetention",
		Default: true,
		Description: `HistoryScannerVerifyRetention indicates the history scanner verify data retention.
If the service configures with archival feature enabled, update worker.historyScannerVerifyRetention to be double of the data retention.`,
	}
	EnableBatcher = &BoolNamespaceSetting{
		Key:         "worker.enableBatcher",
		Default:     true,
		Description: `EnableBatcher decides whether start batcher in our worker`,
	}
	BatcherRPS = &IntNamespaceSetting{
		Key:         "worker.batcherRPS",
		Default:     DefaultRPS,
		Description: `BatcherRPS controls number the rps of batch operations`,
	}
	BatcherConcurrency = &IntNamespaceSetting{
		Key:         "worker.batcherConcurrency",
		Default:     DefaultConcurrency,
		Description: `BatcherConcurrency controls the concurrency of one batch operation`,
	}
	WorkerParentCloseMaxConcurrentActivityExecutionSize = &IntGlobalSetting{
		Key:         "worker.ParentCloseMaxConcurrentActivityExecutionSize",
		Default:     1000,
		Description: `WorkerParentCloseMaxConcurrentActivityExecutionSize indicates worker parent close worker max concurrent activity execution size`,
	}
	WorkerParentCloseMaxConcurrentWorkflowTaskExecutionSize = &IntGlobalSetting{
		Key:         "worker.ParentCloseMaxConcurrentWorkflowTaskExecutionSize",
		Default:     1000,
		Description: `WorkerParentCloseMaxConcurrentWorkflowTaskExecutionSize indicates worker parent close worker max concurrent workflow execution size`,
	}
	WorkerParentCloseMaxConcurrentActivityTaskPollers = &IntGlobalSetting{
		Key:         "worker.ParentCloseMaxConcurrentActivityTaskPollers",
		Default:     4,
		Description: `WorkerParentCloseMaxConcurrentActivityTaskPollers indicates worker parent close worker max concurrent activity pollers`,
	}
	WorkerParentCloseMaxConcurrentWorkflowTaskPollers = &IntGlobalSetting{
		Key:         "worker.ParentCloseMaxConcurrentWorkflowTaskPollers",
		Default:     4,
		Description: `WorkerParentCloseMaxConcurrentWorkflowTaskPollers indicates worker parent close worker max concurrent workflow pollers`,
	}
	WorkerPerNamespaceWorkerCount = &IntNamespaceSetting{
		Key:         "worker.perNamespaceWorkerCount",
		Default:     1,
		Description: `WorkerPerNamespaceWorkerCount controls number of per-ns (scheduler, batcher, etc.) workers to run per namespace`,
	}
	WorkerPerNamespaceWorkerOptions = &MapNamespaceSetting{
		Key:         "worker.perNamespaceWorkerOptions",
		Default:     map[string]any{},
		Description: `WorkerPerNamespaceWorkerOptions are SDK worker options for per-namespace worker`,
	}
	WorkerPerNamespaceWorkerStartRate = &FloatNamespaceSetting{
		Key:         "worker.perNamespaceWorkerStartRate",
		Default:     10.0,
		Description: `WorkerPerNamespaceWorkerStartRate controls how fast per-namespace workers can be started (workers/second)`,
	}
	WorkerEnableScheduler = &BoolNamespaceSetting{
		Key:         "worker.enableScheduler",
		Default:     true,
		Description: `WorkerEnableScheduler controls whether to start the worker for scheduled workflows`,
	}
	WorkerStickyCacheSize = &IntGlobalSetting{
		Key:     "worker.stickyCacheSize",
		Default: 0,
		Description: `WorkerStickyCacheSize controls the sticky cache size for SDK workers on worker nodes
(shared between all workers in the process, cannot be changed after startup)`,
	}
	SchedulerNamespaceStartWorkflowRPS = &FloatNamespaceSetting{
		Key:         "worker.schedulerNamespaceStartWorkflowRPS",
		Default:     30.0,
		Description: `SchedulerNamespaceStartWorkflowRPS is the per-namespace limit for starting workflows by schedules`,
	}
	WorkerDeleteNamespaceActivityLimitsConfig = &MapNamespaceSetting{
		Key:     "worker.deleteNamespaceActivityLimitsConfig",
		Default: map[string]any{},
		Description: `WorkerDeleteNamespaceActivityLimitsConfig is a map that contains a copy of relevant sdkworker.Options
settings for controlling remote activity concurrency for delete namespace workflows.`,
	}
)

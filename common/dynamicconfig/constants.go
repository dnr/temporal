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

	enumspb "go.temporal.io/api/enums/v1"

	"go.temporal.io/server/common/primitives"
)

var (
	// keys for admin

	AdminEnableListHistoryTasks = NewBoolGlobalSetting(
		"admin.enableListHistoryTasks",
		true,
		`AdminEnableListHistoryTasks is the key for enabling listing history tasks`,
	)
	AdminMatchingNamespaceToPartitionDispatchRate = NewFloatNamespaceSetting(
		"admin.matchingNamespaceToPartitionDispatchRate",
		10000,
		`AdminMatchingNamespaceToPartitionDispatchRate is the max qps of any task queue partition for a given namespace`,
	)
	AdminMatchingNamespaceTaskqueueToPartitionDispatchRate = NewFloatTaskQueueSetting(
		"admin.matchingNamespaceTaskqueueToPartitionDispatchRate",
		1000,
		`AdminMatchingNamespaceTaskqueueToPartitionDispatchRate is the max qps of a task queue partition for a given namespace & task queue`,
	)

	// keys for system

	VisibilityPersistenceMaxReadQPS = NewIntGlobalSetting(
		"system.visibilityPersistenceMaxReadQPS",
		9000,
		`VisibilityPersistenceMaxReadQPS is the max QPC system host can query visibility DB for read.`,
	)
	VisibilityPersistenceMaxWriteQPS = NewIntGlobalSetting(
		"system.visibilityPersistenceMaxWriteQPS",
		9000,
		`VisibilityPersistenceMaxWriteQPS is the max QPC system host can query visibility DB for write.`,
	)
	EnableReadFromSecondaryVisibility = NewBoolNamespaceSetting(
		"system.enableReadFromSecondaryVisibility",
		false,
		`EnableReadFromSecondaryVisibility is the config to enable read from secondary visibility`,
	)
	SecondaryVisibilityWritingMode = NewStringGlobalSetting(
		"system.secondaryVisibilityWritingMode",
		"off",
		`SecondaryVisibilityWritingMode is key for how to write to secondary visibility`,
	)
	VisibilityDisableOrderByClause = NewBoolNamespaceSetting(
		"system.visibilityDisableOrderByClause",
		true,
		`VisibilityDisableOrderByClause is the config to disable ORDERY BY clause for Elasticsearch`,
	)
	VisibilityEnableManualPagination = NewBoolNamespaceSetting(
		"system.visibilityEnableManualPagination",
		true,
		`VisibilityEnableManualPagination is the config to enable manual pagination for Elasticsearch`,
	)
	VisibilityAllowList = NewBoolNamespaceSetting(
		"system.visibilityAllowList",
		true,
		`VisibilityAllowList is the config to allow list of values for regular types`,
	)
	SuppressErrorSetSystemSearchAttribute = NewBoolNamespaceSetting(
		"system.suppressErrorSetSystemSearchAttribute",
		false,
		`SuppressErrorSetSystemSearchAttribute suppresses errors when trying to set
values in system search attributes.`,
	)

	HistoryArchivalState = NewStringGlobalSetting(
		"system.historyArchivalState",
		"", // actual default is from static config
		`HistoryArchivalState is key for the state of history archival`,
	)
	EnableReadFromHistoryArchival = NewBoolGlobalSetting(
		"system.enableReadFromHistoryArchival",
		false, // actual default is from static config
		`EnableReadFromHistoryArchival is key for enabling reading history from archival store`,
	)
	VisibilityArchivalState = NewStringGlobalSetting(
		"system.visibilityArchivalState",
		"", // actual default is from static config
		`VisibilityArchivalState is key for the state of visibility archival`,
	)
	EnableReadFromVisibilityArchival = NewBoolGlobalSetting(
		"system.enableReadFromVisibilityArchival",
		false, // actual default is from static config
		`EnableReadFromVisibilityArchival is key for enabling reading visibility from archival store`,
	)
	EnableNamespaceNotActiveAutoForwarding = NewBoolNamespaceSetting(
		"system.enableNamespaceNotActiveAutoForwarding",
		true,
		`EnableNamespaceNotActiveAutoForwarding whether enabling DC auto forwarding to active cluster
for signal / start / signal with start API if namespace is not active`,
	)
	TransactionSizeLimit = NewIntGlobalSetting(
		"system.transactionSizeLimit",
		primitives.DefaultTransactionSizeLimit,
		`TransactionSizeLimit is the largest allowed transaction size to persistence`,
	)
	DisallowQuery = NewBoolNamespaceSetting(
		"system.disallowQuery",
		false,
		`DisallowQuery is the key to disallow query for a namespace`,
	)
	EnableCrossNamespaceCommands = NewBoolGlobalSetting(
		"system.enableCrossNamespaceCommands",
		true,
		`EnableCrossNamespaceCommands is the key to enable commands for external namespaces`,
	)
	ClusterMetadataRefreshInterval = NewDurationGlobalSetting(
		"system.clusterMetadataRefreshInterval",
		time.Minute,
		`ClusterMetadataRefreshInterval is config to manage cluster metadata table refresh interval`,
	)
	ForceSearchAttributesCacheRefreshOnRead = NewBoolGlobalSetting(
		"system.forceSearchAttributesCacheRefreshOnRead",
		false,
		`ForceSearchAttributesCacheRefreshOnRead forces refreshing search attributes cache on a read operation, so we always
get the latest data from DB. This effectively bypasses cache value and is used to facilitate testing of changes in
search attributes. This should not be turned on in production.`,
	)
	EnableRingpopTLS = NewBoolGlobalSetting(
		"system.enableRingpopTLS",
		false,
		`EnableRingpopTLS controls whether to use TLS for ringpop, using the same "internode" TLS
config as the other services.`,
	)
	RingpopApproximateMaxPropagationTime = NewDurationGlobalSetting(
		"system.ringpopApproximateMaxPropagationTime",
		3*time.Second,
		`RingpopApproximateMaxPropagationTime is used for timing certain startup and shutdown processes.
(It is not and doesn't have to be a guarantee.)`,
	)
	EnableParentClosePolicyWorker = NewBoolGlobalSetting(
		"system.enableParentClosePolicyWorker",
		true,
		`EnableParentClosePolicyWorker decides whether or not enable system workers for processing parent close policy task`,
	)
	EnableStickyQuery = NewBoolNamespaceSetting(
		"system.enableStickyQuery",
		true,
		`EnableStickyQuery indicates if sticky query should be enabled per namespace`,
	)
	EnableActivityEagerExecution = NewBoolNamespaceSetting(
		"system.enableActivityEagerExecution",
		false,
		`EnableActivityEagerExecution indicates if activity eager execution is enabled per namespace`,
	)
	EnableEagerWorkflowStart = NewBoolNamespaceSetting(
		"system.enableEagerWorkflowStart",
		false,
		`EnableEagerWorkflowStart toggles "eager workflow start" - returning the first workflow task inline in the
response to a StartWorkflowExecution request and skipping the trip through matching.`,
	)
	NamespaceCacheRefreshInterval = NewDurationGlobalSetting(
		"system.namespaceCacheRefreshInterval",
		10*time.Second,
		`NamespaceCacheRefreshInterval is the key for namespace cache refresh interval dynamic config`,
	)
	PersistenceHealthSignalMetricsEnabled = NewBoolGlobalSetting(
		"system.persistenceHealthSignalMetricsEnabled",
		true,
		`PersistenceHealthSignalMetricsEnabled determines whether persistence shard RPS metrics are emitted`,
	)
	PersistenceHealthSignalAggregationEnabled = NewBoolGlobalSetting(
		"system.persistenceHealthSignalAggregationEnabled",
		true,
		`PersistenceHealthSignalAggregationEnabled determines whether persistence latency and error averages are tracked`,
	)
	PersistenceHealthSignalWindowSize = NewDurationGlobalSetting(
		"system.persistenceHealthSignalWindowSize",
		10*time.Second,
		`PersistenceHealthSignalWindowSize is the time window size in seconds for aggregating persistence signals`,
	)
	PersistenceHealthSignalBufferSize = NewIntGlobalSetting(
		"system.persistenceHealthSignalBufferSize",
		5000,
		`PersistenceHealthSignalBufferSize is the maximum number of persistence signals to buffer in memory per signal key`,
	)
	ShardRPSWarnLimit = NewIntGlobalSetting(
		"system.shardRPSWarnLimit",
		50,
		`ShardRPSWarnLimit is the per-shard RPS limit for warning`,
	)
	ShardPerNsRPSWarnPercent = NewFloatGlobalSetting(
		"system.shardPerNsRPSWarnPercent",
		0.8,
		`ShardPerNsRPSWarnPercent is the per-shard per-namespace RPS limit for warning as a percentage of ShardRPSWarnLimit
these warning are not emitted if the value is set to 0 or less`,
	)
	OperatorRPSRatio = NewFloatGlobalSetting(
		"system.operatorRPSRatio",
		0.2,
		`OperatorRPSRatio is the percentage of the rate limit provided to priority rate limiters that should be used for
operator API calls (highest priority). Should be >0.0 and <= 1.0 (defaults to 20% if not specified)`,
	)
	PersistenceQPSBurstRatio = NewFloatGlobalSetting(
		"system.persistenceQPSBurstRatio",
		1.0,
		`PersistenceQPSBurstRatio is the burst ratio for persistence QPS. This flag controls the burst ratio for all services.`,
	)

	// deadlock detector

	DeadlockDumpGoroutines = NewBoolGlobalSetting(
		"system.deadlock.DumpGoroutines",
		true,
		`Whether the deadlock detector should dump goroutines`,
	)
	DeadlockFailHealthCheck = NewBoolGlobalSetting(
		"system.deadlock.FailHealthCheck",
		false,
		`Whether the deadlock detector should cause the grpc server to fail health checks`,
	)
	DeadlockAbortProcess = NewBoolGlobalSetting(
		"system.deadlock.AbortProcess",
		false,
		`Whether the deadlock detector should abort the process`,
	)
	DeadlockInterval = NewDurationGlobalSetting(
		"system.deadlock.Interval",
		30*time.Second,
		`How often the detector checks each root.`,
	)
	DeadlockMaxWorkersPerRoot = NewIntGlobalSetting(
		"system.deadlock.MaxWorkersPerRoot",
		10,
		`How many extra goroutines can be created per root.`,
	)

	// utf-8 validation

	ValidateUTF8SampleRPCRequest = NewFloatGlobalSetting(
		"system.validateUTF8.sample.rpcRequest",
		0.0,
		`Sample rate of utf-8 string validation for rpc requests`,
	)
	ValidateUTF8SampleRPCResponse = NewFloatGlobalSetting(
		"system.validateUTF8.sample.rpcResponse",
		0.0,
		`Sample rate of utf-8 string validation for rpc responses`,
	)
	ValidateUTF8SamplePersistence = NewFloatGlobalSetting(
		"system.validateUTF8.sample.persistence",
		0.0,
		`Sample rate of utf-8 string validation for persistence [de]serialization`,
	)
	ValidateUTF8FailRPCRequest = NewBoolGlobalSetting(
		"system.validateUTF8.fail.rpcRequest",
		false,
		`Whether to fail rpcs on utf-8 string validation errors`,
	)
	ValidateUTF8FailRPCResponse = NewBoolGlobalSetting(
		"system.validateUTF8.fail.rpcResponse",
		false,
		`Whether to fail rpcs on utf-8 string validation errors`,
	)
	ValidateUTF8FailPersistence = NewBoolGlobalSetting(
		"system.validateUTF8.fail.persistence",
		false,
		`Whether to fail persistence [de]serialization on utf-8 string validation errors`,
	)

	// keys for size limit

	BlobSizeLimitError = NewIntNamespaceSetting(
		"limit.blobSize.error",
		2*1024*1024,
		`BlobSizeLimitError is the per event blob size limit`,
	)
	BlobSizeLimitWarn = NewIntNamespaceSetting(
		"limit.blobSize.warn",
		512*1024,
		`BlobSizeLimitWarn is the per event blob size limit for warning`,
	)
	MemoSizeLimitError = NewIntNamespaceSetting(
		"limit.memoSize.error",
		2*1024*1024,
		`MemoSizeLimitError is the per event memo size limit`,
	)
	MemoSizeLimitWarn = NewIntNamespaceSetting(
		"limit.memoSize.warn",
		2*1024,
		`MemoSizeLimitWarn is the per event memo size limit for warning`,
	)
	NumPendingChildExecutionsLimitError = NewIntNamespaceSetting(
		"limit.numPendingChildExecutions.error",
		2000,
		`NumPendingChildExecutionsLimitError is the maximum number of pending child workflows a workflow can have before
StartChildWorkflowExecution commands will fail.`,
	)
	NumPendingActivitiesLimitError = NewIntNamespaceSetting(
		"limit.numPendingActivities.error",
		2000,
		`NumPendingActivitiesLimitError is the maximum number of pending activities a workflow can have before
ScheduleActivityTask will fail.`,
	)
	NumPendingSignalsLimitError = NewIntNamespaceSetting(
		"limit.numPendingSignals.error",
		2000,
		`NumPendingSignalsLimitError is the maximum number of pending signals a workflow can have before
SignalExternalWorkflowExecution commands from this workflow will fail.`,
	)
	NumPendingCancelRequestsLimitError = NewIntNamespaceSetting(
		"limit.numPendingCancelRequests.error",
		2000,
		`NumPendingCancelRequestsLimitError is the maximum number of pending requests to cancel other workflows a workflow can have before
RequestCancelExternalWorkflowExecution commands will fail.`,
	)
	HistorySizeLimitError = NewIntNamespaceSetting(
		"limit.historySize.error",
		50*1024*1024,
		`HistorySizeLimitError is the per workflow execution history size limit`,
	)
	HistorySizeLimitWarn = NewIntNamespaceSetting(
		"limit.historySize.warn",
		10*1024*1024,
		`HistorySizeLimitWarn is the per workflow execution history size limit for warning`,
	)
	HistorySizeSuggestContinueAsNew = NewIntNamespaceSetting(
		"limit.historySize.suggestContinueAsNew",
		4*1024*1024,
		`HistorySizeSuggestContinueAsNew is the workflow execution history size limit to suggest
continue-as-new (in workflow task started event)`,
	)
	HistoryCountLimitError = NewIntNamespaceSetting(
		"limit.historyCount.error",
		50*1024,
		`HistoryCountLimitError is the per workflow execution history event count limit`,
	)
	HistoryCountLimitWarn = NewIntNamespaceSetting(
		"limit.historyCount.warn",
		10*1024,
		`HistoryCountLimitWarn is the per workflow execution history event count limit for warning`,
	)
	MutableStateActivityFailureSizeLimitError = NewIntNamespaceSetting(
		"limit.mutableStateActivityFailureSize.error",
		4*1024,
		`MutableStateActivityFailureSizeLimitError is the per activity failure size limit for workflow mutable state.
If exceeded, failure will be truncated before being stored in mutable state.`,
	)
	MutableStateActivityFailureSizeLimitWarn = NewIntNamespaceSetting(
		"limit.mutableStateActivityFailureSize.warn",
		2*1024,
		`MutableStateActivityFailureSizeLimitWarn is the per activity failure size warning limit for workflow mutable state`,
	)
	MutableStateSizeLimitError = NewIntGlobalSetting(
		"limit.mutableStateSize.error",
		8*1024*1024,
		`MutableStateSizeLimitError is the per workflow execution mutable state size limit in bytes`,
	)
	MutableStateSizeLimitWarn = NewIntGlobalSetting(
		"limit.mutableStateSize.warn",
		1*1024*1024,
		`MutableStateSizeLimitWarn is the per workflow execution mutable state size limit in bytes for warning`,
	)
	HistoryCountSuggestContinueAsNew = NewIntNamespaceSetting(
		"limit.historyCount.suggestContinueAsNew",
		4*1024,
		`HistoryCountSuggestContinueAsNew is the workflow execution history event count limit to
suggest continue-as-new (in workflow task started event)`,
	)
	HistoryMaxPageSize = NewIntNamespaceSetting(
		"limit.historyMaxPageSize",
		primitives.GetHistoryMaxPageSize,
		`HistoryMaxPageSize is default max size for GetWorkflowExecutionHistory in one page`,
	)
	MaxIDLengthLimit = NewIntGlobalSetting(
		"limit.maxIDLength",
		1000,
		`MaxIDLengthLimit is the length limit for various IDs, including: Namespace, TaskQueue, WorkflowID, ActivityID, TimerID,
WorkflowType, ActivityType, SignalName, MarkerName, ErrorReason/FailureReason/CancelCause, Identity, RequestID`,
	)
	WorkerBuildIdSizeLimit = NewIntGlobalSetting(
		"limit.workerBuildIdSize",
		255,
		`WorkerBuildIdSizeLimit is the byte length limit for a worker build id as used in the rpc methods for updating
the version sets for a task queue.
Do not set this to a value higher than 255 for clusters using SQL based persistence due to predefined VARCHAR
column width.`,
	)
	VersionCompatibleSetLimitPerQueue = NewIntNamespaceSetting(
		"limit.versionCompatibleSetLimitPerQueue",
		10,
		`VersionCompatibleSetLimitPerQueue is the max number of compatible sets allowed in the versioning data for a task
queue. Update requests which would cause the versioning data to exceed this number will fail with a
FailedPrecondition error.`,
	)
	VersionBuildIdLimitPerQueue = NewIntNamespaceSetting(
		"limit.versionBuildIdLimitPerQueue",
		100,
		`VersionBuildIdLimitPerQueue is the max number of build IDs allowed to be defined in the versioning data for a
task queue. Update requests which would cause the versioning data to exceed this number will fail with a
FailedPrecondition error.`,
	)
	AssignmentRuleLimitPerQueue = NewIntNamespaceSetting(
		"limit.wv.AssignmentRuleLimitPerQueue",
		100,
		`AssignmentRuleLimitPerQueue is the max number of Build ID assignment rules allowed to be defined in the
versioning data for a task queue. Update requests which would cause the versioning data to exceed this number
will fail with a FailedPrecondition error.`,
	)
	RedirectRuleLimitPerQueue = NewIntNamespaceSetting(
		"limit.wv.RedirectRuleLimitPerQueue",
		500,
		`RedirectRuleLimitPerQueue is the max number of compatible redirect rules allowed to be defined
in the versioning data for a task queue. Update requests which would cause the versioning data to exceed this
number will fail with a FailedPrecondition error.`,
	)
	RedirectRuleChainLimitPerQueue = NewIntNamespaceSetting(
		"limit.wv.RedirectRuleChainLimitPerQueue",
		50,
		`RedirectRuleChainLimitPerQueue is the max number of compatible redirect rules allowed to be connected
in one chain in the versioning data for a task queue. Update requests which would cause the versioning data
to exceed this number will fail with a FailedPrecondition error.`,
	)
	MatchingDeletedRuleRetentionTime = NewDurationNamespaceSetting(
		"matching.wv.DeletedRuleRetentionTime",
		14*24*time.Hour,
		`MatchingDeletedRuleRetentionTime is the length of time that deleted Version Assignment Rules and
Deleted Redirect Rules will be kept in the DB (with DeleteTimestamp). After this time, the tombstones are deleted at the next time update of versioning data for the task queue.`,
	)
	ReachabilityBuildIdVisibilityGracePeriod = NewDurationNamespaceSetting(
		"matching.wv.ReachabilityBuildIdVisibilityGracePeriod",
		3*time.Minute,
		`ReachabilityBuildIdVisibilityGracePeriod is the time period for which deleted versioning rules are still considered active
to account for the delay in updating the build id field in visibility.`,
	)
	ReachabilityTaskQueueScanLimit = NewIntGlobalSetting(
		"limit.reachabilityTaskQueueScan",
		20,
		`ReachabilityTaskQueueScanLimit limits the number of task queues to scan when responding to a
GetWorkerTaskReachability query.`,
	)
	ReachabilityQueryBuildIdLimit = NewIntGlobalSetting(
		"limit.reachabilityQueryBuildIds",
		5,
		`ReachabilityQueryBuildIdLimit limits the number of build ids that can be requested in a single call to the
DescribeTaskQueue API with ReportTaskQueueReachability==true, or to the GetWorkerTaskReachability API.`,
	)
	ReachabilityQuerySetDurationSinceDefault = NewDurationGlobalSetting(
		"frontend.reachabilityQuerySetDurationSinceDefault",
		5*time.Minute,
		`ReachabilityQuerySetDurationSinceDefault is the minimum period since a version set was demoted from being the
queue default before it is considered unreachable by new workflows.
This setting allows some propagation delay of versioning data for the reachability queries, which may happen for
the following reasons:
1. There are no workflows currently marked as open in the visibility store but a worker for the demoted version
is currently processing a task.
2. There are delays in the visibility task processor (which is asynchronous).
3. There's propagation delay of the versioning data between matching nodes.`,
	)
	TaskQueuesPerBuildIdLimit = NewIntNamespaceSetting(
		"limit.taskQueuesPerBuildId",
		20,
		`TaskQueuesPerBuildIdLimit limits the number of task queue names that can be mapped to a single build id.`,
	)

	NexusIncomingServiceNameMaxLength = NewIntGlobalSetting(
		"limit.incomingServiceNameMaxLength",
		200,
		`NexusIncomingServiceNameMaxLength is the maximum length of a Nexus incoming service name.`,
	)
	NexusIncomingServiceMaxSize = NewIntGlobalSetting(
		"limit.incomingServiceMaxSize",
		4*1024,
		`NexusIncomingServiceMaxSize is the maximum size of a Nexus incoming service in bytes.`,
	)
	NexusIncomingServiceListDefaultPageSize = NewIntGlobalSetting(
		"limit.incomingServiceListDefaultPageSize",
		100,
		`NexusIncomingServiceListDefaultPageSize is the default page size for listing Nexus incoming services.`,
	)
	NexusIncomingServiceListMaxPageSize = NewIntGlobalSetting(
		"limit.incomingServiceListMaxPageSize",
		1000,
		`NexusIncomingServiceListMaxPageSize is the maximum page size for listing Nexus incoming services.`,
	)
	NexusOutgoingServiceURLMaxLength = NewIntGlobalSetting(
		"limit.outgoingServiceURLMaxLength",
		1000,
		`NexusOutgoingServiceURLMaxLength is the maximum length of an outgoing service URL.`,
	)
	NexusOutgoingServiceNameMaxLength = NewIntGlobalSetting(
		"limit.outgoingServiceNameMaxLength",
		200,
		`NexusOutgoingServiceNameMaxLength is the maximum length of an outgoing service name.`,
	)
	NexusOutgoingServiceListDefaultPageSize = NewIntGlobalSetting(
		"limit.outgoingServiceListDefaultPageSize",
		100,
		`NexusOutgoingServiceListDefaultPageSize is the default page size for listing outgoing services.`,
	)
	NexusOutgoingServiceListMaxPageSize = NewIntGlobalSetting(
		"limit.outgoingServiceListMaxPageSize",
		1000,
		`NexusOutgoingServiceListMaxPageSize is the maximum page size for listing outgoing services.`,
	)

	RemovableBuildIdDurationSinceDefault = NewDurationGlobalSetting(
		"worker.removableBuildIdDurationSinceDefault",
		time.Hour,
		`RemovableBuildIdDurationSinceDefault is the minimum duration since a build id was last default in its containing
set for it to be considered for removal, used by the build id scavenger.
This setting allows some propagation delay of versioning data, which may happen for the following reasons:
1. There are no workflows currently marked as open in the visibility store but a worker for the demoted version
is currently processing a task.
2. There are delays in the visibility task processor (which is asynchronous).
3. There's propagation delay of the versioning data between matching nodes.`,
	)
	BuildIdScavengerVisibilityRPS = NewFloatGlobalSetting(
		"worker.buildIdScavengerVisibilityRPS",
		1.0,
		`BuildIdScavengerVisibilityRPS is the rate limit for visibility calls from the build id scavenger`,
	)

	// keys for frontend

	FrontendPersistenceMaxQPS = NewIntGlobalSetting(
		"frontend.persistenceMaxQPS",
		2000,
		`FrontendPersistenceMaxQPS is the max qps frontend host can query DB`,
	)
	FrontendPersistenceGlobalMaxQPS = NewIntGlobalSetting(
		"frontend.persistenceGlobalMaxQPS",
		0,
		`FrontendPersistenceGlobalMaxQPS is the max qps frontend cluster can query DB`,
	)
	FrontendPersistenceNamespaceMaxQPS = NewIntNamespaceSetting(
		"frontend.persistenceNamespaceMaxQPS",
		0,
		`FrontendPersistenceNamespaceMaxQPS is the max qps each namespace on frontend host can query DB`,
	)
	FrontendPersistenceGlobalNamespaceMaxQPS = NewIntNamespaceSetting(
		"frontend.persistenceGlobalNamespaceMaxQPS",
		0,
		`FrontendPersistenceGlobalNamespaceMaxQPS is the max qps each namespace in frontend cluster can query DB`,
	)
	FrontendEnablePersistencePriorityRateLimiting = NewBoolGlobalSetting(
		"frontend.enablePersistencePriorityRateLimiting",
		true,
		`FrontendEnablePersistencePriorityRateLimiting indicates if priority rate limiting is enabled in frontend persistence client`,
	)
	FrontendPersistenceDynamicRateLimitingParams = NewMapGlobalSetting(
		"frontend.persistenceDynamicRateLimitingParams",
		DefaultDynamicRateLimitingParams,
		`FrontendPersistenceDynamicRateLimitingParams is a map that contains all adjustable dynamic rate limiting params
see DefaultDynamicRateLimitingParams for available options and defaults`,
	)
	FrontendVisibilityMaxPageSize = NewIntNamespaceSetting(
		"frontend.visibilityMaxPageSize",
		1000,
		`FrontendVisibilityMaxPageSize is default max size for ListWorkflowExecutions in one page`,
	)
	FrontendHistoryMaxPageSize = NewIntNamespaceSetting(
		"frontend.historyMaxPageSize",
		primitives.GetHistoryMaxPageSize,
		`FrontendHistoryMaxPageSize is default max size for GetWorkflowExecutionHistory in one page`,
	)
	FrontendRPS = NewIntGlobalSetting(
		"frontend.rps",
		2400,
		`FrontendRPS is workflow rate limit per second per-instance`,
	)
	FrontendGlobalRPS = NewIntGlobalSetting(
		"frontend.globalRPS",
		0,
		`FrontendGlobalRPS is workflow rate limit per second for the whole cluster`,
	)
	FrontendNamespaceReplicationInducingAPIsRPS = NewIntGlobalSetting(
		"frontend.rps.namespaceReplicationInducingAPIs",
		20,
		`FrontendNamespaceReplicationInducingAPIsRPS limits the per second request rate for namespace replication inducing
APIs (e.g. RegisterNamespace, UpdateNamespace, UpdateWorkerBuildIdCompatibility).
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	)
	FrontendMaxNamespaceRPSPerInstance = NewIntNamespaceSetting(
		"frontend.namespaceRPS",
		2400,
		`FrontendMaxNamespaceRPSPerInstance is workflow namespace rate limit per second`,
	)
	FrontendMaxNamespaceBurstRatioPerInstance = NewFloatNamespaceSetting(
		"frontend.namespaceBurstRatio",
		2,
		`FrontendMaxNamespaceBurstRatioPerInstance is workflow namespace burst limit as a ratio of namespace RPS. The RPS
used here will be the effective RPS from global and per-instance limits. The value must be 1 or higher.`,
	)
	FrontendMaxConcurrentLongRunningRequestsPerInstance = NewIntNamespaceSetting(
		"frontend.namespaceCount",
		1200,
		`FrontendMaxConcurrentLongRunningRequestsPerInstance limits concurrent long-running requests per-instance,
per-API. Example requests include long-poll requests, and 'Query' requests (which need to wait for WFTs). The
limit is applied individually to each API method. This value is ignored if
FrontendGlobalMaxConcurrentLongRunningRequests is greater than zero. Warning: setting this to zero will cause all
long-running requests to fail. The name 'frontend.namespaceCount' is kept for backwards compatibility with
existing deployments even though it is a bit of a misnomer. This does not limit the number of namespaces; it is a
per-_namespace_ limit on the _count_ of long-running requests. Requests are only throttled when the limit is
exceeded, not when it is only reached.`,
	)
	FrontendGlobalMaxConcurrentLongRunningRequests = NewIntNamespaceSetting(
		"frontend.globalNamespaceCount",
		0,
		`FrontendGlobalMaxConcurrentLongRunningRequests limits concurrent long-running requests across all frontend
instances in the cluster, for a given namespace, per-API method. If this is set to 0 (the default), then it is
ignored. The name 'frontend.globalNamespaceCount' is kept for consistency with the per-instance limit name,
'frontend.namespaceCount'.`,
	)
	FrontendMaxNamespaceVisibilityRPSPerInstance = NewIntNamespaceSetting(
		"frontend.namespaceRPS.visibility",
		10,
		`FrontendMaxNamespaceVisibilityRPSPerInstance is namespace rate limit per second for visibility APIs.
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	)
	FrontendMaxNamespaceNamespaceReplicationInducingAPIsRPSPerInstance = NewIntNamespaceSetting(
		"frontend.namespaceRPS.namespaceReplicationInducingAPIs",
		1,
		`FrontendMaxNamespaceNamespaceReplicationInducingAPIsRPSPerInstance is a per host/per namespace RPS limit for
namespace replication inducing APIs (e.g. RegisterNamespace, UpdateNamespace, UpdateWorkerBuildIdCompatibility).
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	)
	FrontendMaxNamespaceVisibilityBurstRatioPerInstance = NewFloatNamespaceSetting(
		"frontend.namespaceBurstRatio.visibility",
		1,
		`FrontendMaxNamespaceVisibilityBurstRatioPerInstance is namespace burst limit for visibility APIs as a ratio of
namespace visibility RPS. The RPS used here will be the effective RPS from global and per-instance limits. This
config is EXPERIMENTAL and may be changed or removed in a later release. The value must be 1 or higher.`,
	)
	FrontendMaxNamespaceNamespaceReplicationInducingAPIsBurstRatioPerInstance = NewFloatNamespaceSetting(
		"frontend.namespaceBurstRatio.namespaceReplicationInducingAPIs",
		10,
		`FrontendMaxNamespaceNamespaceReplicationInducingAPIsBurstRatioPerInstance is a per host/per namespace burst limit for
namespace replication inducing APIs (e.g. RegisterNamespace, UpdateNamespace, UpdateWorkerBuildIdCompatibility)
as a ratio of namespace ReplicationInducingAPIs RPS. The RPS used here will be the effective RPS from global and
per-instance limits. This config is EXPERIMENTAL and may be changed or removed in a later release. The value must
be 1 or higher.`,
	)
	FrontendGlobalNamespaceRPS = NewIntNamespaceSetting(
		"frontend.globalNamespaceRPS",
		0,
		`FrontendGlobalNamespaceRPS is workflow namespace rate limit per second for the whole cluster.
The limit is evenly distributed among available frontend service instances.
If this is set, it overwrites per instance limit "frontend.namespaceRPS".`,
	)
	InternalFrontendGlobalNamespaceRPS = NewIntNamespaceSetting(
		"internal-frontend.globalNamespaceRPS",
		0,
		`InternalFrontendGlobalNamespaceRPS is workflow namespace rate limit per second across
all internal-frontends.`,
	)
	FrontendGlobalNamespaceVisibilityRPS = NewIntNamespaceSetting(
		"frontend.globalNamespaceRPS.visibility",
		0,
		`FrontendGlobalNamespaceVisibilityRPS is workflow namespace rate limit per second for the whole cluster for visibility API.
The limit is evenly distributed among available frontend service instances.
If this is set, it overwrites per instance limit "frontend.namespaceRPS.visibility".
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	)
	FrontendGlobalNamespaceNamespaceReplicationInducingAPIsRPS = NewIntNamespaceSetting(
		"frontend.globalNamespaceRPS.namespaceReplicationInducingAPIs",
		10,
		`FrontendGlobalNamespaceNamespaceReplicationInducingAPIsRPS is a cluster global, per namespace RPS limit for
namespace replication inducing APIs (e.g. RegisterNamespace, UpdateNamespace, UpdateWorkerBuildIdCompatibility).
The limit is evenly distributed among available frontend service instances.
If this is set, it overwrites the per instance limit configured with
"frontend.namespaceRPS.namespaceReplicationInducingAPIs".
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	)
	InternalFrontendGlobalNamespaceVisibilityRPS = NewIntNamespaceSetting(
		"internal-frontend.globalNamespaceRPS.visibility",
		0,
		`InternalFrontendGlobalNamespaceVisibilityRPS is workflow namespace rate limit per second
across all internal-frontends.
This config is EXPERIMENTAL and may be changed or removed in a later release.`,
	)
	FrontendThrottledLogRPS = NewIntGlobalSetting(
		"frontend.throttledLogRPS",
		20,
		`FrontendThrottledLogRPS is the rate limit on number of log messages emitted per second for throttled logger`,
	)
	FrontendShutdownDrainDuration = NewDurationGlobalSetting(
		"frontend.shutdownDrainDuration",
		0*time.Second,
		`FrontendShutdownDrainDuration is the duration of traffic drain during shutdown`,
	)
	FrontendShutdownFailHealthCheckDuration = NewDurationGlobalSetting(
		"frontend.shutdownFailHealthCheckDuration",
		0*time.Second,
		`FrontendShutdownFailHealthCheckDuration is the duration of shutdown failure detection`,
	)
	FrontendMaxBadBinaries = NewIntNamespaceSetting(
		"frontend.maxBadBinaries",
		10,
		`FrontendMaxBadBinaries is the max number of bad binaries in namespace config`,
	)
	SendRawWorkflowHistory = NewBoolNamespaceSetting(
		"frontend.sendRawWorkflowHistory",
		false,
		`SendRawWorkflowHistory is whether to enable raw history retrieving`,
	)
	SearchAttributesNumberOfKeysLimit = NewIntNamespaceSetting(
		"frontend.searchAttributesNumberOfKeysLimit",
		100,
		`SearchAttributesNumberOfKeysLimit is the limit of number of keys`,
	)
	SearchAttributesSizeOfValueLimit = NewIntNamespaceSetting(
		"frontend.searchAttributesSizeOfValueLimit",
		2*1024,
		`SearchAttributesSizeOfValueLimit is the size limit of each value`,
	)
	SearchAttributesTotalSizeLimit = NewIntNamespaceSetting(
		"frontend.searchAttributesTotalSizeLimit",
		40*1024,
		`SearchAttributesTotalSizeLimit is the size limit of the whole map`,
	)
	VisibilityArchivalQueryMaxPageSize = NewIntGlobalSetting(
		"frontend.visibilityArchivalQueryMaxPageSize",
		10000,
		`VisibilityArchivalQueryMaxPageSize is the maximum page size for a visibility archival query`,
	)
	EnableServerVersionCheck = NewBoolGlobalSetting(
		"frontend.enableServerVersionCheck",
		os.Getenv("TEMPORAL_VERSION_CHECK_DISABLED") == "",
		`EnableServerVersionCheck is a flag that controls whether or not periodic version checking is enabled`,
	)
	EnableTokenNamespaceEnforcement = NewBoolGlobalSetting(
		"frontend.enableTokenNamespaceEnforcement",
		true,
		`EnableTokenNamespaceEnforcement enables enforcement that namespace in completion token matches namespace of the request`,
	)
	DisableListVisibilityByFilter = NewBoolNamespaceSetting(
		"frontend.disableListVisibilityByFilter",
		false,
		`DisableListVisibilityByFilter is config to disable list open/close workflow using filter`,
	)
	KeepAliveMinTime = NewDurationGlobalSetting(
		"frontend.keepAliveMinTime",
		10*time.Second,
		`KeepAliveMinTime is the minimum amount of time a client should wait before sending a keepalive ping.`,
	)
	KeepAlivePermitWithoutStream = NewBoolGlobalSetting(
		"frontend.keepAlivePermitWithoutStream",
		true,
		`KeepAlivePermitWithoutStream If true, server allows keepalive pings even when there are no active
streams(RPCs). If false, and client sends ping when there are no active
streams, server will send GOAWAY and close the connection.`,
	)
	KeepAliveMaxConnectionIdle = NewDurationGlobalSetting(
		"frontend.keepAliveMaxConnectionIdle",
		2*time.Minute,
		`KeepAliveMaxConnectionIdle is a duration for the amount of time after which an
idle connection would be closed by sending a GoAway. Idleness duration is
defined since the most recent time the number of outstanding RPCs became
zero or the connection establishment.`,
	)
	KeepAliveMaxConnectionAge = NewDurationGlobalSetting(
		"frontend.keepAliveMaxConnectionAge",
		5*time.Minute,
		`KeepAliveMaxConnectionAge is a duration for the maximum amount of time a
connection may exist before it will be closed by sending a GoAway. A
random jitter of +/-10% will be added to MaxConnectionAge to spread out
connection storms.`,
	)
	KeepAliveMaxConnectionAgeGrace = NewDurationGlobalSetting(
		"frontend.keepAliveMaxConnectionAgeGrace",
		70*time.Second,
		`KeepAliveMaxConnectionAgeGrace is an additive period after MaxConnectionAge after
which the connection will be forcibly closed.`,
	)
	KeepAliveTime = NewDurationGlobalSetting(
		"frontend.keepAliveTime",
		1*time.Minute,
		`KeepAliveTime After a duration of this time if the server doesn't see any activity it
pings the client to see if the transport is still alive.
If set below 1s, a minimum value of 1s will be used instead.`,
	)
	KeepAliveTimeout = NewDurationGlobalSetting(
		"frontend.keepAliveTimeout",
		10*time.Second,
		`KeepAliveTimeout After having pinged for keepalive check, the server waits for a duration
of Timeout and if no activity is seen even after that the connection is closed.`,
	)
	FrontendEnableSchedules = NewBoolNamespaceSetting(
		"frontend.enableSchedules",
		true,
		`FrontendEnableSchedules enables schedule-related RPCs in the frontend`,
	)
	FrontendEnableNexusAPIs = NewBoolGlobalSetting(
		"frontend.enableNexusAPIs",
		false,
		`FrontendEnableNexusAPIs enables serving Nexus HTTP requests in the frontend.`,
	)
	FrontendRefreshNexusIncomingServicesLongPollTimeout = NewDurationGlobalSetting(
		"frontend.refreshNexusIncomingServicesLongPollTimeout",
		5*time.Minute,
		`FrontendRefreshNexusIncomingServicesLongPollTimeout is the maximum duration of background long poll requests to update Nexus incoming services.`,
	)
	// // FrontendRefreshNexusIncomingServicesMinWait is the minimum wait time between background long poll requests to update Nexus incoming services.
	// FrontendRefreshNexusIncomingServicesMinWait = "frontend.refreshNexusIncomingServicesMinWait"
	FrontendEnableCallbackAttachment = NewBoolNamespaceSetting(
		"frontend.enableCallbackAttachment",
		false,
		`FrontendEnableCallbackAttachment enables attaching callbacks to workflows.`,
	)
	FrontendMaxConcurrentBatchOperationPerNamespace = NewIntNamespaceSetting(
		"frontend.MaxConcurrentBatchOperationPerNamespace",
		1,
		`FrontendMaxConcurrentBatchOperationPerNamespace is the max concurrent batch operation job count per namespace`,
	)
	FrontendMaxExecutionCountBatchOperationPerNamespace = NewIntNamespaceSetting(
		"frontend.MaxExecutionCountBatchOperationPerNamespace",
		1000,
		`FrontendMaxExecutionCountBatchOperationPerNamespace is the max execution count batch operation supports per namespace`,
	)
	FrontendEnableBatcher = NewBoolNamespaceSetting(
		"frontend.enableBatcher",
		true,
		`FrontendEnableBatcher enables batcher-related RPCs in the frontend`,
	)
	FrontendAccessHistoryFraction = NewFloatGlobalSetting(
		"frontend.accessHistoryFraction",
		0.0,
		`FrontendAccessHistoryFraction (0.0~1.0) is the fraction of history operations that are sent to the history
service using the new RPCs. The remaining access history via the existing implementation.
TODO: remove once migration completes.`,
	)
	FrontendAdminDeleteAccessHistoryFraction = NewFloatGlobalSetting(
		"frontend.adminDeleteAccessHistoryFraction",
		0.0,
		`FrontendAdminDeleteAccessHistoryFraction (0.0~1.0) is the fraction of admin DeleteWorkflowExecution requests
that are sent to the history service using the new RPCs. The remaining access history via the existing implementation.
TODO: remove once migration completes.`,
	)

	FrontendEnableUpdateWorkflowExecution = NewBoolNamespaceSetting(
		"frontend.enableUpdateWorkflowExecution",
		false,
		`FrontendEnableUpdateWorkflowExecution enables UpdateWorkflowExecution API in the frontend.
The UpdateWorkflowExecution API has gone through rigorous testing efforts but this config's default is 'false' until the
feature gets more time in production.`,
	)

	FrontendEnableExecuteMultiOperation = NewBoolNamespaceSetting(
		"frontend.enableExecuteMultiOperation",
		false,
		`FrontendEnableExecuteMultiOperation enables the ExecuteMultiOperation API in the frontend.
The API is under active development.`,
	)

	FrontendEnableUpdateWorkflowExecutionAsyncAccepted = NewBoolNamespaceSetting(
		"frontend.enableUpdateWorkflowExecutionAsyncAccepted",
		false,
		`FrontendEnableUpdateWorkflowExecutionAsyncAccepted enables the form of
asynchronous workflow execution update that waits on the "Accepted"
lifecycle stage. Default value is 'false'.`,
	)

	EnableWorkflowIdConflictPolicy = NewBoolNamespaceSetting(
		"frontend.enableWorkflowIdConflictPolicy",
		false,
		`EnableWorkflowIdConflictPolicy enables the 'WorkflowIdConflictPolicy' option for Start and Signal-with-Start`,
	)

	FrontendEnableWorkerVersioningDataAPIs = NewBoolNamespaceSetting(
		"frontend.workerVersioningDataAPIs",
		false,
		`FrontendEnableWorkerVersioningDataAPIs enables worker versioning data read / write APIs.`,
	)
	FrontendEnableWorkerVersioningWorkflowAPIs = NewBoolNamespaceSetting(
		"frontend.workerVersioningWorkflowAPIs",
		false,
		`FrontendEnableWorkerVersioningWorkflowAPIs enables worker versioning in workflow progress APIs.`,
	)
	FrontendEnableWorkerVersioningRuleAPIs = NewBoolNamespaceSetting(
		"frontend.workerVersioningRuleAPIs",
		false,
		`FrontendEnableWorkerVersioningRuleAPIs enables worker versioning in workflow progress APIs.`,
	)

	DeleteNamespaceDeleteActivityRPS = NewIntGlobalSetting(
		"frontend.deleteNamespaceDeleteActivityRPS",
		100,
		`DeleteNamespaceDeleteActivityRPS is an RPS per every parallel delete executions activity.
Total RPS is equal to DeleteNamespaceDeleteActivityRPS * DeleteNamespaceConcurrentDeleteExecutionsActivities.
Default value is 100.`,
	)
	DeleteNamespacePageSize = NewIntGlobalSetting(
		"frontend.deleteNamespaceDeletePageSize",
		1000,
		`DeleteNamespacePageSize is a page size to read executions from visibility for delete executions activity.
Default value is 1000.`,
	)
	DeleteNamespacePagesPerExecution = NewIntGlobalSetting(
		"frontend.deleteNamespacePagesPerExecution",
		256,
		`DeleteNamespacePagesPerExecution is a number of pages before returning ContinueAsNew from delete executions activity.
Default value is 256.`,
	)
	DeleteNamespaceConcurrentDeleteExecutionsActivities = NewIntGlobalSetting(
		"frontend.deleteNamespaceConcurrentDeleteExecutionsActivities",
		4,
		`DeleteNamespaceConcurrentDeleteExecutionsActivities is a number of concurrent delete executions activities.
Must be not greater than 256 and number of worker cores in the cluster.
Default is 4.`,
	)
	DeleteNamespaceNamespaceDeleteDelay = NewDurationGlobalSetting(
		"frontend.deleteNamespaceNamespaceDeleteDelay",
		0*time.Hour,
		`DeleteNamespaceNamespaceDeleteDelay is a duration for how long namespace stays in database
after all namespace resources (i.e. workflow executions) are deleted.
Default is 0, means, namespace will be deleted immediately.`,
	)

	// keys for matching

	MatchingRPS = NewIntGlobalSetting(
		"matching.rps",
		1200,
		`MatchingRPS is request rate per second for each matching host`,
	)
	MatchingPersistenceMaxQPS = NewIntGlobalSetting(
		"matching.persistenceMaxQPS",
		3000,
		`MatchingPersistenceMaxQPS is the max qps matching host can query DB`,
	)
	MatchingPersistenceGlobalMaxQPS = NewIntGlobalSetting(
		"matching.persistenceGlobalMaxQPS",
		0,
		`MatchingPersistenceGlobalMaxQPS is the max qps matching cluster can query DB`,
	)
	MatchingPersistenceNamespaceMaxQPS = NewIntNamespaceSetting(
		"matching.persistenceNamespaceMaxQPS",
		0,
		`MatchingPersistenceNamespaceMaxQPS is the max qps each namespace on matching host can query DB`,
	)
	MatchingPersistenceGlobalNamespaceMaxQPS = NewIntNamespaceSetting(
		"matching.persistenceGlobalNamespaceMaxQPS",
		0,
		`MatchingPersistenceNamespaceMaxQPS is the max qps each namespace in matching cluster can query DB`,
	)
	MatchingEnablePersistencePriorityRateLimiting = NewBoolGlobalSetting(
		"matching.enablePersistencePriorityRateLimiting",
		true,
		`MatchingEnablePersistencePriorityRateLimiting indicates if priority rate limiting is enabled in matching persistence client`,
	)
	MatchingPersistenceDynamicRateLimitingParams = NewMapGlobalSetting(
		"matching.persistenceDynamicRateLimitingParams",
		DefaultDynamicRateLimitingParams,
		`MatchingPersistenceDynamicRateLimitingParams is a map that contains all adjustable dynamic rate limiting params
see DefaultDynamicRateLimitingParams for available options and defaults`,
	)
	MatchingMinTaskThrottlingBurstSize = NewIntTaskQueueSetting(
		"matching.minTaskThrottlingBurstSize",
		1,
		`MatchingMinTaskThrottlingBurstSize is the minimum burst size for task queue throttling`,
	)
	MatchingGetTasksBatchSize = NewIntTaskQueueSetting(
		"matching.getTasksBatchSize",
		1000,
		`MatchingGetTasksBatchSize is the maximum batch size to fetch from the task buffer`,
	)
	MatchingLongPollExpirationInterval = NewDurationTaskQueueSetting(
		"matching.longPollExpirationInterval",
		time.Minute,
		`MatchingLongPollExpirationInterval is the long poll expiration interval in the matching service`,
	)
	MatchingSyncMatchWaitDuration = NewDurationTaskQueueSetting(
		"matching.syncMatchWaitDuration",
		200*time.Millisecond,
		`MatchingSyncMatchWaitDuration is to wait time for sync match`,
	)
	MatchingHistoryMaxPageSize = NewIntNamespaceSetting(
		"matching.historyMaxPageSize",
		primitives.GetHistoryMaxPageSize,
		`MatchingHistoryMaxPageSize is the maximum page size of history events returned on PollWorkflowTaskQueue requests`,
	)
	MatchingLoadUserData = NewBoolTaskQueueSetting(
		"matching.loadUserData",
		true,
		`MatchingLoadUserData can be used to entirely disable loading user data from persistence (and the inter node RPCs
that propoagate it). When turned off, features that rely on user data (e.g. worker versioning) will essentially
be disabled. When disabled, matching will drop tasks for versioned workflows and activities to avoid breaking
versioning semantics. Operator intervention will be required to reschedule the dropped tasks.`,
	)
	MatchingUpdateAckInterval = NewDurationTaskQueueSettingWithConstrainedDefault(
		"matching.updateAckInterval",
		[]TypedConstrainedValue[time.Duration]{
			// Use a longer default interval for the per-namespace internal worker queues.
			{
				Constraints: Constraints{
					TaskQueueName: primitives.PerNSWorkerTaskQueue,
				},
				Value: 5 * time.Minute,
			},
			// Default for everything else.
			{
				Value: 1 * time.Minute,
			},
		},
		`MatchingUpdateAckInterval is the interval for update ack`,
	)
	MatchingMaxTaskQueueIdleTime = NewDurationTaskQueueSetting(
		"matching.maxTaskQueueIdleTime",
		5*time.Minute,
		`MatchingMaxTaskQueueIdleTime is the time after which an idle task queue will be unloaded.
Note: this should be greater than matching.longPollExpirationInterval and matching.getUserDataLongPollTimeout.`,
	)
	MatchingOutstandingTaskAppendsThreshold = NewIntTaskQueueSetting(
		"matching.outstandingTaskAppendsThreshold",
		250,
		`MatchingOutstandingTaskAppendsThreshold is the threshold for outstanding task appends`,
	)
	MatchingMaxTaskBatchSize = NewIntTaskQueueSetting(
		"matching.maxTaskBatchSize",
		100,
		`MatchingMaxTaskBatchSize is max batch size for task writer`,
	)
	MatchingMaxTaskDeleteBatchSize = NewIntTaskQueueSetting(
		"matching.maxTaskDeleteBatchSize",
		100,
		`MatchingMaxTaskDeleteBatchSize is the max batch size for range deletion of tasks`,
	)
	MatchingThrottledLogRPS = NewIntGlobalSetting(
		"matching.throttledLogRPS",
		20,
		`MatchingThrottledLogRPS is the rate limit on number of log messages emitted per second for throttled logger`,
	)
	MatchingNumTaskqueueWritePartitions = NewIntTaskQueueSettingWithConstrainedDefault(
		"matching.numTaskqueueWritePartitions",
		defaultNumTaskQueuePartitions,
		`MatchingNumTaskqueueWritePartitions is the number of write partitions for a task queue`,
	)
	MatchingNumTaskqueueReadPartitions = NewIntTaskQueueSettingWithConstrainedDefault(
		"matching.numTaskqueueReadPartitions",
		defaultNumTaskQueuePartitions,
		`MatchingNumTaskqueueReadPartitions is the number of read partitions for a task queue`,
	)
	MatchingForwarderMaxOutstandingPolls = NewIntTaskQueueSetting(
		"matching.forwarderMaxOutstandingPolls",
		1,
		`MatchingForwarderMaxOutstandingPolls is the max number of inflight polls from the forwarder`,
	)
	MatchingForwarderMaxOutstandingTasks = NewIntTaskQueueSetting(
		"matching.forwarderMaxOutstandingTasks",
		1,
		`MatchingForwarderMaxOutstandingTasks is the max number of inflight addTask/queryTask from the forwarder`,
	)
	MatchingForwarderMaxRatePerSecond = NewIntTaskQueueSetting(
		"matching.forwarderMaxRatePerSecond",
		10,
		`MatchingForwarderMaxRatePerSecond is the max rate at which add/query can be forwarded`,
	)
	MatchingForwarderMaxChildrenPerNode = NewIntTaskQueueSetting(
		"matching.forwarderMaxChildrenPerNode",
		20,
		`MatchingForwarderMaxChildrenPerNode is the max number of children per node in the task queue partition tree`,
	)
	MatchingAlignMembershipChange = NewDurationGlobalSetting(
		"matching.alignMembershipChange",
		0*time.Second,
		`MatchingAlignMembershipChange is a duration to align matching's membership changes to.
This can help reduce effects of task queue movement.`,
	)
	MatchingShutdownDrainDuration = NewDurationGlobalSetting(
		"matching.shutdownDrainDuration",
		0*time.Second,
		`MatchingShutdownDrainDuration is the duration of traffic drain during shutdown`,
	)
	MatchingGetUserDataLongPollTimeout = NewDurationGlobalSetting(
		"matching.getUserDataLongPollTimeout",
		5*time.Minute-10*time.Second,
		`MatchingGetUserDataLongPollTimeout is the max length of long polls for GetUserData calls between partitions.`,
	)
	MatchingBacklogNegligibleAge = NewDurationTaskQueueSetting(
		"matching.backlogNegligibleAge",
		24*365*10*time.Hour,
		`MatchingBacklogNegligibleAge if the head of backlog gets older than this we stop sync match and
forwarding to ensure more equal dispatch order among partitions.`,
	)
	MatchingMaxWaitForPollerBeforeFwd = NewDurationTaskQueueSetting(
		"matching.maxWaitForPollerBeforeFwd",
		200*time.Millisecond,
		`MatchingMaxWaitForPollerBeforeFwd in presence of a non-negligible backlog, we resume forwarding tasks if the
duration since last poll exceeds this threshold.`,
	)
	QueryPollerUnavailableWindow = NewDurationGlobalSetting(
		"matching.queryPollerUnavailableWindow",
		20*time.Second,
		`QueryPollerUnavailableWindow WF Queries are rejected after a while if no poller has been seen within the window`,
	)
	MatchingListNexusIncomingServicesLongPollTimeout = NewDurationGlobalSetting(
		"matching.listNexusIncomingServicesLongPollTimeout",
		5*time.Minute-10*time.Second,
		`MatchingListNexusIncomingServicesLongPollTimeout is the max length of long polls for ListNexusIncomingServices calls.`,
	)
	MatchingMembershipUnloadDelay = NewDurationGlobalSetting(
		"matching.membershipUnloadDelay",
		500*time.Millisecond,
		`MatchingMembershipUnloadDelay is how long to wait to re-confirm loss of ownership before unloading a task queue.
Set to zero to disable proactive unload.`,
	)
	MatchingQueryWorkflowTaskTimeoutLogRate = NewFloatTaskQueueSetting(
		"matching.queryWorkflowTaskTimeoutLogRate",
		0.0,
		`MatchingQueryWorkflowTaskTimeoutLogRate defines the sampling rate for logs when a query workflow task times out. Since
these log lines can be noisy, we want to be able to turn on and sample selectively for each affected namespace.`,
	)

	// for matching testing only:

	TestMatchingDisableSyncMatch = NewBoolGlobalSetting(
		"test.matching.disableSyncMatch",
		false,
		`TestMatchingDisableSyncMatch forces tasks to go through the db once`,
	)
	TestMatchingLBForceReadPartition = NewIntGlobalSetting(
		"test.matching.lbForceReadPartition",
		-1,
		`TestMatchingLBForceReadPartition forces polls to go to a specific partition`,
	)
	TestMatchingLBForceWritePartition = NewIntGlobalSetting(
		"test.matching.lbForceWritePartition",
		-1,
		`TestMatchingLBForceWritePartition forces adds to go to a specific partition`,
	)

	// keys for history

	EnableReplicationStream = NewBoolGlobalSetting(
		"history.enableReplicationStream",
		false,
		`EnableReplicationStream turn on replication stream`,
	)
	EnableHistoryReplicationDLQV2 = NewBoolGlobalSetting(
		"history.enableHistoryReplicationDLQV2",
		false,
		`EnableHistoryReplicationDLQV2 switches to the DLQ v2 implementation for history replication. See details in
[go.temporal.io/server/common/persistence.QueueV2]. This feature is currently in development. Do NOT use it in
production.`,
	)

	HistoryRPS = NewIntGlobalSetting(
		"history.rps",
		3000,
		`HistoryRPS is request rate per second for each history host`,
	)
	HistoryPersistenceMaxQPS = NewIntGlobalSetting(
		"history.persistenceMaxQPS",
		9000,
		`HistoryPersistenceMaxQPS is the max qps history host can query DB`,
	)
	HistoryPersistenceGlobalMaxQPS = NewIntGlobalSetting(
		"history.persistenceGlobalMaxQPS",
		0,
		`HistoryPersistenceGlobalMaxQPS is the max qps history cluster can query DB`,
	)
	HistoryPersistenceNamespaceMaxQPS = NewIntNamespaceSetting(
		"history.persistenceNamespaceMaxQPS",
		0,
		`HistoryPersistenceNamespaceMaxQPS is the max qps each namespace on history host can query DB
If value less or equal to 0, will fall back to HistoryPersistenceMaxQPS`,
	)
	HistoryPersistenceGlobalNamespaceMaxQPS = NewIntNamespaceSetting(
		"history.persistenceGlobalNamespaceMaxQPS",
		0,
		`HistoryPersistenceNamespaceMaxQPS is the max qps each namespace in history cluster can query DB`,
	)
	HistoryPersistencePerShardNamespaceMaxQPS = NewIntNamespaceSetting(
		"history.persistencePerShardNamespaceMaxQPS",
		0,
		`HistoryPersistencePerShardNamespaceMaxQPS is the max qps each namespace on a shard can query DB`,
	)
	HistoryEnablePersistencePriorityRateLimiting = NewBoolGlobalSetting(
		"history.enablePersistencePriorityRateLimiting",
		true,
		`HistoryEnablePersistencePriorityRateLimiting indicates if priority rate limiting is enabled in history persistence client`,
	)
	HistoryPersistenceDynamicRateLimitingParams = NewMapGlobalSetting(
		"history.persistenceDynamicRateLimitingParams",
		DefaultDynamicRateLimitingParams,
		`HistoryPersistenceDynamicRateLimitingParams is a map that contains all adjustable dynamic rate limiting params
see DefaultDynamicRateLimitingParams for available options and defaults`,
	)
	HistoryLongPollExpirationInterval = NewDurationNamespaceSetting(
		"history.longPollExpirationInterval",
		time.Second*20,
		`HistoryLongPollExpirationInterval is the long poll expiration interval in the history service`,
	)
	HistoryCacheSizeBasedLimit = NewBoolGlobalSetting(
		"history.cacheSizeBasedLimit",
		false,
		`HistoryCacheSizeBasedLimit if true, size of the history cache will be limited by HistoryCacheMaxSizeBytes
and HistoryCacheHostLevelMaxSizeBytes. Otherwise, entry count in the history cache will be limited by
HistoryCacheMaxSize and HistoryCacheHostLevelMaxSize.`,
	)
	HistoryCacheInitialSize = NewIntGlobalSetting(
		"history.cacheInitialSize",
		128,
		`HistoryCacheInitialSize is initial size of history cache`,
	)
	HistoryCacheMaxSize = NewIntGlobalSetting(
		"history.cacheMaxSize",
		512,
		`HistoryCacheMaxSize is the maximum number of entries in the shard level history cache`,
	)
	HistoryCacheMaxSizeBytes = NewIntGlobalSetting(
		"history.cacheMaxSizeBytes",
		512*4*1024,
		`HistoryCacheMaxSizeBytes is the maximum size of the shard level history cache in bytes. This is only used if
HistoryCacheSizeBasedLimit is set to true.`,
	)
	HistoryCacheTTL = NewDurationGlobalSetting(
		"history.cacheTTL",
		time.Hour,
		`HistoryCacheTTL is TTL of history cache`,
	)
	HistoryCacheNonUserContextLockTimeout = NewDurationGlobalSetting(
		"history.cacheNonUserContextLockTimeout",
		500*time.Millisecond,
		`HistoryCacheNonUserContextLockTimeout controls how long non-user call (callerType != API or Operator)
will wait on workflow lock acquisition. Requires service restart to take effect.`,
	)
	EnableHostHistoryCache = NewBoolGlobalSetting(
		"history.enableHostHistoryCache",
		false,
		`EnableHostHistoryCache controls if the history cache is host level`,
	)
	HistoryCacheHostLevelMaxSize = NewIntGlobalSetting(
		"history.hostLevelCacheMaxSize",
		256000,
		`HistoryCacheHostLevelMaxSize is the maximum number of entries in the host level history cache`,
	)
	HistoryCacheHostLevelMaxSizeBytes = NewIntGlobalSetting(
		"history.hostLevelCacheMaxSizeBytes",
		256000*4*1024,
		`HistoryCacheHostLevelMaxSizeBytes is the maximum size of the host level history cache. This is only used if
HistoryCacheSizeBasedLimit is set to true.`,
	)
	EnableMutableStateTransitionHistory = NewBoolGlobalSetting(
		"history.enableMutableStateTransitionHistory",
		false,
		`EnableMutableStateTransitionHistory controls whether to record state transition history in mutable state records.
The feature is used in the hierarchical state machine framework and is considered unstable as the structure may
change with the pending replication design.`,
	)
	EnableWorkflowExecutionTimeoutTimer = NewBoolGlobalSetting(
		"history.enableWorkflowExecutionTimeoutTimer",
		false,
		`EnableWorkflowExecutionTimeoutTimer controls whether to enable the new logic for generating a workflow execution
timeout timer when execution timeout is specified when starting a workflow.
For backward compatibility, this feature is disabled by default and should only be enabled after server version
containing this flag is deployed to all history service nodes in the cluster.`,
	)
	HistoryStartupMembershipJoinDelay = NewDurationGlobalSetting(
		"history.startupMembershipJoinDelay",
		0*time.Second,
		`HistoryStartupMembershipJoinDelay is the duration a history instance waits
before joining membership after starting.`,
	)
	HistoryShutdownDrainDuration = NewDurationGlobalSetting(
		"history.shutdownDrainDuration",
		0*time.Second,
		`HistoryShutdownDrainDuration is the duration of traffic drain during shutdown`,
	)
	XDCCacheMaxSizeBytes = NewIntGlobalSetting(
		"history.xdcCacheMaxSizeBytes",
		8*1024*1024,
		`XDCCacheMaxSizeBytes is max size of events cache in bytes`,
	)
	EventsCacheMaxSizeBytes = NewIntGlobalSetting(
		"history.eventsCacheMaxSizeBytes",
		512*1024,
		`EventsCacheMaxSizeBytes is max size of the shard level events cache in bytes`,
	)
	EventsHostLevelCacheMaxSizeBytes = NewIntGlobalSetting(
		"history.eventsHostLevelCacheMaxSizeBytes",
		512*512*1024,
		`EventsHostLevelCacheMaxSizeBytes is max size of the host level events cache in bytes`,
	)
	EventsCacheTTL = NewDurationGlobalSetting(
		"history.eventsCacheTTL",
		time.Hour,
		`EventsCacheTTL is TTL of events cache`,
	)
	EnableHostLevelEventsCache = NewBoolGlobalSetting(
		"history.enableHostLevelEventsCache",
		false,
		`EnableHostLevelEventsCache controls if the events cache is host level`,
	)
	AcquireShardInterval = NewDurationGlobalSetting(
		"history.acquireShardInterval",
		time.Minute,
		`AcquireShardInterval is interval that timer used to acquire shard`,
	)
	AcquireShardConcurrency = NewIntGlobalSetting(
		"history.acquireShardConcurrency",
		10,
		`AcquireShardConcurrency is number of goroutines that can be used to acquire shards in the shard controller.`,
	)
	ShardLingerOwnershipCheckQPS = NewIntGlobalSetting(
		"history.shardLingerOwnershipCheckQPS",
		4,
		`ShardLingerOwnershipCheckQPS is the frequency to perform shard ownership
checks while a shard is lingering.`,
	)
	ShardLingerTimeLimit = NewDurationGlobalSetting(
		"history.shardLingerTimeLimit",
		0,
		`ShardLingerTimeLimit configures if and for how long the shard controller
will temporarily delay closing shards after a membership update, awaiting a
shard ownership lost error from persistence. Not recommended with
persistence layers that are missing AssertShardOwnership support.
If set to zero, shards will not delay closing.`,
	)
	ShardOwnershipAssertionEnabled = NewBoolGlobalSetting(
		"history.shardOwnershipAssertionEnabled",
		false,
		`ShardOwnershipAssertionEnabled configures if the shard ownership is asserted
for API requests when a NotFound or NamespaceNotFound error is returned from
persistence.
NOTE: Shard ownership assertion is not implemented by any persistence implementation
in this codebase, because assertion is not needed for persistence implementation
that guarantees read after write consistency. As a result, even if this config is
enabled, it's a no-op.`,
	)
	HistoryClientOwnershipCachingEnabled = NewBoolGlobalSetting(
		"history.clientOwnershipCachingEnabled",
		false,
		`HistoryClientOwnershipCachingEnabled configures if history clients try to cache
shard ownership information, instead of checking membership for each request.
Only inspected when an instance first creates a history client, so changes
to this require a restart to take effect.`,
	)
	ShardIOConcurrency = NewIntGlobalSetting(
		"history.shardIOConcurrency",
		1,
		`ShardIOConcurrency controls the concurrency of persistence operations in shard context`,
	)
	StandbyClusterDelay = NewDurationGlobalSetting(
		"history.standbyClusterDelay",
		5*time.Minute,
		`StandbyClusterDelay is the artificial delay added to standby cluster's view of active cluster's time`,
	)
	StandbyTaskMissingEventsResendDelay = NewDurationTaskTypeSetting(
		"history.standbyTaskMissingEventsResendDelay",
		10*time.Minute,
		`StandbyTaskMissingEventsResendDelay is the amount of time standby cluster's will wait (if events are missing)
before calling remote for missing events`,
	)
	StandbyTaskMissingEventsDiscardDelay = NewDurationTaskTypeSetting(
		"history.standbyTaskMissingEventsDiscardDelay",
		15*time.Minute,
		`StandbyTaskMissingEventsDiscardDelay is the amount of time standby cluster's will wait (if events are missing)
before discarding the task`,
	)
	QueuePendingTaskCriticalCount = NewIntGlobalSetting(
		"history.queuePendingTaskCriticalCount",
		9000,
		`QueuePendingTaskCriticalCount is the max number of pending task in one queue
before triggering queue slice splitting and unloading`,
	)
	QueueReaderStuckCriticalAttempts = NewIntGlobalSetting(
		"history.queueReaderStuckCriticalAttempts",
		3,
		`QueueReaderStuckCriticalAttempts is the max number of task loading attempts for a certain task range
before that task range is split into a separate slice to unblock loading for later range.
currently only work for scheduled queues and the task range is 1s.`,
	)
	QueueCriticalSlicesCount = NewIntGlobalSetting(
		"history.queueCriticalSlicesCount",
		50,
		`QueueCriticalSlicesCount is the max number of slices in one queue
before force compacting slices`,
	)
	QueuePendingTaskMaxCount = NewIntGlobalSetting(
		"history.queuePendingTasksMaxCount",
		10000,
		`QueuePendingTaskMaxCount is the max number of task pending tasks in one queue before stop
loading new tasks into memory. While QueuePendingTaskCriticalCount won't stop task loading
for the entire queue but only trigger a queue action to unload tasks. Ideally this max count
limit should not be hit and task unloading should happen once critical count is exceeded. But
since queue action is async, we need this hard limit.`,
	)
	ContinueAsNewMinInterval = NewDurationNamespaceSetting(
		"history.continueAsNewMinInterval",
		time.Second,
		`ContinueAsNewMinInterval is the minimal interval between continue_as_new executions.
This is needed to prevent tight loop continue_as_new spin. Default is 1s.`,
	)

	TaskSchedulerEnableRateLimiter = NewBoolGlobalSetting(
		"history.taskSchedulerEnableRateLimiter",
		false,
		`TaskSchedulerEnableRateLimiter indicates if task scheduler rate limiter should be enabled`,
	)
	TaskSchedulerEnableRateLimiterShadowMode = NewBoolGlobalSetting(
		"history.taskSchedulerEnableRateLimiterShadowMode",
		true,
		`TaskSchedulerEnableRateLimiterShadowMode indicates if task scheduler rate limiter should run in shadow mode
i.e. through rate limiter and emit metrics but do not actually block/throttle task scheduling`,
	)
	TaskSchedulerRateLimiterStartupDelay = NewDurationGlobalSetting(
		"history.taskSchedulerRateLimiterStartupDelay",
		5*time.Second,
		`TaskSchedulerRateLimiterStartupDelay is the duration to wait after startup before enforcing task scheduler rate limiting`,
	)
	TaskSchedulerGlobalMaxQPS = NewIntGlobalSetting(
		"history.taskSchedulerGlobalMaxQPS",
		0,
		`TaskSchedulerGlobalMaxQPS is the max qps all task schedulers in the cluster can schedule tasks
If value less or equal to 0, will fall back to TaskSchedulerMaxQPS`,
	)
	TaskSchedulerMaxQPS = NewIntGlobalSetting(
		"history.taskSchedulerMaxQPS",
		0,
		`TaskSchedulerMaxQPS is the max qps task schedulers on a host can schedule tasks
If value less or equal to 0, will fall back to HistoryPersistenceMaxQPS`,
	)
	TaskSchedulerGlobalNamespaceMaxQPS = NewIntNamespaceSetting(
		"history.taskSchedulerGlobalNamespaceMaxQPS",
		0,
		`TaskSchedulerGlobalNamespaceMaxQPS is the max qps all task schedulers in the cluster can schedule tasks for a certain namespace
If value less or equal to 0, will fall back to TaskSchedulerNamespaceMaxQPS`,
	)
	TaskSchedulerNamespaceMaxQPS = NewIntNamespaceSetting(
		"history.taskSchedulerNamespaceMaxQPS",
		0,
		`TaskSchedulerNamespaceMaxQPS is the max qps task schedulers on a host can schedule tasks for a certain namespace
If value less or equal to 0, will fall back to HistoryPersistenceNamespaceMaxQPS`,
	)

	TimerTaskBatchSize = NewIntGlobalSetting(
		"history.timerTaskBatchSize",
		100,
		`TimerTaskBatchSize is batch size for timer processor to process tasks`,
	)
	TimerProcessorSchedulerWorkerCount = NewIntGlobalSetting(
		"history.timerProcessorSchedulerWorkerCount",
		512,
		`TimerProcessorSchedulerWorkerCount is the number of workers in the host level task scheduler for timer processor`,
	)
	TimerProcessorSchedulerActiveRoundRobinWeights = NewMapNamespaceSetting(
		"history.timerProcessorSchedulerActiveRoundRobinWeights",
		nil, // actual default is in service/history/configs package
		`TimerProcessorSchedulerActiveRoundRobinWeights is the priority round robin weights used by timer task scheduler for active namespaces`,
	)
	TimerProcessorSchedulerStandbyRoundRobinWeights = NewMapNamespaceSetting(
		"history.timerProcessorSchedulerStandbyRoundRobinWeights",
		nil, // actual default is in service/history/configs package
		`TimerProcessorSchedulerStandbyRoundRobinWeights is the priority round robin weights used by timer task scheduler for standby namespaces`,
	)
	TimerProcessorUpdateAckInterval = NewDurationGlobalSetting(
		"history.timerProcessorUpdateAckInterval",
		30*time.Second,
		`TimerProcessorUpdateAckInterval is update interval for timer processor`,
	)
	TimerProcessorUpdateAckIntervalJitterCoefficient = NewFloatGlobalSetting(
		"history.timerProcessorUpdateAckIntervalJitterCoefficient",
		0.15,
		`TimerProcessorUpdateAckIntervalJitterCoefficient is the update interval jitter coefficient`,
	)
	TimerProcessorMaxPollRPS = NewIntGlobalSetting(
		"history.timerProcessorMaxPollRPS",
		20,
		`TimerProcessorMaxPollRPS is max poll rate per second for timer processor`,
	)
	TimerProcessorMaxPollHostRPS = NewIntGlobalSetting(
		"history.timerProcessorMaxPollHostRPS",
		0,
		`TimerProcessorMaxPollHostRPS is max poll rate per second for all timer processor on a host`,
	)
	TimerProcessorMaxPollInterval = NewDurationGlobalSetting(
		"history.timerProcessorMaxPollInterval",
		5*time.Minute,
		`TimerProcessorMaxPollInterval is max poll interval for timer processor`,
	)
	TimerProcessorMaxPollIntervalJitterCoefficient = NewFloatGlobalSetting(
		"history.timerProcessorMaxPollIntervalJitterCoefficient",
		0.15,
		`TimerProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	)
	TimerProcessorPollBackoffInterval = NewDurationGlobalSetting(
		"history.timerProcessorPollBackoffInterval",
		5*time.Second,
		`TimerProcessorPollBackoffInterval is the poll backoff interval if task redispatcher's size exceeds limit for timer processor`,
	)
	TimerProcessorMaxTimeShift = NewDurationGlobalSetting(
		"history.timerProcessorMaxTimeShift",
		1*time.Second,
		`TimerProcessorMaxTimeShift is the max shift timer processor can have`,
	)
	TimerQueueMaxReaderCount = NewIntGlobalSetting(
		"history.timerQueueMaxReaderCount",
		2,
		`TimerQueueMaxReaderCount is the max number of readers in one multi-cursor timer queue`,
	)
	RetentionTimerJitterDuration = NewDurationGlobalSetting(
		"history.retentionTimerJitterDuration",
		30*time.Minute,
		`RetentionTimerJitterDuration is a time duration jitter to distribute timer from T0 to T0 + jitter duration`,
	)

	MemoryTimerProcessorSchedulerWorkerCount = NewIntGlobalSetting(
		"history.memoryTimerProcessorSchedulerWorkerCount",
		64,
		`MemoryTimerProcessorSchedulerWorkerCount is the number of workers in the task scheduler for in memory timer processor.`,
	)

	TransferTaskBatchSize = NewIntGlobalSetting(
		"history.transferTaskBatchSize",
		100,
		`TransferTaskBatchSize is batch size for transferQueueProcessor`,
	)
	TransferProcessorMaxPollRPS = NewIntGlobalSetting(
		"history.transferProcessorMaxPollRPS",
		20,
		`TransferProcessorMaxPollRPS is max poll rate per second for transferQueueProcessor`,
	)
	TransferProcessorMaxPollHostRPS = NewIntGlobalSetting(
		"history.transferProcessorMaxPollHostRPS",
		0,
		`TransferProcessorMaxPollHostRPS is max poll rate per second for all transferQueueProcessor on a host`,
	)
	TransferProcessorSchedulerWorkerCount = NewIntGlobalSetting(
		"history.transferProcessorSchedulerWorkerCount",
		512,
		`TransferProcessorSchedulerWorkerCount is the number of workers in the host level task scheduler for transferQueueProcessor`,
	)
	TransferProcessorSchedulerActiveRoundRobinWeights = NewMapNamespaceSetting(
		"history.transferProcessorSchedulerActiveRoundRobinWeights",
		nil, // actual default is in service/history/configs package
		`TransferProcessorSchedulerActiveRoundRobinWeights is the priority round robin weights used by transfer task scheduler for active namespaces`,
	)
	TransferProcessorSchedulerStandbyRoundRobinWeights = NewMapNamespaceSetting(
		"history.transferProcessorSchedulerStandbyRoundRobinWeights",
		nil, // actual default is in service/history/configs package
		`TransferProcessorSchedulerStandbyRoundRobinWeights is the priority round robin weights used by transfer task scheduler for standby namespaces`,
	)
	TransferProcessorMaxPollInterval = NewDurationGlobalSetting(
		"history.transferProcessorMaxPollInterval",
		1*time.Minute,
		`TransferProcessorMaxPollInterval max poll interval for transferQueueProcessor`,
	)
	TransferProcessorMaxPollIntervalJitterCoefficient = NewFloatGlobalSetting(
		"history.transferProcessorMaxPollIntervalJitterCoefficient",
		0.15,
		`TransferProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	)
	TransferProcessorUpdateAckInterval = NewDurationGlobalSetting(
		"history.transferProcessorUpdateAckInterval",
		30*time.Second,
		`TransferProcessorUpdateAckInterval is update interval for transferQueueProcessor`,
	)
	TransferProcessorUpdateAckIntervalJitterCoefficient = NewFloatGlobalSetting(
		"history.transferProcessorUpdateAckIntervalJitterCoefficient",
		0.15,
		`TransferProcessorUpdateAckIntervalJitterCoefficient is the update interval jitter coefficient`,
	)
	TransferProcessorPollBackoffInterval = NewDurationGlobalSetting(
		"history.transferProcessorPollBackoffInterval",
		5*time.Second,
		`TransferProcessorPollBackoffInterval is the poll backoff interval if task redispatcher's size exceeds limit for transferQueueProcessor`,
	)
	TransferProcessorEnsureCloseBeforeDelete = NewBoolGlobalSetting(
		"history.transferProcessorEnsureCloseBeforeDelete",
		true,
		`TransferProcessorEnsureCloseBeforeDelete means we ensure the execution is closed before we delete it`,
	)
	TransferQueueMaxReaderCount = NewIntGlobalSetting(
		"history.transferQueueMaxReaderCount",
		2,
		`TransferQueueMaxReaderCount is the max number of readers in one multi-cursor transfer queue`,
	)

	OutboundProcessorEnabled = NewBoolGlobalSetting(
		"history.outboundProcessorEnabled",
		false,
		`OutboundProcessorEnabled enables starting the outbound queue processor.`,
	)
	OutboundTaskBatchSize = NewIntGlobalSetting(
		"history.outboundTaskBatchSize",
		100,
		`OutboundTaskBatchSize is batch size for outboundQueueFactory`,
	)
	OutboundProcessorMaxPollRPS = NewIntGlobalSetting(
		"history.outboundProcessorMaxPollRPS",
		20,
		`OutboundProcessorMaxPollRPS is max poll rate per second for outboundQueueFactory`,
	)
	OutboundProcessorMaxPollHostRPS = NewIntGlobalSetting(
		"history.outboundProcessorMaxPollHostRPS",
		0,
		`OutboundProcessorMaxPollHostRPS is max poll rate per second for all outboundQueueFactory on a host`,
	)
	OutboundProcessorMaxPollInterval = NewDurationGlobalSetting(
		"history.outboundProcessorMaxPollInterval",
		1*time.Minute,
		`OutboundProcessorMaxPollInterval max poll interval for outboundQueueFactory`,
	)
	OutboundProcessorMaxPollIntervalJitterCoefficient = NewFloatGlobalSetting(
		"history.outboundProcessorMaxPollIntervalJitterCoefficient",
		0.15,
		`OutboundProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	)
	OutboundProcessorUpdateAckInterval = NewDurationGlobalSetting(
		"history.outboundProcessorUpdateAckInterval",
		30*time.Second,
		`OutboundProcessorUpdateAckInterval is update interval for outboundQueueFactory`,
	)
	OutboundProcessorUpdateAckIntervalJitterCoefficient = NewFloatGlobalSetting(
		"history.outboundProcessorUpdateAckIntervalJitterCoefficient",
		0.15,
		`OutboundProcessorUpdateAckIntervalJitterCoefficient is the update interval jitter coefficient`,
	)
	OutboundProcessorPollBackoffInterval = NewDurationGlobalSetting(
		"history.outboundProcessorPollBackoffInterval",
		5*time.Second,
		`OutboundProcessorPollBackoffInterval is the poll backoff interval if task redispatcher's size exceeds limit for outboundQueueFactory`,
	)
	OutboundQueueMaxReaderCount = NewIntGlobalSetting(
		"history.outboundQueueMaxReaderCount",
		4,
		`OutboundQueueMaxReaderCount is the max number of readers in one multi-cursor outbound queue`,
	)

	VisibilityTaskBatchSize = NewIntGlobalSetting(
		"history.visibilityTaskBatchSize",
		100,
		`VisibilityTaskBatchSize is batch size for visibilityQueueProcessor`,
	)
	VisibilityProcessorMaxPollRPS = NewIntGlobalSetting(
		"history.visibilityProcessorMaxPollRPS",
		20,
		`VisibilityProcessorMaxPollRPS is max poll rate per second for visibilityQueueProcessor`,
	)
	VisibilityProcessorMaxPollHostRPS = NewIntGlobalSetting(
		"history.visibilityProcessorMaxPollHostRPS",
		0,
		`VisibilityProcessorMaxPollHostRPS is max poll rate per second for all visibilityQueueProcessor on a host`,
	)
	VisibilityProcessorSchedulerWorkerCount = NewIntGlobalSetting(
		"history.visibilityProcessorSchedulerWorkerCount",
		512,
		`VisibilityProcessorSchedulerWorkerCount is the number of workers in the host level task scheduler for visibilityQueueProcessor`,
	)
	VisibilityProcessorSchedulerActiveRoundRobinWeights = NewMapNamespaceSetting(
		"history.visibilityProcessorSchedulerActiveRoundRobinWeights",
		nil, // actual default is in service/history/configs package
		`VisibilityProcessorSchedulerActiveRoundRobinWeights is the priority round robin weights by visibility task scheduler for active namespaces`,
	)
	VisibilityProcessorSchedulerStandbyRoundRobinWeights = NewMapNamespaceSetting(
		"history.visibilityProcessorSchedulerStandbyRoundRobinWeights",
		nil, // actual default is in service/history/configs package
		`VisibilityProcessorSchedulerStandbyRoundRobinWeights is the priority round robin weights by visibility task scheduler for standby namespaces`,
	)
	VisibilityProcessorMaxPollInterval = NewDurationGlobalSetting(
		"history.visibilityProcessorMaxPollInterval",
		1*time.Minute,
		`VisibilityProcessorMaxPollInterval max poll interval for visibilityQueueProcessor`,
	)
	VisibilityProcessorMaxPollIntervalJitterCoefficient = NewFloatGlobalSetting(
		"history.visibilityProcessorMaxPollIntervalJitterCoefficient",
		0.15,
		`VisibilityProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	)
	VisibilityProcessorUpdateAckInterval = NewDurationGlobalSetting(
		"history.visibilityProcessorUpdateAckInterval",
		30*time.Second,
		`VisibilityProcessorUpdateAckInterval is update interval for visibilityQueueProcessor`,
	)
	VisibilityProcessorUpdateAckIntervalJitterCoefficient = NewFloatGlobalSetting(
		"history.visibilityProcessorUpdateAckIntervalJitterCoefficient",
		0.15,
		`VisibilityProcessorUpdateAckIntervalJitterCoefficient is the update interval jitter coefficient`,
	)
	VisibilityProcessorPollBackoffInterval = NewDurationGlobalSetting(
		"history.visibilityProcessorPollBackoffInterval",
		5*time.Second,
		`VisibilityProcessorPollBackoffInterval is the poll backoff interval if task redispatcher's size exceeds limit for visibilityQueueProcessor`,
	)
	VisibilityProcessorEnsureCloseBeforeDelete = NewBoolGlobalSetting(
		"history.visibilityProcessorEnsureCloseBeforeDelete",
		false,
		`VisibilityProcessorEnsureCloseBeforeDelete means we ensure the visibility of an execution is closed before we delete its visibility records`,
	)
	VisibilityProcessorEnableCloseWorkflowCleanup = NewBoolNamespaceSetting(
		"history.visibilityProcessorEnableCloseWorkflowCleanup",
		false,
		`VisibilityProcessorEnableCloseWorkflowCleanup to clean up the mutable state after visibility
close task has been processed. Must use Elasticsearch as visibility store, otherwise workflow
data (eg: search attributes) will be lost after workflow is closed.`,
	)
	VisibilityQueueMaxReaderCount = NewIntGlobalSetting(
		"history.visibilityQueueMaxReaderCount",
		2,
		`VisibilityQueueMaxReaderCount is the max number of readers in one multi-cursor visibility queue`,
	)

	ArchivalTaskBatchSize = NewIntGlobalSetting(
		"history.archivalTaskBatchSize",
		100,
		`ArchivalTaskBatchSize is batch size for archivalQueueProcessor`,
	)
	ArchivalProcessorMaxPollRPS = NewIntGlobalSetting(
		"history.archivalProcessorMaxPollRPS",
		20,
		`ArchivalProcessorMaxPollRPS is max poll rate per second for archivalQueueProcessor`,
	)
	ArchivalProcessorMaxPollHostRPS = NewIntGlobalSetting(
		"history.archivalProcessorMaxPollHostRPS",
		0,
		`ArchivalProcessorMaxPollHostRPS is max poll rate per second for all archivalQueueProcessor on a host`,
	)
	ArchivalProcessorSchedulerWorkerCount = NewIntGlobalSetting(
		"history.archivalProcessorSchedulerWorkerCount",
		512,
		`ArchivalProcessorSchedulerWorkerCount is the number of workers in the host level task scheduler for
archivalQueueProcessor`,
	)
	ArchivalProcessorMaxPollInterval = NewDurationGlobalSetting(
		"history.archivalProcessorMaxPollInterval",
		5*time.Minute,
		`ArchivalProcessorMaxPollInterval max poll interval for archivalQueueProcessor`,
	)
	ArchivalProcessorMaxPollIntervalJitterCoefficient = NewFloatGlobalSetting(
		"history.archivalProcessorMaxPollIntervalJitterCoefficient",
		0.15,
		`ArchivalProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	)
	ArchivalProcessorUpdateAckInterval = NewDurationGlobalSetting(
		"history.archivalProcessorUpdateAckInterval",
		30*time.Second,
		`ArchivalProcessorUpdateAckInterval is update interval for archivalQueueProcessor`,
	)
	ArchivalProcessorUpdateAckIntervalJitterCoefficient = NewFloatGlobalSetting(
		"history.archivalProcessorUpdateAckIntervalJitterCoefficient",
		0.15,
		`ArchivalProcessorUpdateAckIntervalJitterCoefficient is the update interval jitter coefficient`,
	)
	ArchivalProcessorPollBackoffInterval = NewDurationGlobalSetting(
		"history.archivalProcessorPollBackoffInterval",
		5*time.Second,
		`ArchivalProcessorPollBackoffInterval is the poll backoff interval if task redispatcher's size exceeds limit for
archivalQueueProcessor`,
	)
	ArchivalProcessorArchiveDelay = NewDurationGlobalSetting(
		"history.archivalProcessorArchiveDelay",
		5*time.Minute,
		`ArchivalProcessorArchiveDelay is the delay before archivalQueueProcessor starts to process archival tasks`,
	)
	ArchivalBackendMaxRPS = NewFloatGlobalSetting(
		"history.archivalBackendMaxRPS",
		10000.0,
		`ArchivalBackendMaxRPS is the maximum rate of requests per second to the archival backend`,
	)
	ArchivalQueueMaxReaderCount = NewIntGlobalSetting(
		"history.archivalQueueMaxReaderCount",
		2,
		`ArchivalQueueMaxReaderCount is the max number of readers in one multi-cursor archival queue`,
	)

	WorkflowExecutionMaxInFlightUpdates = NewIntNamespaceSetting(
		"history.maxInFlightUpdates",
		10,
		`WorkflowExecutionMaxInFlightUpdates is the max number of updates that can be in-flight (admitted but not yet completed) for any given workflow execution.`,
	)
	WorkflowExecutionMaxTotalUpdates = NewIntNamespaceSetting(
		"history.maxTotalUpdates",
		2000,
		`WorkflowExecutionMaxTotalUpdates is the max number of updates that any given workflow execution can receive.`,
	)

	ReplicatorTaskBatchSize = NewIntGlobalSetting(
		"history.replicatorTaskBatchSize",
		25,
		`ReplicatorTaskBatchSize is batch size for ReplicatorProcessor`,
	)
	ReplicatorMaxSkipTaskCount = NewIntGlobalSetting(
		"history.replicatorMaxSkipTaskCount",
		250,
		`ReplicatorMaxSkipTaskCount is maximum number of tasks that can be skipped during tasks pagination due to not meeting filtering conditions (e.g. missed namespace).`,
	)
	ReplicatorProcessorMaxPollInterval = NewDurationGlobalSetting(
		"history.replicatorProcessorMaxPollInterval",
		1*time.Minute,
		`ReplicatorProcessorMaxPollInterval is max poll interval for ReplicatorProcessor`,
	)
	ReplicatorProcessorMaxPollIntervalJitterCoefficient = NewFloatGlobalSetting(
		"history.replicatorProcessorMaxPollIntervalJitterCoefficient",
		0.15,
		`ReplicatorProcessorMaxPollIntervalJitterCoefficient is the max poll interval jitter coefficient`,
	)
	MaximumBufferedEventsBatch = NewIntGlobalSetting(
		"history.maximumBufferedEventsBatch",
		100,
		`MaximumBufferedEventsBatch is the maximum permissible number of buffered events for any given mutable state.`,
	)
	MaximumBufferedEventsSizeInBytes = NewIntGlobalSetting(
		"history.maximumBufferedEventsSizeInBytes",
		2*1024*1024,
		`MaximumBufferedEventsSizeInBytes is the maximum permissible size of all buffered events for any given mutable
state. The total size is determined by the sum of the size, in bytes, of each HistoryEvent proto.`,
	)
	MaximumSignalsPerExecution = NewIntNamespaceSetting(
		"history.maximumSignalsPerExecution",
		10000,
		`MaximumSignalsPerExecution is max number of signals supported by single execution`,
	)
	ShardUpdateMinInterval = NewDurationGlobalSetting(
		"history.shardUpdateMinInterval",
		5*time.Minute,
		`ShardUpdateMinInterval is the minimal time interval which the shard info can be updated`,
	)
	ShardUpdateMinTasksCompleted = NewIntGlobalSetting(
		"history.shardUpdateMinTasksCompleted",
		1000,
		`ShardUpdateMinTasksCompleted is the minimum number of tasks which must be completed (across all queues) before the shard info can be updated.
Note that once history.shardUpdateMinInterval amount of time has passed we'll update the shard info regardless of the number of tasks completed.
When the this config is zero or lower we will only update shard info at most once every history.shardUpdateMinInterval.`,
	)
	ShardSyncMinInterval = NewDurationGlobalSetting(
		"history.shardSyncMinInterval",
		5*time.Minute,
		`ShardSyncMinInterval is the minimal time interval which the shard info should be sync to remote`,
	)
	EmitShardLagLog = NewBoolGlobalSetting(
		"history.emitShardLagLog",
		false,
		`EmitShardLagLog whether emit the shard lag log`,
	)
	DefaultEventEncoding = NewStringNamespaceSetting(
		"history.defaultEventEncoding",
		enumspb.ENCODING_TYPE_PROTO3.String(),
		`DefaultEventEncoding is the encoding type for history events`,
	)
	DefaultActivityRetryPolicy = NewMapNamespaceSetting(
		"history.defaultActivityRetryPolicy",
		GetDefaultRetryPolicyConfigOptions(),
		`DefaultActivityRetryPolicy represents the out-of-box retry policy for activities where
the user has not specified an explicit RetryPolicy`,
	)
	DefaultWorkflowRetryPolicy = NewMapNamespaceSetting(
		"history.defaultWorkflowRetryPolicy",
		GetDefaultRetryPolicyConfigOptions(),
		`DefaultWorkflowRetryPolicy represents the out-of-box retry policy for unset fields
where the user has set an explicit RetryPolicy, but not specified all the fields`,
	)
	HistoryMaxAutoResetPoints = NewIntNamespaceSetting(
		"history.historyMaxAutoResetPoints",
		primitives.DefaultHistoryMaxAutoResetPoints,
		`HistoryMaxAutoResetPoints is the key for max number of auto reset points stored in mutableState`,
	)
	EnableParentClosePolicy = NewBoolNamespaceSetting(
		"history.enableParentClosePolicy",
		true,
		`EnableParentClosePolicy whether to  ParentClosePolicy`,
	)
	ParentClosePolicyThreshold = NewIntNamespaceSetting(
		"history.parentClosePolicyThreshold",
		10,
		`ParentClosePolicyThreshold decides that parent close policy will be processed by sys workers(if enabled) if
the number of children greater than or equal to this threshold`,
	)
	NumParentClosePolicySystemWorkflows = NewIntGlobalSetting(
		"history.numParentClosePolicySystemWorkflows",
		10,
		`NumParentClosePolicySystemWorkflows is key for number of parentClosePolicy system workflows running in total`,
	)
	HistoryThrottledLogRPS = NewIntGlobalSetting(
		"history.throttledLogRPS",
		4,
		`HistoryThrottledLogRPS is the rate limit on number of log messages emitted per second for throttled logger`,
	)
	WorkflowTaskHeartbeatTimeout = NewDurationNamespaceSetting(
		"history.workflowTaskHeartbeatTimeout",
		time.Minute*30,
		`WorkflowTaskHeartbeatTimeout for workflow task heartbeat`,
	)
	WorkflowTaskCriticalAttempts = NewIntGlobalSetting(
		"history.workflowTaskCriticalAttempt",
		10,
		`WorkflowTaskCriticalAttempts is the number of attempts for a workflow task that's regarded as critical`,
	)
	WorkflowTaskRetryMaxInterval = NewDurationGlobalSetting(
		"history.workflowTaskRetryMaxInterval",
		time.Minute*10,
		`WorkflowTaskRetryMaxInterval is the maximum interval added to a workflow task's startToClose timeout for slowing down retry`,
	)
	DefaultWorkflowTaskTimeout = NewDurationNamespaceSetting(
		"history.defaultWorkflowTaskTimeout",
		primitives.DefaultWorkflowTaskTimeout,
		`DefaultWorkflowTaskTimeout for a workflow task`,
	)
	SkipReapplicationByNamespaceID = NewBoolNamespaceIDSetting(
		"history.SkipReapplicationByNamespaceID",
		false,
		`SkipReapplicationByNamespaceID is whether skipping a event re-application for a namespace`,
	)
	StandbyTaskReReplicationContextTimeout = NewDurationNamespaceIDSetting(
		"history.standbyTaskReReplicationContextTimeout",
		30*time.Second,
		`StandbyTaskReReplicationContextTimeout is the context timeout for standby task re-replication`,
	)
	MaxBufferedQueryCount = NewIntGlobalSetting(
		"history.MaxBufferedQueryCount",
		1,
		`MaxBufferedQueryCount indicates max buffer query count`,
	)
	MutableStateChecksumGenProbability = NewIntNamespaceSetting(
		"history.mutableStateChecksumGenProbability",
		0,
		`MutableStateChecksumGenProbability is the probability [0-100] that checksum will be generated for mutable state`,
	)
	MutableStateChecksumVerifyProbability = NewIntNamespaceSetting(
		"history.mutableStateChecksumVerifyProbability",
		0,
		`MutableStateChecksumVerifyProbability is the probability [0-100] that checksum will be verified for mutable state`,
	)
	MutableStateChecksumInvalidateBefore = NewFloatGlobalSetting(
		"history.mutableStateChecksumInvalidateBefore",
		0,
		`MutableStateChecksumInvalidateBefore is the epoch timestamp before which all checksums are to be discarded`,
	)

	ReplicationTaskFetcherParallelism = NewIntGlobalSetting(
		"history.ReplicationTaskFetcherParallelism",
		4,
		`ReplicationTaskFetcherParallelism determines how many go routines we spin up for fetching tasks`,
	)
	ReplicationTaskFetcherAggregationInterval = NewDurationGlobalSetting(
		"history.ReplicationTaskFetcherAggregationInterval",
		2*time.Second,
		`ReplicationTaskFetcherAggregationInterval determines how frequently the fetch requests are sent`,
	)
	ReplicationTaskFetcherTimerJitterCoefficient = NewFloatGlobalSetting(
		"history.ReplicationTaskFetcherTimerJitterCoefficient",
		0.15,
		`ReplicationTaskFetcherTimerJitterCoefficient is the jitter for fetcher timer`,
	)
	ReplicationTaskFetcherErrorRetryWait = NewDurationGlobalSetting(
		"history.ReplicationTaskFetcherErrorRetryWait",
		time.Second,
		`ReplicationTaskFetcherErrorRetryWait is the wait time when fetcher encounters error`,
	)
	ReplicationTaskProcessorErrorRetryWait = NewDurationShardIDSetting(
		"history.ReplicationTaskProcessorErrorRetryWait",
		1*time.Second,
		`ReplicationTaskProcessorErrorRetryWait is the initial retry wait when we see errors in applying replication tasks`,
	)
	ReplicationTaskProcessorErrorRetryBackoffCoefficient = NewFloatShardIDSetting(
		"history.ReplicationTaskProcessorErrorRetryBackoffCoefficient",
		1.2,
		`ReplicationTaskProcessorErrorRetryBackoffCoefficient is the retry wait backoff time coefficient`,
	)
	ReplicationTaskProcessorErrorRetryMaxInterval = NewDurationShardIDSetting(
		"history.ReplicationTaskProcessorErrorRetryMaxInterval",
		5*time.Second,
		`ReplicationTaskProcessorErrorRetryMaxInterval is the retry wait backoff max duration`,
	)
	ReplicationTaskProcessorErrorRetryMaxAttempts = NewIntShardIDSetting(
		"history.ReplicationTaskProcessorErrorRetryMaxAttempts",
		80,
		`ReplicationTaskProcessorErrorRetryMaxAttempts is the max retry attempts for applying replication tasks`,
	)
	ReplicationTaskProcessorErrorRetryExpiration = NewDurationShardIDSetting(
		"history.ReplicationTaskProcessorErrorRetryExpiration",
		5*time.Minute,
		`ReplicationTaskProcessorErrorRetryExpiration is the max retry duration for applying replication tasks`,
	)
	ReplicationTaskProcessorNoTaskInitialWait = NewDurationShardIDSetting(
		"history.ReplicationTaskProcessorNoTaskInitialWait",
		2*time.Second,
		`ReplicationTaskProcessorNoTaskInitialWait is the wait time when not ask is returned`,
	)
	ReplicationTaskProcessorCleanupInterval = NewDurationShardIDSetting(
		"history.ReplicationTaskProcessorCleanupInterval",
		1*time.Minute,
		`ReplicationTaskProcessorCleanupInterval determines how frequently the cleanup replication queue`,
	)
	ReplicationTaskProcessorCleanupJitterCoefficient = NewFloatShardIDSetting(
		"history.ReplicationTaskProcessorCleanupJitterCoefficient",
		0.15,
		`ReplicationTaskProcessorCleanupJitterCoefficient is the jitter for cleanup timer`,
	)
	ReplicationTaskProcessorHostQPS = NewFloatGlobalSetting(
		"history.ReplicationTaskProcessorHostQPS",
		1500,
		`ReplicationTaskProcessorHostQPS is the qps of task processing rate limiter on host level`,
	)
	ReplicationTaskProcessorShardQPS = NewFloatGlobalSetting(
		"history.ReplicationTaskProcessorShardQPS",
		30,
		`ReplicationTaskProcessorShardQPS is the qps of task processing rate limiter on shard level`,
	)
	ReplicationEnableDLQMetrics = NewBoolGlobalSetting(
		"history.ReplicationEnableDLQMetrics",
		true,
		`ReplicationEnableDLQMetrics is the flag to emit DLQ metrics`,
	)
	ReplicationEnableUpdateWithNewTaskMerge = NewBoolGlobalSetting(
		"history.ReplicationEnableUpdateWithNewTaskMerge",
		false,
		`ReplicationEnableUpdateWithNewTaskMerge is the flag controlling whether replication task merging logic
should be enabled for non continuedAsNew workflow UpdateWithNew case.`,
	)
	HistoryTaskDLQEnabled = NewBoolGlobalSetting(
		"history.TaskDLQEnabled",
		true,
		`HistoryTaskDLQEnabled enables the history task DLQ. This applies to internal tasks like transfer and timer tasks.
Do not turn this on if you aren't using Cassandra as the history task DLQ is not implemented for other databases.`,
	)
	HistoryTaskDLQUnexpectedErrorAttempts = NewIntGlobalSetting(
		"history.TaskDLQUnexpectedErrorAttempts",
		100,
		`HistoryTaskDLQUnexpectedErrorAttempts is the number of task execution attempts before sending the task to DLQ.`,
	)
	HistoryTaskDLQInternalErrors = NewBoolGlobalSetting(
		"history.TaskDLQInternalErrors",
		false,
		`HistoryTaskDLQInternalErrors causes history task processing to send tasks failing with serviceerror.Internal to
the dlq (or will drop them if not enabled)`,
	)
	HistoryTaskDLQErrorPattern = NewStringGlobalSetting(
		"history.TaskDLQErrorPattern",
		"",
		`HistoryTaskDLQErrorPattern specifies a regular expression. If a task processing error matches with this regex,
that task will be sent to DLQ.`,
	)

	ReplicationStreamSyncStatusDuration = NewDurationGlobalSetting(
		"history.ReplicationStreamSyncStatusDuration",
		1*time.Second,
		`ReplicationStreamSyncStatusDuration sync replication status duration`,
	)
	ReplicationProcessorSchedulerQueueSize = NewIntGlobalSetting(
		"history.ReplicationProcessorSchedulerQueueSize",
		128,
		`ReplicationProcessorSchedulerQueueSize is the replication task executor queue size`,
	)
	ReplicationProcessorSchedulerWorkerCount = NewIntGlobalSetting(
		"history.ReplicationProcessorSchedulerWorkerCount",
		512,
		`ReplicationProcessorSchedulerWorkerCount is the replication task executor worker count`,
	)
	EnableEagerNamespaceRefresher = NewBoolGlobalSetting(
		"history.EnableEagerNamespaceRefresher",
		false,
		`EnableEagerNamespaceRefresher is a feature flag for eagerly refresh namespace during processing replication task`,
	)
	EnableReplicationTaskBatching = NewBoolGlobalSetting(
		"history.EnableReplicationTaskBatching",
		false,
		`EnableReplicationTaskBatching is a feature flag for batching replicate history event task`,
	)
	EnableReplicateLocalGeneratedEvents = NewBoolGlobalSetting(
		"history.EnableReplicateLocalGeneratedEvents",
		false,
		`EnableReplicateLocalGeneratedEvents is a feature flag for replicating locally generated events`,
	)

	// keys for worker

	WorkerPersistenceMaxQPS = NewIntGlobalSetting(
		"worker.persistenceMaxQPS",
		500,
		`WorkerPersistenceMaxQPS is the max qps worker host can query DB`,
	)
	WorkerPersistenceGlobalMaxQPS = NewIntGlobalSetting(
		"worker.persistenceGlobalMaxQPS",
		0,
		`WorkerPersistenceGlobalMaxQPS is the max qps worker cluster can query DB`,
	)
	WorkerPersistenceNamespaceMaxQPS = NewIntNamespaceSetting(
		"worker.persistenceNamespaceMaxQPS",
		0,
		`WorkerPersistenceNamespaceMaxQPS is the max qps each namespace on worker host can query DB`,
	)
	WorkerPersistenceGlobalNamespaceMaxQPS = NewIntNamespaceSetting(
		"worker.persistenceGlobalNamespaceMaxQPS",
		0,
		`WorkerPersistenceNamespaceMaxQPS is the max qps each namespace in worker cluster can query DB`,
	)
	WorkerEnablePersistencePriorityRateLimiting = NewBoolGlobalSetting(
		"worker.enablePersistencePriorityRateLimiting",
		true,
		`WorkerEnablePersistencePriorityRateLimiting indicates if priority rate limiting is enabled in worker persistence client`,
	)
	WorkerPersistenceDynamicRateLimitingParams = NewMapGlobalSetting(
		"worker.persistenceDynamicRateLimitingParams",
		DefaultDynamicRateLimitingParams,
		`WorkerPersistenceDynamicRateLimitingParams is a map that contains all adjustable dynamic rate limiting params
see DefaultDynamicRateLimitingParams for available options and defaults`,
	)
	WorkerIndexerConcurrency = NewIntGlobalSetting(
		"worker.indexerConcurrency",
		100,
		`WorkerIndexerConcurrency is the max concurrent messages to be processed at any given time`,
	)
	WorkerESProcessorNumOfWorkers = NewIntGlobalSetting(
		"worker.ESProcessorNumOfWorkers",
		2,
		`WorkerESProcessorNumOfWorkers is num of workers for esProcessor`,
	)
	WorkerESProcessorBulkActions = NewIntGlobalSetting(
		"worker.ESProcessorBulkActions",
		500,
		`WorkerESProcessorBulkActions is max number of requests in bulk for esProcessor`,
	)
	WorkerESProcessorBulkSize = NewIntGlobalSetting(
		"worker.ESProcessorBulkSize",
		16*1024*1024,
		`WorkerESProcessorBulkSize is max total size of bulk in bytes for esProcessor`,
	)
	WorkerESProcessorFlushInterval = NewDurationGlobalSetting(
		"worker.ESProcessorFlushInterval",
		1*time.Second,
		`WorkerESProcessorFlushInterval is flush interval for esProcessor`,
	)
	WorkerESProcessorAckTimeout = NewDurationGlobalSetting(
		"worker.ESProcessorAckTimeout",
		30*time.Second,
		`WorkerESProcessorAckTimeout is the timeout that store will wait to get ack signal from ES processor.
Should be at least WorkerESProcessorFlushInterval+<time to process request>.`,
	)
	WorkerThrottledLogRPS = NewIntGlobalSetting(
		"worker.throttledLogRPS",
		20,
		`WorkerThrottledLogRPS is the rate limit on number of log messages emitted per second for throttled logger`,
	)
	WorkerScannerMaxConcurrentActivityExecutionSize = NewIntGlobalSetting(
		"worker.ScannerMaxConcurrentActivityExecutionSize",
		10,
		`WorkerScannerMaxConcurrentActivityExecutionSize indicates worker scanner max concurrent activity execution size`,
	)
	WorkerScannerMaxConcurrentWorkflowTaskExecutionSize = NewIntGlobalSetting(
		"worker.ScannerMaxConcurrentWorkflowTaskExecutionSize",
		10,
		`WorkerScannerMaxConcurrentWorkflowTaskExecutionSize indicates worker scanner max concurrent workflow execution size`,
	)
	WorkerScannerMaxConcurrentActivityTaskPollers = NewIntGlobalSetting(
		"worker.ScannerMaxConcurrentActivityTaskPollers",
		8,
		`WorkerScannerMaxConcurrentActivityTaskPollers indicates worker scanner max concurrent activity pollers`,
	)
	WorkerScannerMaxConcurrentWorkflowTaskPollers = NewIntGlobalSetting(
		"worker.ScannerMaxConcurrentWorkflowTaskPollers",
		8,
		`WorkerScannerMaxConcurrentWorkflowTaskPollers indicates worker scanner max concurrent workflow pollers`,
	)
	ScannerPersistenceMaxQPS = NewIntGlobalSetting(
		"worker.scannerPersistenceMaxQPS",
		100,
		`ScannerPersistenceMaxQPS is the maximum rate of persistence calls from worker.Scanner`,
	)
	ExecutionScannerPerHostQPS = NewIntGlobalSetting(
		"worker.executionScannerPerHostQPS",
		10,
		`ExecutionScannerPerHostQPS is the maximum rate of calls per host from executions.Scanner`,
	)
	ExecutionScannerPerShardQPS = NewIntGlobalSetting(
		"worker.executionScannerPerShardQPS",
		1,
		`ExecutionScannerPerShardQPS is the maximum rate of calls per shard from executions.Scanner`,
	)
	ExecutionDataDurationBuffer = NewDurationGlobalSetting(
		"worker.executionDataDurationBuffer",
		time.Hour*24*90,
		`ExecutionDataDurationBuffer is the data TTL duration buffer of execution data`,
	)
	ExecutionScannerWorkerCount = NewIntGlobalSetting(
		"worker.executionScannerWorkerCount",
		8,
		`ExecutionScannerWorkerCount is the execution scavenger worker count`,
	)
	ExecutionScannerHistoryEventIdValidator = NewBoolGlobalSetting(
		"worker.executionEnableHistoryEventIdValidator",
		true,
		`ExecutionScannerHistoryEventIdValidator is the flag to enable history event id validator`,
	)
	TaskQueueScannerEnabled = NewBoolGlobalSetting(
		"worker.taskQueueScannerEnabled",
		true,
		`TaskQueueScannerEnabled indicates if task queue scanner should be started as part of worker.Scanner`,
	)
	BuildIdScavengerEnabled = NewBoolGlobalSetting(
		"worker.buildIdScavengerEnabled",
		false,
		`BuildIdScavengerEnabled indicates if the build id scavenger should be started as part of worker.Scanner`,
	)
	HistoryScannerEnabled = NewBoolGlobalSetting(
		"worker.historyScannerEnabled",
		true,
		`HistoryScannerEnabled indicates if history scanner should be started as part of worker.Scanner`,
	)
	ExecutionsScannerEnabled = NewBoolGlobalSetting(
		"worker.executionsScannerEnabled",
		false,
		`ExecutionsScannerEnabled indicates if executions scanner should be started as part of worker.Scanner`,
	)
	HistoryScannerDataMinAge = NewDurationGlobalSetting(
		"worker.historyScannerDataMinAge",
		60*24*time.Hour,
		`HistoryScannerDataMinAge indicates the history scanner cleanup minimum age.`,
	)
	HistoryScannerVerifyRetention = NewBoolGlobalSetting(
		"worker.historyScannerVerifyRetention",
		true,
		`HistoryScannerVerifyRetention indicates the history scanner verify data retention.
If the service configures with archival feature enabled, update worker.historyScannerVerifyRetention to be double of the data retention.`,
	)
	EnableBatcherGlobal = NewBoolGlobalSetting(
		"worker.enableBatcher",
		true,
		`EnableBatcher decides whether to start old (system namespace) batcher in our worker`,
	)
	EnableBatcherNamespace = NewBoolNamespaceSetting(
		"worker.enableNamespaceBatcher",
		true,
		`EnableBatcher decides whether to start new (per-namespace) batcher in our worker`,
	)
	BatcherRPS = NewIntNamespaceSetting(
		"worker.batcherRPS",
		50,
		`BatcherRPS controls number the rps of batch operations`,
	)
	BatcherConcurrency = NewIntNamespaceSetting(
		"worker.batcherConcurrency",
		5,
		`BatcherConcurrency controls the concurrency of one batch operation`,
	)
	WorkerParentCloseMaxConcurrentActivityExecutionSize = NewIntGlobalSetting(
		"worker.ParentCloseMaxConcurrentActivityExecutionSize",
		1000,
		`WorkerParentCloseMaxConcurrentActivityExecutionSize indicates worker parent close worker max concurrent activity execution size`,
	)
	WorkerParentCloseMaxConcurrentWorkflowTaskExecutionSize = NewIntGlobalSetting(
		"worker.ParentCloseMaxConcurrentWorkflowTaskExecutionSize",
		1000,
		`WorkerParentCloseMaxConcurrentWorkflowTaskExecutionSize indicates worker parent close worker max concurrent workflow execution size`,
	)
	WorkerParentCloseMaxConcurrentActivityTaskPollers = NewIntGlobalSetting(
		"worker.ParentCloseMaxConcurrentActivityTaskPollers",
		4,
		`WorkerParentCloseMaxConcurrentActivityTaskPollers indicates worker parent close worker max concurrent activity pollers`,
	)
	WorkerParentCloseMaxConcurrentWorkflowTaskPollers = NewIntGlobalSetting(
		"worker.ParentCloseMaxConcurrentWorkflowTaskPollers",
		4,
		`WorkerParentCloseMaxConcurrentWorkflowTaskPollers indicates worker parent close worker max concurrent workflow pollers`,
	)
	WorkerPerNamespaceWorkerCount = NewIntNamespaceSetting(
		"worker.perNamespaceWorkerCount",
		1,
		`WorkerPerNamespaceWorkerCount controls number of per-ns (scheduler, batcher, etc.) workers to run per namespace`,
	)
	WorkerPerNamespaceWorkerOptions = NewMapNamespaceSetting(
		"worker.perNamespaceWorkerOptions",
		map[string]any{},
		`WorkerPerNamespaceWorkerOptions are SDK worker options for per-namespace worker`,
	)
	WorkerPerNamespaceWorkerStartRate = NewFloatGlobalSetting(
		"worker.perNamespaceWorkerStartRate",
		10.0,
		`WorkerPerNamespaceWorkerStartRate controls how fast per-namespace workers can be started (workers/second)`,
	)
	WorkerEnableScheduler = NewBoolNamespaceSetting(
		"worker.enableScheduler",
		true,
		`WorkerEnableScheduler controls whether to start the worker for scheduled workflows`,
	)
	WorkerStickyCacheSize = NewIntGlobalSetting(
		"worker.stickyCacheSize",
		0,
		`WorkerStickyCacheSize controls the sticky cache size for SDK workers on worker nodes
(shared between all workers in the process, cannot be changed after startup)`,
	)
	SchedulerNamespaceStartWorkflowRPS = NewFloatNamespaceSetting(
		"worker.schedulerNamespaceStartWorkflowRPS",
		30.0,
		`SchedulerNamespaceStartWorkflowRPS is the per-namespace limit for starting workflows by schedules`,
	)
	WorkerDeleteNamespaceActivityLimitsConfig = NewMapGlobalSetting(
		"worker.deleteNamespaceActivityLimitsConfig",
		map[string]any{},
		`WorkerDeleteNamespaceActivityLimitsConfig is a map that contains a copy of relevant sdkworker.Options
settings for controlling remote activity concurrency for delete namespace workflows.`,
	)
)

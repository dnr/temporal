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

package configs

import (
	"time"

	enumspb "go.temporal.io/api/enums/v1"

	"go.temporal.io/server/common"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/persistence/visibility"
)

// Config represents configuration for history service
type Config struct {
	NumberOfShards int32

	EnableReplicationStream dynamicconfig.BoolPropertyFn
	HistoryReplicationDLQV2 dynamicconfig.BoolPropertyFn

	RPS                                   dynamicconfig.IntPropertyFn
	OperatorRPSRatio                      dynamicconfig.FloatPropertyFn
	MaxIDLengthLimit                      dynamicconfig.IntPropertyFn
	PersistenceMaxQPS                     dynamicconfig.IntPropertyFn
	PersistenceGlobalMaxQPS               dynamicconfig.IntPropertyFn
	PersistenceNamespaceMaxQPS            dynamicconfig.IntPropertyFnWithNamespaceFilter
	PersistenceGlobalNamespaceMaxQPS      dynamicconfig.IntPropertyFnWithNamespaceFilter
	PersistencePerShardNamespaceMaxQPS    dynamicconfig.IntPropertyFnWithNamespaceFilter
	EnablePersistencePriorityRateLimiting dynamicconfig.BoolPropertyFn
	PersistenceDynamicRateLimitingParams  dynamicconfig.MapPropertyFn

	VisibilityPersistenceMaxReadQPS       dynamicconfig.IntPropertyFn
	VisibilityPersistenceMaxWriteQPS      dynamicconfig.IntPropertyFn
	EnableReadFromSecondaryVisibility     dynamicconfig.BoolPropertyFnWithNamespaceFilter
	SecondaryVisibilityWritingMode        dynamicconfig.StringPropertyFn
	VisibilityDisableOrderByClause        dynamicconfig.BoolPropertyFnWithNamespaceFilter
	VisibilityEnableManualPagination      dynamicconfig.BoolPropertyFnWithNamespaceFilter
	VisibilityAllowList                   dynamicconfig.BoolPropertyFnWithNamespaceFilter
	SuppressErrorSetSystemSearchAttribute dynamicconfig.BoolPropertyFnWithNamespaceFilter

	EmitShardLagLog            dynamicconfig.BoolPropertyFn
	MaxAutoResetPoints         dynamicconfig.IntPropertyFnWithNamespaceFilter
	ThrottledLogRPS            dynamicconfig.IntPropertyFn
	EnableStickyQuery          dynamicconfig.BoolPropertyFnWithNamespaceFilter
	ShutdownDrainDuration      dynamicconfig.DurationPropertyFn
	StartupMembershipJoinDelay dynamicconfig.DurationPropertyFn

	// HistoryCache settings
	// Change of these configs require shard restart
	HistoryCacheLimitSizeBased            bool
	HistoryCacheInitialSize               dynamicconfig.IntPropertyFn
	HistoryShardLevelCacheMaxSize         dynamicconfig.IntPropertyFn
	HistoryShardLevelCacheMaxSizeBytes    dynamicconfig.IntPropertyFn
	HistoryHostLevelCacheMaxSize          dynamicconfig.IntPropertyFn
	HistoryHostLevelCacheMaxSizeBytes     dynamicconfig.IntPropertyFn
	HistoryCacheTTL                       dynamicconfig.DurationPropertyFn
	HistoryCacheNonUserContextLockTimeout dynamicconfig.DurationPropertyFn
	EnableHostLevelHistoryCache           dynamicconfig.BoolPropertyFn
	EnableMutableStateTransitionHistory   dynamicconfig.BoolPropertyFn

	// EventsCache settings
	// Change of these configs require shard restart
	EventsShardLevelCacheMaxSizeBytes dynamicconfig.IntPropertyFn
	EventsCacheTTL                    dynamicconfig.DurationPropertyFn
	// Change of these configs require service restart
	EnableHostLevelEventsCache       dynamicconfig.BoolPropertyFn
	EventsHostLevelCacheMaxSizeBytes dynamicconfig.IntPropertyFn

	// ShardController settings
	RangeSizeBits                  uint
	AcquireShardInterval           dynamicconfig.DurationPropertyFn
	AcquireShardConcurrency        dynamicconfig.IntPropertyFn
	ShardIOConcurrency             dynamicconfig.IntPropertyFn
	ShardLingerOwnershipCheckQPS   dynamicconfig.IntPropertyFn
	ShardLingerTimeLimit           dynamicconfig.DurationPropertyFn
	ShardOwnershipAssertionEnabled dynamicconfig.BoolPropertyFn

	HistoryClientOwnershipCachingEnabled dynamicconfig.BoolPropertyFn

	// the artificial delay added to standby cluster's view of active cluster's time
	StandbyClusterDelay                  dynamicconfig.DurationPropertyFn
	StandbyTaskMissingEventsResendDelay  dynamicconfig.DurationPropertyFnWithTaskTypeFilter
	StandbyTaskMissingEventsDiscardDelay dynamicconfig.DurationPropertyFnWithTaskTypeFilter

	QueuePendingTaskCriticalCount    dynamicconfig.IntPropertyFn
	QueueReaderStuckCriticalAttempts dynamicconfig.IntPropertyFn
	QueueCriticalSlicesCount         dynamicconfig.IntPropertyFn
	QueuePendingTaskMaxCount         dynamicconfig.IntPropertyFn

	TaskDLQEnabled                 dynamicconfig.BoolPropertyFn
	TaskDLQUnexpectedErrorAttempts dynamicconfig.IntPropertyFn
	TaskDLQInternalErrors          dynamicconfig.BoolPropertyFn
	TaskDLQErrorPattern            dynamicconfig.StringPropertyFn

	TaskSchedulerEnableRateLimiter           dynamicconfig.BoolPropertyFn
	TaskSchedulerEnableRateLimiterShadowMode dynamicconfig.BoolPropertyFn
	TaskSchedulerRateLimiterStartupDelay     dynamicconfig.DurationPropertyFn
	TaskSchedulerGlobalMaxQPS                dynamicconfig.IntPropertyFn
	TaskSchedulerMaxQPS                      dynamicconfig.IntPropertyFn
	TaskSchedulerGlobalNamespaceMaxQPS       dynamicconfig.IntPropertyFnWithNamespaceFilter
	TaskSchedulerNamespaceMaxQPS             dynamicconfig.IntPropertyFnWithNamespaceFilter

	// TimerQueueProcessor settings
	TimerTaskBatchSize                               dynamicconfig.IntPropertyFn
	TimerProcessorSchedulerWorkerCount               dynamicconfig.IntPropertyFn
	TimerProcessorSchedulerActiveRoundRobinWeights   dynamicconfig.MapPropertyFnWithNamespaceFilter
	TimerProcessorSchedulerStandbyRoundRobinWeights  dynamicconfig.MapPropertyFnWithNamespaceFilter
	TimerProcessorUpdateAckInterval                  dynamicconfig.DurationPropertyFn
	TimerProcessorUpdateAckIntervalJitterCoefficient dynamicconfig.FloatPropertyFn
	TimerProcessorMaxPollRPS                         dynamicconfig.IntPropertyFn
	TimerProcessorMaxPollHostRPS                     dynamicconfig.IntPropertyFn
	TimerProcessorMaxPollInterval                    dynamicconfig.DurationPropertyFn
	TimerProcessorMaxPollIntervalJitterCoefficient   dynamicconfig.FloatPropertyFn
	TimerProcessorPollBackoffInterval                dynamicconfig.DurationPropertyFn
	TimerProcessorMaxTimeShift                       dynamicconfig.DurationPropertyFn
	TimerQueueMaxReaderCount                         dynamicconfig.IntPropertyFn
	RetentionTimerJitterDuration                     dynamicconfig.DurationPropertyFn

	MemoryTimerProcessorSchedulerWorkerCount dynamicconfig.IntPropertyFn

	// TransferQueueProcessor settings
	TransferTaskBatchSize                               dynamicconfig.IntPropertyFn
	TransferProcessorSchedulerWorkerCount               dynamicconfig.IntPropertyFn
	TransferProcessorSchedulerActiveRoundRobinWeights   dynamicconfig.MapPropertyFnWithNamespaceFilter
	TransferProcessorSchedulerStandbyRoundRobinWeights  dynamicconfig.MapPropertyFnWithNamespaceFilter
	TransferProcessorMaxPollRPS                         dynamicconfig.IntPropertyFn
	TransferProcessorMaxPollHostRPS                     dynamicconfig.IntPropertyFn
	TransferProcessorMaxPollInterval                    dynamicconfig.DurationPropertyFn
	TransferProcessorMaxPollIntervalJitterCoefficient   dynamicconfig.FloatPropertyFn
	TransferProcessorUpdateAckInterval                  dynamicconfig.DurationPropertyFn
	TransferProcessorUpdateAckIntervalJitterCoefficient dynamicconfig.FloatPropertyFn
	TransferProcessorPollBackoffInterval                dynamicconfig.DurationPropertyFn
	TransferProcessorEnsureCloseBeforeDelete            dynamicconfig.BoolPropertyFn
	TransferQueueMaxReaderCount                         dynamicconfig.IntPropertyFn

	// OutboundQueueProcessor settings
	OutboundTaskBatchSize                               dynamicconfig.IntPropertyFn
	OutboundProcessorMaxPollRPS                         dynamicconfig.IntPropertyFn
	OutboundProcessorMaxPollHostRPS                     dynamicconfig.IntPropertyFn
	OutboundProcessorMaxPollInterval                    dynamicconfig.DurationPropertyFn
	OutboundProcessorMaxPollIntervalJitterCoefficient   dynamicconfig.FloatPropertyFn
	OutboundProcessorUpdateAckInterval                  dynamicconfig.DurationPropertyFn
	OutboundProcessorUpdateAckIntervalJitterCoefficient dynamicconfig.FloatPropertyFn
	OutboundProcessorPollBackoffInterval                dynamicconfig.DurationPropertyFn
	OutboundQueueMaxReaderCount                         dynamicconfig.IntPropertyFn

	// ReplicatorQueueProcessor settings
	ReplicatorProcessorMaxPollInterval                  dynamicconfig.DurationPropertyFn
	ReplicatorProcessorMaxPollIntervalJitterCoefficient dynamicconfig.FloatPropertyFn
	ReplicatorProcessorFetchTasksBatchSize              dynamicconfig.IntPropertyFn
	ReplicatorProcessorMaxSkipTaskCount                 dynamicconfig.IntPropertyFn

	// System Limits
	MaximumBufferedEventsBatch       dynamicconfig.IntPropertyFn
	MaximumBufferedEventsSizeInBytes dynamicconfig.IntPropertyFn
	MaximumSignalsPerExecution       dynamicconfig.IntPropertyFnWithNamespaceFilter

	// ShardUpdateMinInterval is the minimum time interval within which the shard info can be updated.
	ShardUpdateMinInterval dynamicconfig.DurationPropertyFn
	// ShardUpdateMinTasksCompleted is the minimum number of tasks which must be completed before the shard info can be updated before
	// history.shardUpdateMinInterval has passed
	ShardUpdateMinTasksCompleted dynamicconfig.IntPropertyFn
	// ShardSyncMinInterval is the minimum time interval within which the shard info can be synced to the remote.
	ShardSyncMinInterval            dynamicconfig.DurationPropertyFn
	ShardSyncTimerJitterCoefficient dynamicconfig.FloatPropertyFn

	// Time to hold a poll request before returning an empty response
	// right now only used by GetMutableState
	LongPollExpirationInterval dynamicconfig.DurationPropertyFnWithNamespaceFilter

	// encoding the history events
	EventEncodingType dynamicconfig.StringPropertyFnWithNamespaceFilter
	// whether or not using ParentClosePolicy
	EnableParentClosePolicy dynamicconfig.BoolPropertyFnWithNamespaceFilter
	// whether or not enable system workers for processing parent close policy task
	EnableParentClosePolicyWorker dynamicconfig.BoolPropertyFn
	// parent close policy will be processed by sys workers(if enabled) if
	// the number of children greater than or equal to this threshold
	ParentClosePolicyThreshold dynamicconfig.IntPropertyFnWithNamespaceFilter
	// total number of parentClosePolicy system workflows
	NumParentClosePolicySystemWorkflows dynamicconfig.IntPropertyFn

	// Size limit related settings
	BlobSizeLimitError                        dynamicconfig.IntPropertyFnWithNamespaceFilter
	BlobSizeLimitWarn                         dynamicconfig.IntPropertyFnWithNamespaceFilter
	MemoSizeLimitError                        dynamicconfig.IntPropertyFnWithNamespaceFilter
	MemoSizeLimitWarn                         dynamicconfig.IntPropertyFnWithNamespaceFilter
	HistorySizeLimitError                     dynamicconfig.IntPropertyFnWithNamespaceFilter
	HistorySizeLimitWarn                      dynamicconfig.IntPropertyFnWithNamespaceFilter
	HistorySizeSuggestContinueAsNew           dynamicconfig.IntPropertyFnWithNamespaceFilter
	HistoryCountLimitError                    dynamicconfig.IntPropertyFnWithNamespaceFilter
	HistoryCountLimitWarn                     dynamicconfig.IntPropertyFnWithNamespaceFilter
	HistoryCountSuggestContinueAsNew          dynamicconfig.IntPropertyFnWithNamespaceFilter
	HistoryMaxPageSize                        dynamicconfig.IntPropertyFnWithNamespaceFilter
	MutableStateActivityFailureSizeLimitError dynamicconfig.IntPropertyFnWithNamespaceFilter
	MutableStateActivityFailureSizeLimitWarn  dynamicconfig.IntPropertyFnWithNamespaceFilter
	MutableStateSizeLimitError                dynamicconfig.IntPropertyFn
	MutableStateSizeLimitWarn                 dynamicconfig.IntPropertyFn
	NumPendingChildExecutionsLimit            dynamicconfig.IntPropertyFnWithNamespaceFilter
	NumPendingActivitiesLimit                 dynamicconfig.IntPropertyFnWithNamespaceFilter
	NumPendingSignalsLimit                    dynamicconfig.IntPropertyFnWithNamespaceFilter
	NumPendingCancelsRequestLimit             dynamicconfig.IntPropertyFnWithNamespaceFilter

	// DefaultActivityRetryOptions specifies the out-of-box retry policy if
	// none is configured on the Activity by the user.
	DefaultActivityRetryPolicy dynamicconfig.MapPropertyFnWithNamespaceFilter

	// DefaultWorkflowRetryPolicy specifies the out-of-box retry policy for
	// any unset fields on a RetryPolicy configured on a Workflow
	DefaultWorkflowRetryPolicy dynamicconfig.MapPropertyFnWithNamespaceFilter

	// Workflow task settings
	// DefaultWorkflowTaskTimeout the default workflow task timeout
	DefaultWorkflowTaskTimeout dynamicconfig.DurationPropertyFnWithNamespaceFilter
	// WorkflowTaskHeartbeatTimeout is to timeout behavior of: RespondWorkflowTaskComplete with ForceCreateNewWorkflowTask == true
	// without any commands or messages. After this timeout workflow task will be scheduled to another worker(by clear stickyness).
	WorkflowTaskHeartbeatTimeout dynamicconfig.DurationPropertyFnWithNamespaceFilter
	WorkflowTaskCriticalAttempts dynamicconfig.IntPropertyFn
	WorkflowTaskRetryMaxInterval dynamicconfig.DurationPropertyFn

	// ContinueAsNewMinInterval is the minimal interval between continue_as_new to prevent tight continue_as_new loop.
	ContinueAsNewMinInterval dynamicconfig.DurationPropertyFnWithNamespaceFilter

	// The following is used by the new RPC replication stack
	ReplicationTaskFetcherParallelism                    dynamicconfig.IntPropertyFn
	ReplicationTaskFetcherAggregationInterval            dynamicconfig.DurationPropertyFn
	ReplicationTaskFetcherTimerJitterCoefficient         dynamicconfig.FloatPropertyFn
	ReplicationTaskFetcherErrorRetryWait                 dynamicconfig.DurationPropertyFn
	ReplicationTaskProcessorErrorRetryWait               dynamicconfig.DurationPropertyFnWithShardIDFilter
	ReplicationTaskProcessorErrorRetryBackoffCoefficient dynamicconfig.FloatPropertyFnWithShardIDFilter
	ReplicationTaskProcessorErrorRetryMaxInterval        dynamicconfig.DurationPropertyFnWithShardIDFilter
	ReplicationTaskProcessorErrorRetryMaxAttempts        dynamicconfig.IntPropertyFnWithShardIDFilter
	ReplicationTaskProcessorErrorRetryExpiration         dynamicconfig.DurationPropertyFnWithShardIDFilter
	ReplicationTaskProcessorNoTaskRetryWait              dynamicconfig.DurationPropertyFnWithShardIDFilter
	ReplicationTaskProcessorCleanupInterval              dynamicconfig.DurationPropertyFnWithShardIDFilter
	ReplicationTaskProcessorCleanupJitterCoefficient     dynamicconfig.FloatPropertyFnWithShardIDFilter
	ReplicationTaskProcessorHostQPS                      dynamicconfig.FloatPropertyFn
	ReplicationTaskProcessorShardQPS                     dynamicconfig.FloatPropertyFn
	ReplicationEnableDLQMetrics                          dynamicconfig.BoolPropertyFn
	ReplicationEnableUpdateWithNewTaskMerge              dynamicconfig.BoolPropertyFn

	ReplicationStreamSyncStatusDuration      dynamicconfig.DurationPropertyFn
	ReplicationProcessorSchedulerQueueSize   dynamicconfig.IntPropertyFn
	ReplicationProcessorSchedulerWorkerCount dynamicconfig.IntPropertyFn
	EnableReplicationEagerRefreshNamespace   dynamicconfig.BoolPropertyFn
	EnableReplicationTaskBatching            dynamicconfig.BoolPropertyFn
	EnableReplicateLocalGeneratedEvent       dynamicconfig.BoolPropertyFn

	// The following are used by consistent query
	MaxBufferedQueryCount dynamicconfig.IntPropertyFn

	// Data integrity check related config knobs
	MutableStateChecksumGenProbability    dynamicconfig.IntPropertyFnWithNamespaceFilter
	MutableStateChecksumVerifyProbability dynamicconfig.IntPropertyFnWithNamespaceFilter
	MutableStateChecksumInvalidateBefore  dynamicconfig.FloatPropertyFn

	// NDC Replication configuration
	StandbyTaskReReplicationContextTimeout dynamicconfig.DurationPropertyFnWithNamespaceIDFilter

	SkipReapplicationByNamespaceID dynamicconfig.BoolPropertyFnWithNamespaceIDFilter

	// ===== Visibility related =====
	// VisibilityQueueProcessor settings
	VisibilityTaskBatchSize                               dynamicconfig.IntPropertyFn
	VisibilityProcessorSchedulerWorkerCount               dynamicconfig.IntPropertyFn
	VisibilityProcessorSchedulerActiveRoundRobinWeights   dynamicconfig.MapPropertyFnWithNamespaceFilter
	VisibilityProcessorSchedulerStandbyRoundRobinWeights  dynamicconfig.MapPropertyFnWithNamespaceFilter
	VisibilityProcessorMaxPollRPS                         dynamicconfig.IntPropertyFn
	VisibilityProcessorMaxPollHostRPS                     dynamicconfig.IntPropertyFn
	VisibilityProcessorMaxPollInterval                    dynamicconfig.DurationPropertyFn
	VisibilityProcessorMaxPollIntervalJitterCoefficient   dynamicconfig.FloatPropertyFn
	VisibilityProcessorUpdateAckInterval                  dynamicconfig.DurationPropertyFn
	VisibilityProcessorUpdateAckIntervalJitterCoefficient dynamicconfig.FloatPropertyFn
	VisibilityProcessorPollBackoffInterval                dynamicconfig.DurationPropertyFn
	VisibilityProcessorEnsureCloseBeforeDelete            dynamicconfig.BoolPropertyFn
	VisibilityProcessorEnableCloseWorkflowCleanup         dynamicconfig.BoolPropertyFnWithNamespaceFilter
	VisibilityQueueMaxReaderCount                         dynamicconfig.IntPropertyFn

	SearchAttributesNumberOfKeysLimit dynamicconfig.IntPropertyFnWithNamespaceFilter
	SearchAttributesSizeOfValueLimit  dynamicconfig.IntPropertyFnWithNamespaceFilter
	SearchAttributesTotalSizeLimit    dynamicconfig.IntPropertyFnWithNamespaceFilter
	IndexerConcurrency                dynamicconfig.IntPropertyFn
	ESProcessorNumOfWorkers           dynamicconfig.IntPropertyFn
	ESProcessorBulkActions            dynamicconfig.IntPropertyFn // max number of requests in bulk
	ESProcessorBulkSize               dynamicconfig.IntPropertyFn // max total size of bytes in bulk
	ESProcessorFlushInterval          dynamicconfig.DurationPropertyFn
	ESProcessorAckTimeout             dynamicconfig.DurationPropertyFn

	EnableCrossNamespaceCommands  dynamicconfig.BoolPropertyFn
	EnableActivityEagerExecution  dynamicconfig.BoolPropertyFnWithNamespaceFilter
	EnableEagerWorkflowStart      dynamicconfig.BoolPropertyFnWithNamespaceFilter
	NamespaceCacheRefreshInterval dynamicconfig.DurationPropertyFn

	// ArchivalQueueProcessor settings
	ArchivalProcessorSchedulerWorkerCount               dynamicconfig.IntPropertyFn
	ArchivalProcessorMaxPollHostRPS                     dynamicconfig.IntPropertyFn
	ArchivalTaskBatchSize                               dynamicconfig.IntPropertyFn
	ArchivalProcessorPollBackoffInterval                dynamicconfig.DurationPropertyFn
	ArchivalProcessorMaxPollRPS                         dynamicconfig.IntPropertyFn
	ArchivalProcessorMaxPollInterval                    dynamicconfig.DurationPropertyFn
	ArchivalProcessorMaxPollIntervalJitterCoefficient   dynamicconfig.FloatPropertyFn
	ArchivalProcessorUpdateAckInterval                  dynamicconfig.DurationPropertyFn
	ArchivalProcessorUpdateAckIntervalJitterCoefficient dynamicconfig.FloatPropertyFn
	ArchivalProcessorArchiveDelay                       dynamicconfig.DurationPropertyFn
	ArchivalBackendMaxRPS                               dynamicconfig.FloatPropertyFn
	ArchivalQueueMaxReaderCount                         dynamicconfig.IntPropertyFn

	WorkflowExecutionMaxInFlightUpdates dynamicconfig.IntPropertyFnWithNamespaceFilter
	WorkflowExecutionMaxTotalUpdates    dynamicconfig.IntPropertyFnWithNamespaceFilter

	SendRawWorkflowHistory dynamicconfig.BoolPropertyFnWithNamespaceFilter

	// FrontendAccessHistoryFraction is an interim flag across 2 minor releases and will be removed once fully enabled.
	FrontendAccessHistoryFraction dynamicconfig.FloatPropertyFn
}

// NewConfig returns new service config with default values
func NewConfig(
	dc *dynamicconfig.Collection,
	numberOfShards int32,
) *Config {
	cfg := &Config{
		NumberOfShards: numberOfShards,

		EnableReplicationStream: dc.GetBool(dynamicconfig.EnableReplicationStream),
		HistoryReplicationDLQV2: dc.GetBoolProperty(dynamicconfig.EnableHistoryReplicationDLQV2, false),

		RPS:                                   dc.GetInt(dynamicconfig.HistoryRPS),
		OperatorRPSRatio:                      dc.GetFloat64(dynamicconfig.OperatorRPSRatio),
		MaxIDLengthLimit:                      dc.GetInt(dynamicconfig.MaxIDLengthLimit),
		PersistenceMaxQPS:                     dc.GetInt(dynamicconfig.HistoryPersistenceMaxQPS),
		PersistenceGlobalMaxQPS:               dc.GetInt(dynamicconfig.HistoryPersistenceGlobalMaxQPS),
		PersistenceNamespaceMaxQPS:            dc.GetIntByNamespace(dynamicconfig.HistoryPersistenceNamespaceMaxQPS),
		PersistenceGlobalNamespaceMaxQPS:      dc.GetIntByNamespace(dynamicconfig.HistoryPersistenceGlobalNamespaceMaxQPS),
		PersistencePerShardNamespaceMaxQPS:    dc.GetIntByNamespace(dynamicconfig.HistoryPersistencePerShardNamespaceMaxQPS),
		EnablePersistencePriorityRateLimiting: dc.GetBool(dynamicconfig.HistoryEnablePersistencePriorityRateLimiting),
		PersistenceDynamicRateLimitingParams:  dc.GetMap(dynamicconfig.HistoryPersistenceDynamicRateLimitingParams),
		ShutdownDrainDuration:                 dc.GetDuration(dynamicconfig.HistoryShutdownDrainDuration),
		StartupMembershipJoinDelay:            dc.GetDuration(dynamicconfig.HistoryStartupMembershipJoinDelay),
		MaxAutoResetPoints:                    dc.GetIntByNamespace(dynamicconfig.HistoryMaxAutoResetPoints),
		DefaultWorkflowTaskTimeout:            dc.GetDurationByNamespace(dynamicconfig.DefaultWorkflowTaskTimeout),
		ContinueAsNewMinInterval:              dc.GetDurationByNamespace(dynamicconfig.ContinueAsNewMinInterval),

		VisibilityPersistenceMaxReadQPS:       visibility.GetVisibilityPersistenceMaxReadQPS(dc),
		VisibilityPersistenceMaxWriteQPS:      visibility.GetVisibilityPersistenceMaxWriteQPS(dc),
		EnableReadFromSecondaryVisibility:     visibility.GetEnableReadFromSecondaryVisibilityConfig(dc),
		SecondaryVisibilityWritingMode:        visibility.GetSecondaryVisibilityWritingModeConfig(dc),
		VisibilityDisableOrderByClause:        dc.GetBoolByNamespace(dynamicconfig.VisibilityDisableOrderByClause),
		VisibilityEnableManualPagination:      dc.GetBoolByNamespace(dynamicconfig.VisibilityEnableManualPagination),
		VisibilityAllowList:                   dc.GetBoolByNamespace(dynamicconfig.VisibilityAllowList),
		SuppressErrorSetSystemSearchAttribute: dc.GetBoolByNamespace(dynamicconfig.SuppressErrorSetSystemSearchAttribute),

		EmitShardLagLog: dc.GetBool(dynamicconfig.EmitShardLagLog),
		// HistoryCacheLimitSizeBased should not change during runtime.
		HistoryCacheLimitSizeBased:            dc.GetBool(dynamicconfig.HistoryCacheSizeBasedLimit)(),
		HistoryCacheInitialSize:               dc.GetInt(dynamicconfig.HistoryCacheInitialSize),
		HistoryShardLevelCacheMaxSize:         dc.GetInt(dynamicconfig.HistoryCacheMaxSize),
		HistoryShardLevelCacheMaxSizeBytes:    dc.GetInt(dynamicconfig.HistoryCacheMaxSizeBytes),
		HistoryHostLevelCacheMaxSize:          dc.GetInt(dynamicconfig.HistoryCacheHostLevelMaxSize),
		HistoryHostLevelCacheMaxSizeBytes:     dc.GetInt(dynamicconfig.HistoryCacheHostLevelMaxSizeBytes),
		HistoryCacheTTL:                       dc.GetDuration(dynamicconfig.HistoryCacheTTL),
		HistoryCacheNonUserContextLockTimeout: dc.GetDuration(dynamicconfig.HistoryCacheNonUserContextLockTimeout),
		EnableHostLevelHistoryCache:           dc.GetBool(dynamicconfig.EnableHostHistoryCache),
		EnableMutableStateTransitionHistory:   dc.GetBool(dynamicconfig.EnableMutableStateTransitionHistory),

		EventsShardLevelCacheMaxSizeBytes: dc.GetInt(dynamicconfig.EventsCacheMaxSizeBytes),              // 512KB
		EventsHostLevelCacheMaxSizeBytes:  dc.GetInt(dynamicconfig.EventsHostLevelCacheMaxSizeBytes), // 256MB
		EventsCacheTTL:                    dc.GetDuration(dynamicconfig.EventsCacheTTL),
		EnableHostLevelEventsCache:        dc.GetBool(dynamicconfig.EnableHostLevelEventsCache),

		RangeSizeBits:                  20, // 20 bits for sequencer, 2^20 sequence number for any range
		AcquireShardInterval:           dc.GetDuration(dynamicconfig.AcquireShardInterval),
		AcquireShardConcurrency:        dc.GetInt(dynamicconfig.AcquireShardConcurrency),
		ShardIOConcurrency:             dc.GetInt(dynamicconfig.ShardIOConcurrency),
		ShardLingerOwnershipCheckQPS:   dc.GetInt(dynamicconfig.ShardLingerOwnershipCheckQPS),
		ShardLingerTimeLimit:           dc.GetDuration(dynamicconfig.ShardLingerTimeLimit),
		ShardOwnershipAssertionEnabled: dc.GetBool(dynamicconfig.ShardOwnershipAssertionEnabled),

		HistoryClientOwnershipCachingEnabled: dc.GetBool(dynamicconfig.HistoryClientOwnershipCachingEnabled),

		StandbyClusterDelay:                  dc.GetDuration(dynamicconfig.StandbyClusterDelay),
		StandbyTaskMissingEventsResendDelay:  dc.GetDurationPropertyFilteredByTaskType(dynamicconfig.StandbyTaskMissingEventsResendDelay, 10*time.Minute),
		StandbyTaskMissingEventsDiscardDelay: dc.GetDurationPropertyFilteredByTaskType(dynamicconfig.StandbyTaskMissingEventsDiscardDelay, 15*time.Minute),

		QueuePendingTaskCriticalCount:    dc.GetInt(dynamicconfig.QueuePendingTaskCriticalCount),
		QueueReaderStuckCriticalAttempts: dc.GetInt(dynamicconfig.QueueReaderStuckCriticalAttempts),
		QueueCriticalSlicesCount:         dc.GetInt(dynamicconfig.QueueCriticalSlicesCount),
		QueuePendingTaskMaxCount:         dc.GetInt(dynamicconfig.QueuePendingTaskMaxCount),

		TaskDLQEnabled:                 dc.GetBool(dynamicconfig.HistoryTaskDLQEnabled),
		TaskDLQUnexpectedErrorAttempts: dc.GetInt(dynamicconfig.HistoryTaskDLQUnexpectedErrorAttempts),
		TaskDLQInternalErrors:          dc.GetBool(dynamicconfig.HistoryTaskDLQInternalErrors),
		TaskDLQErrorPattern:            dc.GetString(dynamicconfig.HistoryTaskDLQErrorPattern),

		TaskSchedulerEnableRateLimiter:           dc.GetBool(dynamicconfig.TaskSchedulerEnableRateLimiter),
		TaskSchedulerEnableRateLimiterShadowMode: dc.GetBool(dynamicconfig.TaskSchedulerEnableRateLimiterShadowMode),
		TaskSchedulerRateLimiterStartupDelay:     dc.GetDuration(dynamicconfig.TaskSchedulerRateLimiterStartupDelay),
		TaskSchedulerGlobalMaxQPS:                dc.GetInt(dynamicconfig.TaskSchedulerGlobalMaxQPS),
		TaskSchedulerMaxQPS:                      dc.GetInt(dynamicconfig.TaskSchedulerMaxQPS),
		TaskSchedulerNamespaceMaxQPS:             dc.GetIntByNamespace(dynamicconfig.TaskSchedulerNamespaceMaxQPS),
		TaskSchedulerGlobalNamespaceMaxQPS:       dc.GetIntByNamespace(dynamicconfig.TaskSchedulerGlobalNamespaceMaxQPS),

		TimerTaskBatchSize:                               dc.GetInt(dynamicconfig.TimerTaskBatchSize),
		TimerProcessorSchedulerWorkerCount:               dc.GetInt(dynamicconfig.TimerProcessorSchedulerWorkerCount),
		TimerProcessorSchedulerActiveRoundRobinWeights:   dc.GetMapPropertyFnFilteredByNamespace(dynamicconfig.WithDefault(dynamicconfig.TimerProcessorSchedulerActiveRoundRobinWeights, ConvertWeightsToDynamicConfigValue(DefaultActiveTaskPriorityWeight))),
		TimerProcessorSchedulerStandbyRoundRobinWeights:  dc.GetMapPropertyFnFilteredByNamespace(dynamicconfig.WithDefault(dynamicconfig.TimerProcessorSchedulerStandbyRoundRobinWeights, ConvertWeightsToDynamicConfigValue(DefaultStandbyTaskPriorityWeight))),
		TimerProcessorUpdateAckInterval:                  dc.GetDuration(dynamicconfig.TimerProcessorUpdateAckInterval),
		TimerProcessorUpdateAckIntervalJitterCoefficient: dc.GetFloat64(dynamicconfig.TimerProcessorUpdateAckIntervalJitterCoefficient),
		TimerProcessorMaxPollRPS:                         dc.GetInt(dynamicconfig.TimerProcessorMaxPollRPS),
		TimerProcessorMaxPollHostRPS:                     dc.GetInt(dynamicconfig.TimerProcessorMaxPollHostRPS),
		TimerProcessorMaxPollInterval:                    dc.GetDuration(dynamicconfig.TimerProcessorMaxPollInterval),
		TimerProcessorMaxPollIntervalJitterCoefficient:   dc.GetFloat64(dynamicconfig.TimerProcessorMaxPollIntervalJitterCoefficient),
		TimerProcessorPollBackoffInterval:                dc.GetDuration(dynamicconfig.TimerProcessorPollBackoffInterval),
		TimerProcessorMaxTimeShift:                       dc.GetDuration(dynamicconfig.TimerProcessorMaxTimeShift),
		TransferQueueMaxReaderCount:                      dc.GetInt(dynamicconfig.TransferQueueMaxReaderCount),
		RetentionTimerJitterDuration:                     dc.GetDuration(dynamicconfig.RetentionTimerJitterDuration),

		MemoryTimerProcessorSchedulerWorkerCount: dc.GetInt(dynamicconfig.MemoryTimerProcessorSchedulerWorkerCount),

		TransferTaskBatchSize:                               dc.GetInt(dynamicconfig.TransferTaskBatchSize),
		TransferProcessorSchedulerWorkerCount:               dc.GetInt(dynamicconfig.TransferProcessorSchedulerWorkerCount),
		TransferProcessorSchedulerActiveRoundRobinWeights:   dc.GetMapPropertyFnFilteredByNamespace(dynamicconfig.WithDefault(dynamicconfig.TransferProcessorSchedulerActiveRoundRobinWeights, ConvertWeightsToDynamicConfigValue(DefaultActiveTaskPriorityWeight))),
		TransferProcessorSchedulerStandbyRoundRobinWeights:  dc.GetMapPropertyFnFilteredByNamespace(dynamicconfig.WithDefault(dynamicconfig.TransferProcessorSchedulerStandbyRoundRobinWeights, ConvertWeightsToDynamicConfigValue(DefaultStandbyTaskPriorityWeight))),
		TransferProcessorMaxPollRPS:                         dc.GetInt(dynamicconfig.TransferProcessorMaxPollRPS),
		TransferProcessorMaxPollHostRPS:                     dc.GetInt(dynamicconfig.TransferProcessorMaxPollHostRPS),
		TransferProcessorMaxPollInterval:                    dc.GetDuration(dynamicconfig.TransferProcessorMaxPollInterval),
		TransferProcessorMaxPollIntervalJitterCoefficient:   dc.GetFloat64(dynamicconfig.TransferProcessorMaxPollIntervalJitterCoefficient),
		TransferProcessorUpdateAckInterval:                  dc.GetDuration(dynamicconfig.TransferProcessorUpdateAckInterval),
		TransferProcessorUpdateAckIntervalJitterCoefficient: dc.GetFloat64(dynamicconfig.TransferProcessorUpdateAckIntervalJitterCoefficient),
		TransferProcessorPollBackoffInterval:                dc.GetDuration(dynamicconfig.TransferProcessorPollBackoffInterval),
		TransferProcessorEnsureCloseBeforeDelete:            dc.GetBool(dynamicconfig.TransferProcessorEnsureCloseBeforeDelete),
		TimerQueueMaxReaderCount:                            dc.GetInt(dynamicconfig.TimerQueueMaxReaderCount),

		OutboundTaskBatchSize:                               dc.GetInt(dynamicconfig.OutboundTaskBatchSize),
		OutboundProcessorMaxPollRPS:                         dc.GetInt(dynamicconfig.OutboundProcessorMaxPollRPS),
		OutboundProcessorMaxPollHostRPS:                     dc.GetInt(dynamicconfig.OutboundProcessorMaxPollHostRPS),
		OutboundProcessorMaxPollInterval:                    dc.GetDuration(dynamicconfig.OutboundProcessorMaxPollInterval),
		OutboundProcessorMaxPollIntervalJitterCoefficient:   dc.GetFloat64(dynamicconfig.OutboundProcessorMaxPollIntervalJitterCoefficient),
		OutboundProcessorUpdateAckInterval:                  dc.GetDuration(dynamicconfig.OutboundProcessorUpdateAckInterval),
		OutboundProcessorUpdateAckIntervalJitterCoefficient: dc.GetFloat64(dynamicconfig.OutboundProcessorUpdateAckIntervalJitterCoefficient),
		OutboundProcessorPollBackoffInterval:                dc.GetDuration(dynamicconfig.OutboundProcessorPollBackoffInterval),
		OutboundQueueMaxReaderCount:                         dc.GetInt(dynamicconfig.OutboundQueueMaxReaderCount),

		ReplicatorProcessorMaxPollInterval:                  dc.GetDuration(dynamicconfig.ReplicatorProcessorMaxPollInterval),
		ReplicatorProcessorMaxPollIntervalJitterCoefficient: dc.GetFloat64(dynamicconfig.ReplicatorProcessorMaxPollIntervalJitterCoefficient),
		ReplicatorProcessorFetchTasksBatchSize:              dc.GetInt(dynamicconfig.ReplicatorTaskBatchSize),
		ReplicatorProcessorMaxSkipTaskCount:                 dc.GetInt(dynamicconfig.ReplicatorMaxSkipTaskCount),
		ReplicationTaskProcessorHostQPS:                     dc.GetFloat64(dynamicconfig.ReplicationTaskProcessorHostQPS),
		ReplicationTaskProcessorShardQPS:                    dc.GetFloat64(dynamicconfig.ReplicationTaskProcessorShardQPS),
		ReplicationEnableDLQMetrics:                         dc.GetBool(dynamicconfig.ReplicationEnableDLQMetrics),
		ReplicationEnableUpdateWithNewTaskMerge:             dc.GetBool(dynamicconfig.ReplicationEnableUpdateWithNewTaskMerge),
		ReplicationStreamSyncStatusDuration:                 dc.GetDuration(dynamicconfig.ReplicationStreamSyncStatusDuration),
		ReplicationProcessorSchedulerQueueSize:              dc.GetInt(dynamicconfig.ReplicationProcessorSchedulerQueueSize),
		ReplicationProcessorSchedulerWorkerCount:            dc.GetInt(dynamicconfig.ReplicationProcessorSchedulerWorkerCount),
		EnableReplicationEagerRefreshNamespace:              dc.GetBool(dynamicconfig.EnableEagerNamespaceRefresher),
		EnableReplicationTaskBatching:                       dc.GetBool(dynamicconfig.EnableReplicationTaskBatching),
		EnableReplicateLocalGeneratedEvent:                  dc.GetBool(dynamicconfig.EnableReplicateLocalGeneratedEvents),

		MaximumBufferedEventsBatch:       dc.GetInt(dynamicconfig.MaximumBufferedEventsBatch),
		MaximumBufferedEventsSizeInBytes: dc.GetInt(dynamicconfig.MaximumBufferedEventsSizeInBytes),
		MaximumSignalsPerExecution:       dc.GetIntByNamespace(dynamicconfig.MaximumSignalsPerExecution),
		ShardUpdateMinInterval:           dc.GetDuration(dynamicconfig.ShardUpdateMinInterval),
		ShardUpdateMinTasksCompleted:     dc.GetInt(dynamicconfig.ShardUpdateMinTasksCompleted),
		ShardSyncMinInterval:             dc.GetDuration(dynamicconfig.ShardSyncMinInterval),
		ShardSyncTimerJitterCoefficient:  dc.GetFloat64(dynamicconfig.TransferProcessorMaxPollIntervalJitterCoefficient),

		// history client: client/history/client.go set the client timeout 30s
		// TODO: Return this value to the client: go.temporal.io/server/issues/294
		LongPollExpirationInterval:          dc.GetDurationByNamespace(dynamicconfig.HistoryLongPollExpirationInterval),
		EventEncodingType:                   dc.GetStringByNamespace(dynamicconfig.DefaultEventEncoding)),
		EnableParentClosePolicy:             dc.GetBoolByNamespace(dynamicconfig.EnableParentClosePolicy),
		NumParentClosePolicySystemWorkflows: dc.GetInt(dynamicconfig.NumParentClosePolicySystemWorkflows),
		EnableParentClosePolicyWorker:       dc.GetBool(dynamicconfig.EnableParentClosePolicyWorker),
		ParentClosePolicyThreshold:          dc.GetIntByNamespace(dynamicconfig.ParentClosePolicyThreshold),

		BlobSizeLimitError:                        dc.GetIntByNamespace(dynamicconfig.BlobSizeLimitError),
		BlobSizeLimitWarn:                         dc.GetIntByNamespace(dynamicconfig.BlobSizeLimitWarn),
		MemoSizeLimitError:                        dc.GetIntByNamespace(dynamicconfig.MemoSizeLimitError),
		MemoSizeLimitWarn:                         dc.GetIntByNamespace(dynamicconfig.MemoSizeLimitWarn),
		NumPendingChildExecutionsLimit:            dc.GetIntByNamespace(dynamicconfig.NumPendingChildExecutionsLimitError),
		NumPendingActivitiesLimit:                 dc.GetIntByNamespace(dynamicconfig.NumPendingActivitiesLimitError),
		NumPendingSignalsLimit:                    dc.GetIntByNamespace(dynamicconfig.NumPendingSignalsLimitError),
		NumPendingCancelsRequestLimit:             dc.GetIntByNamespace(dynamicconfig.NumPendingCancelRequestsLimitError),
		HistorySizeLimitError:                     dc.GetIntByNamespace(dynamicconfig.HistorySizeLimitError),
		HistorySizeLimitWarn:                      dc.GetIntByNamespace(dynamicconfig.HistorySizeLimitWarn),
		HistorySizeSuggestContinueAsNew:           dc.GetIntByNamespace(dynamicconfig.HistorySizeSuggestContinueAsNew),
		HistoryCountLimitError:                    dc.GetIntByNamespace(dynamicconfig.HistoryCountLimitError),
		HistoryCountLimitWarn:                     dc.GetIntByNamespace(dynamicconfig.HistoryCountLimitWarn),
		HistoryCountSuggestContinueAsNew:          dc.GetIntByNamespace(dynamicconfig.HistoryCountSuggestContinueAsNew),
		HistoryMaxPageSize:                        dc.GetIntByNamespace(dynamicconfig.HistoryMaxPageSize),
		MutableStateActivityFailureSizeLimitError: dc.GetIntByNamespace(dynamicconfig.MutableStateActivityFailureSizeLimitError),
		MutableStateActivityFailureSizeLimitWarn:  dc.GetIntByNamespace(dynamicconfig.MutableStateActivityFailureSizeLimitWarn),
		MutableStateSizeLimitError:                dc.GetInt(dynamicconfig.MutableStateSizeLimitError),
		MutableStateSizeLimitWarn:                 dc.GetInt(dynamicconfig.MutableStateSizeLimitWarn),

		ThrottledLogRPS:   dc.GetInt(dynamicconfig.HistoryThrottledLogRPS),
		EnableStickyQuery: dc.GetBoolByNamespace(dynamicconfig.EnableStickyQuery),

		DefaultActivityRetryPolicy:   dc.GetMapByNamespace(dynamicconfig.DefaultActivityRetryPolicy)),
		DefaultWorkflowRetryPolicy:   dc.GetMapByNamespace(dynamicconfig.DefaultWorkflowRetryPolicy)),
		WorkflowTaskHeartbeatTimeout: dc.GetDurationByNamespace(dynamicconfig.WorkflowTaskHeartbeatTimeout),
		WorkflowTaskCriticalAttempts: dc.GetInt(dynamicconfig.WorkflowTaskCriticalAttempts),
		WorkflowTaskRetryMaxInterval: dc.GetDuration(dynamicconfig.WorkflowTaskRetryMaxInterval),

		ReplicationTaskFetcherParallelism:            dc.GetInt(dynamicconfig.ReplicationTaskFetcherParallelism),
		ReplicationTaskFetcherAggregationInterval:    dc.GetDuration(dynamicconfig.ReplicationTaskFetcherAggregationInterval),
		ReplicationTaskFetcherTimerJitterCoefficient: dc.GetFloat64(dynamicconfig.ReplicationTaskFetcherTimerJitterCoefficient),
		ReplicationTaskFetcherErrorRetryWait:         dc.GetDuration(dynamicconfig.ReplicationTaskFetcherErrorRetryWait),

		ReplicationTaskProcessorErrorRetryWait:               dc.GetDurationPropertyFilteredByShardID(dynamicconfig.ReplicationTaskProcessorErrorRetryWait, 1*time.Second),
		ReplicationTaskProcessorErrorRetryBackoffCoefficient: dc.GetFloat64PropertyFilteredByShardID(dynamicconfig.ReplicationTaskProcessorErrorRetryBackoffCoefficient, 1.2),
		ReplicationTaskProcessorErrorRetryMaxInterval:        dc.GetDurationPropertyFilteredByShardID(dynamicconfig.ReplicationTaskProcessorErrorRetryMaxInterval, 5*time.Second),
		ReplicationTaskProcessorErrorRetryMaxAttempts:        dc.GetIntPropertyFilteredByShardID(dynamicconfig.ReplicationTaskProcessorErrorRetryMaxAttempts, 80),
		ReplicationTaskProcessorErrorRetryExpiration:         dc.GetDurationPropertyFilteredByShardID(dynamicconfig.ReplicationTaskProcessorErrorRetryExpiration, 5*time.Minute),
		ReplicationTaskProcessorNoTaskRetryWait:              dc.GetDurationPropertyFilteredByShardID(dynamicconfig.ReplicationTaskProcessorNoTaskInitialWait, 2*time.Second),
		ReplicationTaskProcessorCleanupInterval:              dc.GetDurationPropertyFilteredByShardID(dynamicconfig.ReplicationTaskProcessorCleanupInterval, 1*time.Minute),
		ReplicationTaskProcessorCleanupJitterCoefficient:     dc.GetFloat64PropertyFilteredByShardID(dynamicconfig.ReplicationTaskProcessorCleanupJitterCoefficient, 0.15),

		MaxBufferedQueryCount:                 dc.GetInt(dynamicconfig.MaxBufferedQueryCount),
		MutableStateChecksumGenProbability:    dc.GetIntByNamespace(dynamicconfig.MutableStateChecksumGenProbability),
		MutableStateChecksumVerifyProbability: dc.GetIntByNamespace(dynamicconfig.MutableStateChecksumVerifyProbability),
		MutableStateChecksumInvalidateBefore:  dc.GetFloat64(dynamicconfig.MutableStateChecksumInvalidateBefore),

		StandbyTaskReReplicationContextTimeout: dc.GetDurationByNamespaceID(dynamicconfig.StandbyTaskReReplicationContextTimeout),

		SkipReapplicationByNamespaceID: dc.GetBoolByNamespaceID(dynamicconfig.SkipReapplicationByNamespaceID),

		// ===== Visibility related =====
		VisibilityTaskBatchSize:                               dc.GetInt(dynamicconfig.VisibilityTaskBatchSize),
		VisibilityProcessorMaxPollRPS:                         dc.GetInt(dynamicconfig.VisibilityProcessorMaxPollRPS),
		VisibilityProcessorMaxPollHostRPS:                     dc.GetInt(dynamicconfig.VisibilityProcessorMaxPollHostRPS),
		VisibilityProcessorSchedulerWorkerCount:               dc.GetInt(dynamicconfig.VisibilityProcessorSchedulerWorkerCount),
		VisibilityProcessorSchedulerActiveRoundRobinWeights:   dc.GetMapPropertyFnFilteredByNamespace(dynamicconfig.WithDefault(dynamicconfig.VisibilityProcessorSchedulerActiveRoundRobinWeights, ConvertWeightsToDynamicConfigValue(DefaultActiveTaskPriorityWeight))),
		VisibilityProcessorSchedulerStandbyRoundRobinWeights:  dc.GetMapPropertyFnFilteredByNamespace(dynamicconfig.WithDefault(dynamicconfig.VisibilityProcessorSchedulerStandbyRoundRobinWeights, ConvertWeightsToDynamicConfigValue(DefaultStandbyTaskPriorityWeight))),
		VisibilityProcessorMaxPollInterval:                    dc.GetDuration(dynamicconfig.VisibilityProcessorMaxPollInterval),
		VisibilityProcessorMaxPollIntervalJitterCoefficient:   dc.GetFloat64(dynamicconfig.VisibilityProcessorMaxPollIntervalJitterCoefficient),
		VisibilityProcessorUpdateAckInterval:                  dc.GetDuration(dynamicconfig.VisibilityProcessorUpdateAckInterval),
		VisibilityProcessorUpdateAckIntervalJitterCoefficient: dc.GetFloat64(dynamicconfig.VisibilityProcessorUpdateAckIntervalJitterCoefficient),
		VisibilityProcessorPollBackoffInterval:                dc.GetDuration(dynamicconfig.VisibilityProcessorPollBackoffInterval),
		VisibilityProcessorEnsureCloseBeforeDelete:            dc.GetBool(dynamicconfig.VisibilityProcessorEnsureCloseBeforeDelete),
		VisibilityProcessorEnableCloseWorkflowCleanup:         dc.GetBoolByNamespace(dynamicconfig.VisibilityProcessorEnableCloseWorkflowCleanup),
		VisibilityQueueMaxReaderCount:                         dc.GetInt(dynamicconfig.VisibilityQueueMaxReaderCount),

		SearchAttributesNumberOfKeysLimit: dc.GetIntByNamespace(dynamicconfig.SearchAttributesNumberOfKeysLimit),
		SearchAttributesSizeOfValueLimit:  dc.GetIntByNamespace(dynamicconfig.SearchAttributesSizeOfValueLimit),
		SearchAttributesTotalSizeLimit:    dc.GetIntByNamespace(dynamicconfig.SearchAttributesTotalSizeLimit),
		IndexerConcurrency:                dc.GetInt(dynamicconfig.WorkerIndexerConcurrency),
		ESProcessorNumOfWorkers:           dc.GetInt(dynamicconfig.WorkerESProcessorNumOfWorkers),
		// Should not be greater than number of visibility task queue workers VisibilityProcessorSchedulerWorkerCount (default 512)
		// Otherwise, visibility queue processors won't be able to fill up bulk with documents (even under heavy load) and bulk will flush due to interval, not number of actions.
		ESProcessorBulkActions: dc.GetInt(dynamicconfig.WorkerESProcessorBulkActions),
		// 16MB - just a sanity check. With ES document size ~1Kb it should never be reached.
		ESProcessorBulkSize: dc.GetInt(dynamicconfig.WorkerESProcessorBulkSize),
		// Bulk processor will flush every this interval regardless of last flush due to bulk actions.
		ESProcessorFlushInterval: dc.GetDuration(dynamicconfig.WorkerESProcessorFlushInterval),
		ESProcessorAckTimeout:    dc.GetDuration(dynamicconfig.WorkerESProcessorAckTimeout),

		EnableCrossNamespaceCommands:  dc.GetBool(dynamicconfig.EnableCrossNamespaceCommands),
		EnableActivityEagerExecution:  dc.GetBoolByNamespace(dynamicconfig.EnableActivityEagerExecution),
		EnableEagerWorkflowStart:      dc.GetBoolByNamespace(dynamicconfig.EnableEagerWorkflowStart),
		NamespaceCacheRefreshInterval: dc.GetDuration(dynamicconfig.NamespaceCacheRefreshInterval),

		// Archival related
		ArchivalTaskBatchSize:                               dc.GetInt(dynamicconfig.ArchivalTaskBatchSize),
		ArchivalProcessorMaxPollRPS:                         dc.GetInt(dynamicconfig.ArchivalProcessorMaxPollRPS),
		ArchivalProcessorMaxPollHostRPS:                     dc.GetInt(dynamicconfig.ArchivalProcessorMaxPollHostRPS),
		ArchivalProcessorSchedulerWorkerCount:               dc.GetInt(dynamicconfig.ArchivalProcessorSchedulerWorkerCount),
		ArchivalProcessorMaxPollInterval:                    dc.GetDuration(dynamicconfig.ArchivalProcessorMaxPollInterval),
		ArchivalProcessorMaxPollIntervalJitterCoefficient:   dc.GetFloat64(dynamicconfig.ArchivalProcessorMaxPollIntervalJitterCoefficient),
		ArchivalProcessorUpdateAckInterval:                  dc.GetDuration(dynamicconfig.ArchivalProcessorUpdateAckInterval),
		ArchivalProcessorUpdateAckIntervalJitterCoefficient: dc.GetFloat64(dynamicconfig.ArchivalProcessorUpdateAckIntervalJitterCoefficient),
		ArchivalProcessorPollBackoffInterval:                dc.GetDuration(dynamicconfig.ArchivalProcessorPollBackoffInterval),
		ArchivalProcessorArchiveDelay:                       dc.GetDuration(dynamicconfig.ArchivalProcessorArchiveDelay),
		ArchivalBackendMaxRPS:                               dc.GetFloat64(dynamicconfig.ArchivalBackendMaxRPS),
		ArchivalQueueMaxReaderCount:                         dc.GetInt(dynamicconfig.ArchivalQueueMaxReaderCount),

		// workflow update related
		WorkflowExecutionMaxInFlightUpdates: dc.GetIntByNamespace(dynamicconfig.WorkflowExecutionMaxInFlightUpdates),
		WorkflowExecutionMaxTotalUpdates:    dc.GetIntByNamespace(dynamicconfig.WorkflowExecutionMaxTotalUpdates),

		SendRawWorkflowHistory: dc.GetBoolByNamespace(dynamicconfig.SendRawWorkflowHistory),

		FrontendAccessHistoryFraction: dc.GetFloat64(dynamicconfig.FrontendAccessHistoryFraction),
	}

	return cfg
}

// GetShardID return the corresponding shard ID for a given namespaceID and workflowID pair
func (config *Config) GetShardID(namespaceID namespace.ID, workflowID string) int32 {
	return common.WorkflowIDToHistoryShard(namespaceID.String(), workflowID, config.NumberOfShards)
}

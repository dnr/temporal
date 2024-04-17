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

package matching

import (
	"time"

	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/persistence/visibility"
	"go.temporal.io/server/common/tqid"
)

type (
	// Config represents configuration for matching service
	Config struct {
		PersistenceMaxQPS                     dynamicconfig.IntPropertyFn
		PersistenceGlobalMaxQPS               dynamicconfig.IntPropertyFn
		PersistenceNamespaceMaxQPS            dynamicconfig.IntPropertyFnWithNamespaceFilter
		PersistenceGlobalNamespaceMaxQPS      dynamicconfig.IntPropertyFnWithNamespaceFilter
		PersistencePerShardNamespaceMaxQPS    dynamicconfig.IntPropertyFnWithNamespaceFilter
		EnablePersistencePriorityRateLimiting dynamicconfig.BoolPropertyFn
		PersistenceDynamicRateLimitingParams  dynamicconfig.MapPropertyFn
		PersistenceQPSBurstRatio              dynamicconfig.FloatPropertyFn
		SyncMatchWaitDuration                 dynamicconfig.DurationPropertyFnWithTaskQueueFilter
		TestDisableSyncMatch                  dynamicconfig.BoolPropertyFn
		RPS                                   dynamicconfig.IntPropertyFn
		OperatorRPSRatio                      dynamicconfig.FloatPropertyFn
		AlignMembershipChange                 dynamicconfig.DurationPropertyFn
		ShutdownDrainDuration                 dynamicconfig.DurationPropertyFn
		HistoryMaxPageSize                    dynamicconfig.IntPropertyFnWithNamespaceFilter

		// task queue configuration

		RangeSize                                int64
		GetTasksBatchSize                        dynamicconfig.IntPropertyFnWithTaskQueueFilter
		UpdateAckInterval                        dynamicconfig.DurationPropertyFnWithTaskQueueFilter
		MaxTaskQueueIdleTime                     dynamicconfig.DurationPropertyFnWithTaskQueueFilter
		NumTaskqueueWritePartitions              dynamicconfig.IntPropertyFnWithTaskQueueFilter
		NumTaskqueueReadPartitions               dynamicconfig.IntPropertyFnWithTaskQueueFilter
		ForwarderMaxOutstandingPolls             dynamicconfig.IntPropertyFnWithTaskQueueFilter
		ForwarderMaxOutstandingTasks             dynamicconfig.IntPropertyFnWithTaskQueueFilter
		ForwarderMaxRatePerSecond                dynamicconfig.IntPropertyFnWithTaskQueueFilter
		ForwarderMaxChildrenPerNode              dynamicconfig.IntPropertyFnWithTaskQueueFilter
		VersionCompatibleSetLimitPerQueue        dynamicconfig.IntPropertyFnWithNamespaceFilter
		VersionBuildIdLimitPerQueue              dynamicconfig.IntPropertyFnWithNamespaceFilter
		AssignmentRuleLimitPerQueue              dynamicconfig.IntPropertyFnWithNamespaceFilter
		RedirectRuleLimitPerQueue                dynamicconfig.IntPropertyFnWithNamespaceFilter
		RedirectRuleChainLimitPerQueue           dynamicconfig.IntPropertyFnWithNamespaceFilter
		DeletedRuleRetentionTime                 dynamicconfig.DurationPropertyFnWithNamespaceFilter
		ReachabilityBuildIdVisibilityGracePeriod dynamicconfig.DurationPropertyFnWithNamespaceFilter
		TaskQueueLimitPerBuildId                 dynamicconfig.IntPropertyFnWithNamespaceFilter
		GetUserDataLongPollTimeout               dynamicconfig.DurationPropertyFn
		BacklogNegligibleAge                     dynamicconfig.DurationPropertyFnWithTaskQueueFilter
		MaxWaitForPollerBeforeFwd                dynamicconfig.DurationPropertyFnWithTaskQueueFilter
		QueryPollerUnavailableWindow             dynamicconfig.DurationPropertyFn
		QueryWorkflowTaskTimeoutLogRate          dynamicconfig.FloatPropertyFnWithTaskQueueFilter
		MembershipUnloadDelay                    dynamicconfig.DurationPropertyFn

		// Time to hold a poll request before returning an empty response if there are no tasks
		LongPollExpirationInterval dynamicconfig.DurationPropertyFnWithTaskQueueFilter
		MinTaskThrottlingBurstSize dynamicconfig.IntPropertyFnWithTaskQueueFilter
		MaxTaskDeleteBatchSize     dynamicconfig.IntPropertyFnWithTaskQueueFilter

		// taskWriter configuration
		OutstandingTaskAppendsThreshold dynamicconfig.IntPropertyFnWithTaskQueueFilter
		MaxTaskBatchSize                dynamicconfig.IntPropertyFnWithTaskQueueFilter

		ThrottledLogRPS dynamicconfig.IntPropertyFn

		AdminNamespaceToPartitionDispatchRate          dynamicconfig.FloatPropertyFnWithNamespaceFilter
		AdminNamespaceTaskqueueToPartitionDispatchRate dynamicconfig.FloatPropertyFnWithTaskQueueFilter

		VisibilityPersistenceMaxReadQPS   dynamicconfig.IntPropertyFn
		VisibilityPersistenceMaxWriteQPS  dynamicconfig.IntPropertyFn
		EnableReadFromSecondaryVisibility dynamicconfig.BoolPropertyFnWithNamespaceFilter
		VisibilityDisableOrderByClause    dynamicconfig.BoolPropertyFnWithNamespaceFilter
		VisibilityEnableManualPagination  dynamicconfig.BoolPropertyFnWithNamespaceFilter

		LoadUserData dynamicconfig.BoolPropertyFnWithTaskQueueFilter

		ListNexusIncomingServicesLongPollTimeout dynamicconfig.DurationPropertyFn

		// FrontendAccessHistoryFraction is an interim flag across 2 minor releases and will be removed once fully enabled.
		FrontendAccessHistoryFraction dynamicconfig.FloatPropertyFn
	}

	forwarderConfig struct {
		ForwarderMaxOutstandingPolls func() int
		ForwarderMaxOutstandingTasks func() int
		ForwarderMaxRatePerSecond    func() int
		ForwarderMaxChildrenPerNode  func() int
	}

	taskQueueConfig struct {
		forwarderConfig
		SyncMatchWaitDuration        func() time.Duration
		BacklogNegligibleAge         func() time.Duration
		MaxWaitForPollerBeforeFwd    func() time.Duration
		QueryPollerUnavailableWindow func() time.Duration
		TestDisableSyncMatch         func() bool
		// Time to hold a poll request before returning an empty response if there are no tasks
		LongPollExpirationInterval func() time.Duration
		RangeSize                  int64
		GetTasksBatchSize          func() int
		UpdateAckInterval          func() time.Duration
		MaxTaskQueueIdleTime       func() time.Duration
		MinTaskThrottlingBurstSize func() int
		MaxTaskDeleteBatchSize     func() int

		GetUserDataLongPollTimeout dynamicconfig.DurationPropertyFn
		GetUserDataMinWaitTime     time.Duration

		// taskWriter configuration
		OutstandingTaskAppendsThreshold func() int
		MaxTaskBatchSize                func() int
		NumWritePartitions              func() int
		NumReadPartitions               func() int

		// partition qps = AdminNamespaceToPartitionDispatchRate(namespace)
		AdminNamespaceToPartitionDispatchRate func() float64
		// partition qps = AdminNamespaceTaskQueueToPartitionDispatchRate(namespace, task_queue)
		AdminNamespaceTaskQueueToPartitionDispatchRate func() float64

		// Retry policy for fetching user data from root partition. Should retry forever.
		GetUserDataRetryPolicy backoff.RetryPolicy
	}
)

// NewConfig returns new service config with default values
func NewConfig(
	dc *dynamicconfig.Collection,
) *Config {
	return &Config{
		PersistenceMaxQPS:                        dc.GetInt(dynamicconfig.MatchingPersistenceMaxQPS),
		PersistenceGlobalMaxQPS:                  dc.GetInt(dynamicconfig.MatchingPersistenceGlobalMaxQPS),
		PersistenceNamespaceMaxQPS:               dc.GetIntByNamespace(dynamicconfig.MatchingPersistenceNamespaceMaxQPS),
		PersistenceGlobalNamespaceMaxQPS:         dc.GetIntByNamespace(dynamicconfig.MatchingPersistenceGlobalNamespaceMaxQPS),
		PersistencePerShardNamespaceMaxQPS:       dynamicconfig.DefaultPerShardNamespaceRPSMax,
		EnablePersistencePriorityRateLimiting:    dc.GetBool(dynamicconfig.MatchingEnablePersistencePriorityRateLimiting),
		PersistenceDynamicRateLimitingParams:     dc.GetMap(dynamicconfig.MatchingPersistenceDynamicRateLimitingParams),
		PersistenceQPSBurstRatio:                 dc.GetFloat(dynamicconfig.PersistenceQPSBurstRatio),
		SyncMatchWaitDuration:                    dc.GetDurationByTaskQueue(dynamicconfig.MatchingSyncMatchWaitDuration),
		TestDisableSyncMatch:                     dc.GetBool(dynamicconfig.TestMatchingDisableSyncMatch),
		LoadUserData:                             dc.GetBoolByTaskQueue(dynamicconfig.MatchingLoadUserData),
		HistoryMaxPageSize:                       dc.GetIntByNamespace(dynamicconfig.MatchingHistoryMaxPageSize),
		RPS:                                      dc.GetInt(dynamicconfig.MatchingRPS),
		OperatorRPSRatio:                         dc.GetFloat(dynamicconfig.OperatorRPSRatio),
		RangeSize:                                100000,
		GetTasksBatchSize:                        dc.GetIntByTaskQueue(dynamicconfig.MatchingGetTasksBatchSize),
		UpdateAckInterval:                        dc.GetDurationByTaskQueue(dynamicconfig.MatchingUpdateAckInterval),
		MaxTaskQueueIdleTime:                     dc.GetDurationByTaskQueue(dynamicconfig.MatchingMaxTaskQueueIdleTime),
		LongPollExpirationInterval:               dc.GetDurationByTaskQueue(dynamicconfig.MatchingLongPollExpirationInterval),
		MinTaskThrottlingBurstSize:               dc.GetIntByTaskQueue(dynamicconfig.MatchingMinTaskThrottlingBurstSize),
		MaxTaskDeleteBatchSize:                   dc.GetIntByTaskQueue(dynamicconfig.MatchingMaxTaskDeleteBatchSize),
		OutstandingTaskAppendsThreshold:          dc.GetIntByTaskQueue(dynamicconfig.MatchingOutstandingTaskAppendsThreshold),
		MaxTaskBatchSize:                         dc.GetIntByTaskQueue(dynamicconfig.MatchingMaxTaskBatchSize),
		ThrottledLogRPS:                          dc.GetInt(dynamicconfig.MatchingThrottledLogRPS),
		NumTaskqueueWritePartitions:              dc.GetIntByTaskQueue(dynamicconfig.MatchingNumTaskqueueWritePartitions),
		NumTaskqueueReadPartitions:               dc.GetIntByTaskQueue(dynamicconfig.MatchingNumTaskqueueReadPartitions),
		ForwarderMaxOutstandingPolls:             dc.GetIntByTaskQueue(dynamicconfig.MatchingForwarderMaxOutstandingPolls),
		ForwarderMaxOutstandingTasks:             dc.GetIntByTaskQueue(dynamicconfig.MatchingForwarderMaxOutstandingTasks),
		ForwarderMaxRatePerSecond:                dc.GetIntByTaskQueue(dynamicconfig.MatchingForwarderMaxRatePerSecond),
		ForwarderMaxChildrenPerNode:              dc.GetIntByTaskQueue(dynamicconfig.MatchingForwarderMaxChildrenPerNode),
		AlignMembershipChange:                    dc.GetDuration(dynamicconfig.MatchingAlignMembershipChange),
		ShutdownDrainDuration:                    dc.GetDuration(dynamicconfig.MatchingShutdownDrainDuration),
		VersionCompatibleSetLimitPerQueue:        dc.GetIntByNamespace(dynamicconfig.VersionCompatibleSetLimitPerQueue),
		VersionBuildIdLimitPerQueue:              dc.GetIntByNamespace(dynamicconfig.VersionBuildIdLimitPerQueue),
		AssignmentRuleLimitPerQueue:              dc.GetIntByNamespace(dynamicconfig.AssignmentRuleLimitPerQueue),
		RedirectRuleLimitPerQueue:                dc.GetIntByNamespace(dynamicconfig.RedirectRuleLimitPerQueue),
		RedirectRuleChainLimitPerQueue:           dc.GetIntByNamespace(dynamicconfig.RedirectRuleChainLimitPerQueue),
		DeletedRuleRetentionTime:                 dc.GetDurationByNamespace(dynamicconfig.MatchingDeletedRuleRetentionTime),
		ReachabilityBuildIdVisibilityGracePeriod: dc.GetDurationByNamespace(dynamicconfig.ReachabilityBuildIdVisibilityGracePeriod),
		TaskQueueLimitPerBuildId:                 dc.GetIntByNamespace(dynamicconfig.TaskQueuesPerBuildIdLimit),
		GetUserDataLongPollTimeout:               dc.GetDuration(dynamicconfig.MatchingGetUserDataLongPollTimeout), // Use -10 seconds so that we send back empty response instead of timeout
		BacklogNegligibleAge:                     dc.GetDurationByTaskQueue(dynamicconfig.MatchingBacklogNegligibleAge),
		MaxWaitForPollerBeforeFwd:                dc.GetDurationByTaskQueue(dynamicconfig.MatchingMaxWaitForPollerBeforeFwd),
		QueryPollerUnavailableWindow:             dc.GetDuration(dynamicconfig.QueryPollerUnavailableWindow),
		QueryWorkflowTaskTimeoutLogRate:          dc.GetFloatByTaskQueue(dynamicconfig.MatchingQueryWorkflowTaskTimeoutLogRate),
		MembershipUnloadDelay:                    dc.GetDuration(dynamicconfig.MatchingMembershipUnloadDelay),

		AdminNamespaceToPartitionDispatchRate:          dc.GetFloatByNamespace(dynamicconfig.AdminMatchingNamespaceToPartitionDispatchRate),
		AdminNamespaceTaskqueueToPartitionDispatchRate: dc.GetFloatByTaskQueue(dynamicconfig.AdminMatchingNamespaceTaskqueueToPartitionDispatchRate),

		VisibilityPersistenceMaxReadQPS:   visibility.GetVisibilityPersistenceMaxReadQPS(dc),
		VisibilityPersistenceMaxWriteQPS:  visibility.GetVisibilityPersistenceMaxWriteQPS(dc),
		EnableReadFromSecondaryVisibility: visibility.GetEnableReadFromSecondaryVisibilityConfig(dc),
		VisibilityDisableOrderByClause:    dc.GetBoolByNamespace(dynamicconfig.VisibilityDisableOrderByClause),
		VisibilityEnableManualPagination:  dc.GetBoolByNamespace(dynamicconfig.VisibilityEnableManualPagination),

		ListNexusIncomingServicesLongPollTimeout: dc.GetDuration(dynamicconfig.MatchingListNexusIncomingServicesLongPollTimeout), // Use -10 seconds so that we send back empty response instead of timeout

		FrontendAccessHistoryFraction: dc.GetFloat(dynamicconfig.FrontendAccessHistoryFraction),
	}
}

func newTaskQueueConfig(tq *tqid.TaskQueue, config *Config, ns namespace.Name) *taskQueueConfig {
	taskQueueName := tq.Name()
	taskType := tq.TaskType()

	return &taskQueueConfig{
		RangeSize: config.RangeSize,
		GetTasksBatchSize: func() int {
			return config.GetTasksBatchSize(ns.String(), taskQueueName, taskType)
		},
		UpdateAckInterval: func() time.Duration {
			return config.UpdateAckInterval(ns.String(), taskQueueName, taskType)
		},
		MaxTaskQueueIdleTime: func() time.Duration {
			return config.MaxTaskQueueIdleTime(ns.String(), taskQueueName, taskType)
		},
		MinTaskThrottlingBurstSize: func() int {
			return config.MinTaskThrottlingBurstSize(ns.String(), taskQueueName, taskType)
		},
		SyncMatchWaitDuration: func() time.Duration {
			return config.SyncMatchWaitDuration(ns.String(), taskQueueName, taskType)
		},
		BacklogNegligibleAge: func() time.Duration {
			return config.BacklogNegligibleAge(ns.String(), taskQueueName, taskType)
		},
		MaxWaitForPollerBeforeFwd: func() time.Duration {
			return config.MaxWaitForPollerBeforeFwd(ns.String(), taskQueueName, taskType)
		},
		QueryPollerUnavailableWindow: config.QueryPollerUnavailableWindow,
		TestDisableSyncMatch:         config.TestDisableSyncMatch,
		LongPollExpirationInterval: func() time.Duration {
			return config.LongPollExpirationInterval(ns.String(), taskQueueName, taskType)
		},
		MaxTaskDeleteBatchSize: func() int {
			return config.MaxTaskDeleteBatchSize(ns.String(), taskQueueName, taskType)
		},
		GetUserDataLongPollTimeout: config.GetUserDataLongPollTimeout,
		GetUserDataMinWaitTime:     1 * time.Second,
		OutstandingTaskAppendsThreshold: func() int {
			return config.OutstandingTaskAppendsThreshold(ns.String(), taskQueueName, taskType)
		},
		MaxTaskBatchSize: func() int {
			return config.MaxTaskBatchSize(ns.String(), taskQueueName, taskType)
		},
		NumWritePartitions: func() int {
			return max(1, config.NumTaskqueueWritePartitions(ns.String(), taskQueueName, taskType))
		},
		NumReadPartitions: func() int {
			return max(1, config.NumTaskqueueReadPartitions(ns.String(), taskQueueName, taskType))
		},
		AdminNamespaceToPartitionDispatchRate: func() float64 {
			return config.AdminNamespaceToPartitionDispatchRate(ns.String())
		},
		AdminNamespaceTaskQueueToPartitionDispatchRate: func() float64 {
			return config.AdminNamespaceTaskqueueToPartitionDispatchRate(ns.String(), taskQueueName, taskType)
		},
		forwarderConfig: forwarderConfig{
			ForwarderMaxOutstandingPolls: func() int {
				return config.ForwarderMaxOutstandingPolls(ns.String(), taskQueueName, taskType)
			},
			ForwarderMaxOutstandingTasks: func() int {
				return config.ForwarderMaxOutstandingTasks(ns.String(), taskQueueName, taskType)
			},
			ForwarderMaxRatePerSecond: func() int {
				return config.ForwarderMaxRatePerSecond(ns.String(), taskQueueName, taskType)
			},
			ForwarderMaxChildrenPerNode: func() int {
				return max(1, config.ForwarderMaxChildrenPerNode(ns.String(), taskQueueName, taskType))
			},
		},
		GetUserDataRetryPolicy: backoff.NewExponentialRetryPolicy(1 * time.Second).WithMaximumInterval(5 * time.Minute),
	}
}

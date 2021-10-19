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

package shard

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/serviceerror"

	persistencespb "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/convert"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/common/persistence"
	"go.temporal.io/server/common/primitives/timestamp"
	"go.temporal.io/server/common/resource"
	"go.temporal.io/server/common/tasks"
	"go.temporal.io/server/service/history/configs"
	"go.temporal.io/server/service/history/events"
)

var (
	defaultTime = time.Unix(0, 0)
)

const (
	// These are the possible values of ContextImpl.status
	contextStatusInitialized contextStatus = iota
	contextStatusAcquiring
	contextStatusAcquired
	contextStatusStopped

	// These are the requests that can be sent to the context lifecycle
	contextRequestAcquire contextRequest = iota
	contextRequestAcquired
	contextRequestLost
	contextRequestStop
)

type (
	contextStatus  int32
	contextRequest int

	ContextImpl struct {
		// These fields are constant:
		resource.Resource
		shardID          int32
		executionManager persistence.ExecutionManager
		metricsClient    metrics.Client
		eventsCache      events.Cache
		closeCallback    func(*ContextImpl)
		config           *configs.Config
		logger           log.Logger
		throttledLogger  log.Logger
		engineFactory    EngineFactory
		requestCh        chan contextRequest

		// All following fields are protected by rwLock, and only valid if status >= Acquiring:
		rwLock                    sync.RWMutex
		status                    contextStatus
		engine                    Engine
		lastUpdated               time.Time
		shardInfo                 *persistence.ShardInfoWithFailover
		transferSequenceNumber    int64
		maxTransferSequenceNumber int64
		transferMaxReadLevel      int64
		timerMaxReadLevelMap      map[string]time.Time // cluster -> timerMaxReadLevel

		// exist only in memory
		remoteClusterCurrentTime map[string]time.Time

		// true if previous owner was different from the acquirer's identity.
		previousShardOwnerWasDifferent bool
	}
)

var _ Context = (*ContextImpl)(nil)

var (
	// ErrShardClosed is returned when shard is closed and a req cannot be processed
	ErrShardClosed = errors.New("shard closed")

	// ErrShardStatusUnknown means we're not sure if we have the shard lock or not. This may be returned
	// during short windows at initialization and if we've lost the connection to the database.
	// FIXME: rename this?
	ErrShardStatusUnknown = errors.New("shard status unknown")
)

const (
	logWarnTransferLevelDiff = 3000000 // 3 million
	logWarnTimerLevelDiff    = time.Duration(30 * time.Minute)
	historySizeLogThreshold  = 10 * 1024 * 1024
)

func (s *ContextImpl) GetShardID() int32 {
	// constant from initialization, no need for locks
	return s.shardID
}

func (s *ContextImpl) GetService() resource.Resource {
	// constant from initialization, no need for locks
	return s.Resource
}

func (s *ContextImpl) GetExecutionManager() persistence.ExecutionManager {
	// constant from initialization, no need for locks
	return s.executionManager
}

func (s *ContextImpl) GetEngine() Engine {
	s.rlock()
	defer s.runlock()

	return s.engine
}

func (s *ContextImpl) SetEngine(engine Engine) {
	s.lock()
	defer s.unlock()

	s.engine = engine
}

func (s *ContextImpl) GenerateTransferTaskID() (int64, error) {
	s.lock()
	defer s.unlock()

	return s.generateTransferTaskIDLocked()
}

func (s *ContextImpl) GenerateTransferTaskIDs(number int) ([]int64, error) {
	s.lock()
	defer s.unlock()

	result := []int64{}
	for i := 0; i < number; i++ {
		id, err := s.generateTransferTaskIDLocked()
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

func (s *ContextImpl) GetTransferMaxReadLevel() int64 {
	s.rlock()
	defer s.runlock()
	return s.transferMaxReadLevel
}

func (s *ContextImpl) GetTransferAckLevel() int64 {
	s.rlock()
	defer s.runlock()

	return s.shardInfo.TransferAckLevel
}

func (s *ContextImpl) UpdateTransferAckLevel(ackLevel int64) error {
	s.lock()
	defer s.unlock()

	s.shardInfo.TransferAckLevel = ackLevel
	s.shardInfo.StolenSinceRenew = 0
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) GetTransferClusterAckLevel(cluster string) int64 {
	s.rlock()
	defer s.runlock()

	// if we can find corresponding ack level
	if ackLevel, ok := s.shardInfo.ClusterTransferAckLevel[cluster]; ok {
		return ackLevel
	}
	// otherwise, default to existing ack level, which belongs to local cluster
	// this can happen if you add more cluster
	return s.shardInfo.TransferAckLevel
}

func (s *ContextImpl) UpdateTransferClusterAckLevel(cluster string, ackLevel int64) error {
	s.lock()
	defer s.unlock()

	s.shardInfo.ClusterTransferAckLevel[cluster] = ackLevel
	s.shardInfo.StolenSinceRenew = 0
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) GetVisibilityAckLevel() int64 {
	s.rlock()
	defer s.runlock()

	return s.shardInfo.VisibilityAckLevel
}

func (s *ContextImpl) UpdateVisibilityAckLevel(ackLevel int64) error {
	s.lock()
	defer s.unlock()

	s.shardInfo.VisibilityAckLevel = ackLevel
	s.shardInfo.StolenSinceRenew = 0
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) GetReplicatorAckLevel() int64 {
	s.rlock()
	defer s.runlock()

	return s.shardInfo.ReplicationAckLevel
}

func (s *ContextImpl) UpdateReplicatorAckLevel(ackLevel int64) error {
	s.lock()
	defer s.unlock()
	s.shardInfo.ReplicationAckLevel = ackLevel
	s.shardInfo.StolenSinceRenew = 0
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) GetReplicatorDLQAckLevel(sourceCluster string) int64 {
	s.rlock()
	defer s.runlock()

	if ackLevel, ok := s.shardInfo.ReplicationDlqAckLevel[sourceCluster]; ok {
		return ackLevel
	}
	return -1
}

func (s *ContextImpl) UpdateReplicatorDLQAckLevel(
	sourceCluster string,
	ackLevel int64,
) error {

	s.lock()
	defer s.unlock()

	s.shardInfo.ReplicationDlqAckLevel[sourceCluster] = ackLevel
	s.shardInfo.StolenSinceRenew = 0
	if err := s.updateShardInfoLocked(); err != nil {
		return err
	}

	s.GetMetricsClient().Scope(
		metrics.ReplicationDLQStatsScope,
		metrics.TargetClusterTag(sourceCluster),
		metrics.InstanceTag(convert.Int32ToString(s.shardID)),
	).UpdateGauge(
		metrics.ReplicationDLQAckLevelGauge,
		float64(ackLevel),
	)
	return nil
}

func (s *ContextImpl) GetClusterReplicationLevel(cluster string) int64 {
	s.rlock()
	defer s.runlock()

	// if we can find corresponding replication level
	if replicationLevel, ok := s.shardInfo.ClusterReplicationLevel[cluster]; ok {
		return replicationLevel
	}

	// New cluster always starts from -1
	return persistence.EmptyQueueMessageID
}

func (s *ContextImpl) UpdateClusterReplicationLevel(cluster string, ackTaskID int64) error {
	s.lock()
	defer s.unlock()

	s.shardInfo.ClusterReplicationLevel[cluster] = ackTaskID
	s.shardInfo.StolenSinceRenew = 0
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) GetTimerAckLevel() time.Time {
	s.rlock()
	defer s.runlock()

	return timestamp.TimeValue(s.shardInfo.TimerAckLevelTime)
}

func (s *ContextImpl) UpdateTimerAckLevel(ackLevel time.Time) error {
	s.lock()
	defer s.unlock()

	s.shardInfo.TimerAckLevelTime = &ackLevel
	s.shardInfo.StolenSinceRenew = 0
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) GetTimerClusterAckLevel(cluster string) time.Time {
	s.rlock()
	defer s.runlock()

	// if we can find corresponding ack level
	if ackLevel, ok := s.shardInfo.ClusterTimerAckLevel[cluster]; ok {
		return timestamp.TimeValue(ackLevel)
	}
	// otherwise, default to existing ack level, which belongs to local cluster
	// this can happen if you add more cluster
	return timestamp.TimeValue(s.shardInfo.TimerAckLevelTime)
}

func (s *ContextImpl) UpdateTimerClusterAckLevel(cluster string, ackLevel time.Time) error {
	s.lock()
	defer s.unlock()

	s.shardInfo.ClusterTimerAckLevel[cluster] = &ackLevel
	s.shardInfo.StolenSinceRenew = 0
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) UpdateTransferFailoverLevel(failoverID string, level persistence.TransferFailoverLevel) error {
	s.lock()
	defer s.unlock()

	s.shardInfo.TransferFailoverLevels[failoverID] = level
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) DeleteTransferFailoverLevel(failoverID string) error {
	s.lock()
	defer s.unlock()

	if level, ok := s.shardInfo.TransferFailoverLevels[failoverID]; ok {
		s.GetMetricsClient().RecordTimer(metrics.ShardInfoScope, metrics.ShardInfoTransferFailoverLatencyTimer, time.Since(level.StartTime))
		delete(s.shardInfo.TransferFailoverLevels, failoverID)
	}
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) GetAllTransferFailoverLevels() map[string]persistence.TransferFailoverLevel {
	s.rlock()
	defer s.runlock()

	ret := map[string]persistence.TransferFailoverLevel{}
	for k, v := range s.shardInfo.TransferFailoverLevels {
		ret[k] = v
	}
	return ret
}

func (s *ContextImpl) UpdateTimerFailoverLevel(failoverID string, level persistence.TimerFailoverLevel) error {
	s.lock()
	defer s.unlock()

	s.shardInfo.TimerFailoverLevels[failoverID] = level
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) DeleteTimerFailoverLevel(failoverID string) error {
	s.lock()
	defer s.unlock()

	if level, ok := s.shardInfo.TimerFailoverLevels[failoverID]; ok {
		s.GetMetricsClient().RecordTimer(metrics.ShardInfoScope, metrics.ShardInfoTimerFailoverLatencyTimer, time.Since(level.StartTime))
		delete(s.shardInfo.TimerFailoverLevels, failoverID)
	}
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) GetAllTimerFailoverLevels() map[string]persistence.TimerFailoverLevel {
	s.rlock()
	defer s.runlock()

	ret := map[string]persistence.TimerFailoverLevel{}
	for k, v := range s.shardInfo.TimerFailoverLevels {
		ret[k] = v
	}
	return ret
}

func (s *ContextImpl) GetNamespaceNotificationVersion() int64 {
	s.rlock()
	defer s.runlock()

	return s.shardInfo.NamespaceNotificationVersion
}

func (s *ContextImpl) UpdateNamespaceNotificationVersion(namespaceNotificationVersion int64) error {
	s.lock()
	defer s.unlock()

	s.shardInfo.NamespaceNotificationVersion = namespaceNotificationVersion
	return s.updateShardInfoLocked()
}

func (s *ContextImpl) GetTimerMaxReadLevel(cluster string) time.Time {
	s.rlock()
	defer s.runlock()

	return s.timerMaxReadLevelMap[cluster]
}

func (s *ContextImpl) UpdateTimerMaxReadLevel(cluster string) time.Time {
	s.lock()
	defer s.unlock()

	currentTime := s.GetTimeSource().Now()
	if cluster != "" && cluster != s.GetClusterMetadata().GetCurrentClusterName() {
		currentTime = s.remoteClusterCurrentTime[cluster]
	}

	s.timerMaxReadLevelMap[cluster] = currentTime.Add(s.config.TimerProcessorMaxTimeShift()).Truncate(time.Millisecond)
	return s.timerMaxReadLevelMap[cluster]
}

func (s *ContextImpl) CreateWorkflowExecution(
	request *persistence.CreateWorkflowExecutionRequest,
) (*persistence.CreateWorkflowExecutionResponse, error) {
	if err := s.errorByStatus(); err != nil {
		return nil, err
	}

	namespaceID := request.NewWorkflowSnapshot.ExecutionInfo.NamespaceId
	workflowID := request.NewWorkflowSnapshot.ExecutionInfo.WorkflowId

	// do not try to get namespace cache within shard lock
	namespaceEntry, err := s.GetNamespaceRegistry().GetNamespaceByID(namespaceID)
	if err != nil {
		return nil, err
	}

	s.lock()
	defer s.unlock()

	transferMaxReadLevel := int64(0)
	if err := s.allocateTaskIDsLocked(
		namespaceEntry,
		workflowID,
		request.NewWorkflowSnapshot.TransferTasks,
		request.NewWorkflowSnapshot.ReplicationTasks,
		request.NewWorkflowSnapshot.TimerTasks,
		request.NewWorkflowSnapshot.VisibilityTasks,
		&transferMaxReadLevel,
	); err != nil {
		return nil, err
	}
	defer s.updateMaxReadLevelLocked(transferMaxReadLevel)

	currentRangeID := s.getRangeIDLocked()
	request.RangeID = currentRangeID
	resp, err := s.executionManager.CreateWorkflowExecution(request)
	if err = s.handleErrorLocked(err); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *ContextImpl) UpdateWorkflowExecution(
	request *persistence.UpdateWorkflowExecutionRequest,
) (*persistence.UpdateWorkflowExecutionResponse, error) {
	if err := s.errorByStatus(); err != nil {
		return nil, err
	}

	namespaceID := request.UpdateWorkflowMutation.ExecutionInfo.NamespaceId
	workflowID := request.UpdateWorkflowMutation.ExecutionInfo.WorkflowId

	// do not try to get namespace cache within shard lock
	namespaceEntry, err := s.GetNamespaceRegistry().GetNamespaceByID(namespaceID)
	if err != nil {
		return nil, err
	}

	s.lock()
	defer s.unlock()

	transferMaxReadLevel := int64(0)
	if err := s.allocateTaskIDsLocked(
		namespaceEntry,
		workflowID,
		request.UpdateWorkflowMutation.TransferTasks,
		request.UpdateWorkflowMutation.ReplicationTasks,
		request.UpdateWorkflowMutation.TimerTasks,
		request.UpdateWorkflowMutation.VisibilityTasks,
		&transferMaxReadLevel,
	); err != nil {
		return nil, err
	}
	if request.NewWorkflowSnapshot != nil {
		if err := s.allocateTaskIDsLocked(
			namespaceEntry,
			workflowID,
			request.NewWorkflowSnapshot.TransferTasks,
			request.NewWorkflowSnapshot.ReplicationTasks,
			request.NewWorkflowSnapshot.TimerTasks,
			request.NewWorkflowSnapshot.VisibilityTasks,
			&transferMaxReadLevel,
		); err != nil {
			return nil, err
		}
	}
	defer s.updateMaxReadLevelLocked(transferMaxReadLevel)

	currentRangeID := s.getRangeIDLocked()
	request.RangeID = currentRangeID
	resp, err := s.executionManager.UpdateWorkflowExecution(request)
	if err = s.handleErrorLocked(err); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *ContextImpl) ConflictResolveWorkflowExecution(
	request *persistence.ConflictResolveWorkflowExecutionRequest,
) (*persistence.ConflictResolveWorkflowExecutionResponse, error) {
	if err := s.errorByStatus(); err != nil {
		return nil, err
	}

	namespaceID := request.ResetWorkflowSnapshot.ExecutionInfo.NamespaceId
	workflowID := request.ResetWorkflowSnapshot.ExecutionInfo.WorkflowId

	// do not try to get namespace cache within shard lock
	namespaceEntry, err := s.GetNamespaceRegistry().GetNamespaceByID(namespaceID)
	if err != nil {
		return nil, err
	}

	s.lock()
	defer s.unlock()

	transferMaxReadLevel := int64(0)
	if request.CurrentWorkflowMutation != nil {
		if err := s.allocateTaskIDsLocked(
			namespaceEntry,
			workflowID,
			request.CurrentWorkflowMutation.TransferTasks,
			request.CurrentWorkflowMutation.ReplicationTasks,
			request.CurrentWorkflowMutation.TimerTasks,
			request.CurrentWorkflowMutation.VisibilityTasks,
			&transferMaxReadLevel,
		); err != nil {
			return nil, err
		}
	}
	if err := s.allocateTaskIDsLocked(
		namespaceEntry,
		workflowID,
		request.ResetWorkflowSnapshot.TransferTasks,
		request.ResetWorkflowSnapshot.ReplicationTasks,
		request.ResetWorkflowSnapshot.TimerTasks,
		request.ResetWorkflowSnapshot.VisibilityTasks,
		&transferMaxReadLevel,
	); err != nil {
		return nil, err
	}
	if request.NewWorkflowSnapshot != nil {
		if err := s.allocateTaskIDsLocked(
			namespaceEntry,
			workflowID,
			request.NewWorkflowSnapshot.TransferTasks,
			request.NewWorkflowSnapshot.ReplicationTasks,
			request.NewWorkflowSnapshot.TimerTasks,
			request.NewWorkflowSnapshot.VisibilityTasks,
			&transferMaxReadLevel,
		); err != nil {
			return nil, err
		}
	}
	defer s.updateMaxReadLevelLocked(transferMaxReadLevel)

	currentRangeID := s.getRangeIDLocked()
	request.RangeID = currentRangeID
	resp, err := s.executionManager.ConflictResolveWorkflowExecution(request)
	if err := s.handleErrorLocked(err); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *ContextImpl) AddTasks(
	request *persistence.AddTasksRequest,
) error {
	if err := s.errorByStatus(); err != nil {
		return err
	}

	namespaceID := request.NamespaceID
	workflowID := request.WorkflowID

	// do not try to get namespace cache within shard lock
	namespaceEntry, err := s.GetNamespaceRegistry().GetNamespaceByID(namespaceID)
	if err != nil {
		return err
	}

	s.lock()
	defer s.unlock()

	transferMaxReadLevel := int64(0)
	if err := s.allocateTaskIDsLocked(
		namespaceEntry,
		workflowID,
		request.TransferTasks,
		request.ReplicationTasks,
		request.TimerTasks,
		request.VisibilityTasks,
		&transferMaxReadLevel,
	); err != nil {
		return err
	}
	defer s.updateMaxReadLevelLocked(transferMaxReadLevel)

	request.RangeID = s.getRangeIDLocked()
	err = s.executionManager.AddTasks(request)
	if err = s.handleErrorLocked(err); err != nil {
		return err
	}
	s.engine.NotifyNewTransferTasks(request.TransferTasks)
	s.engine.NotifyNewTimerTasks(request.TimerTasks)
	s.engine.NotifyNewVisibilityTasks(request.VisibilityTasks)
	s.engine.NotifyNewReplicationTasks(request.ReplicationTasks)
	return nil
}

func (s *ContextImpl) AppendHistoryEvents(
	request *persistence.AppendHistoryNodesRequest,
	namespaceID string,
	execution commonpb.WorkflowExecution,
) (int, error) {
	if err := s.errorByStatus(); err != nil {
		return 0, err
	}

	request.ShardID = s.shardID

	size := 0
	defer func() {
		// N.B. - Dual emit here makes sense so that we can see aggregate timer stats across all
		// namespaces along with the individual namespaces stats
		s.GetMetricsClient().RecordDistribution(metrics.SessionSizeStatsScope, metrics.HistorySize, size)
		if entry, err := s.GetNamespaceRegistry().GetNamespaceByID(namespaceID); err == nil && entry != nil {
			s.GetMetricsClient().Scope(
				metrics.SessionSizeStatsScope,
				metrics.NamespaceTag(entry.Name()),
			).RecordDistribution(metrics.HistorySize, size)
		}
		if size >= historySizeLogThreshold {
			s.throttledLogger.Warn("history size threshold breached",
				tag.WorkflowID(execution.GetWorkflowId()),
				tag.WorkflowRunID(execution.GetRunId()),
				tag.WorkflowNamespaceID(namespaceID),
				tag.WorkflowHistorySizeBytes(size))
		}
	}()
	resp, err0 := s.GetExecutionManager().AppendHistoryNodes(request)
	if resp != nil {
		size = resp.Size
	}
	return size, err0
}

func (s *ContextImpl) GetConfig() *configs.Config {
	// constant from initialization, no need for locks
	return s.config
}

func (s *ContextImpl) GetEventsCache() events.Cache {
	// constant from initialization (except for tests), no need for locks
	return s.eventsCache
}

func (s *ContextImpl) SetEventsCacheForTesting(c events.Cache) {
	// for testing only, will only be called immediately after initialization
	s.eventsCache = c
}

func (s *ContextImpl) GetLogger() log.Logger {
	// constant from initialization, no need for locks
	return s.logger
}

func (s *ContextImpl) GetThrottledLogger() log.Logger {
	// constant from initialization, no need for locks
	return s.throttledLogger
}

func (s *ContextImpl) getRangeIDLocked() int64 {
	return s.shardInfo.GetRangeId()
}

func (s *ContextImpl) errorByStatus() error {
	s.rlock()
	defer s.runlock()
	return s.errorByStatusLocked()
}

func (s *ContextImpl) errorByStatusLocked() error {
	switch s.status {
	case contextStatusInitialized:
		return ErrShardStatusUnknown
	case contextStatusAcquiring:
		return ErrShardStatusUnknown
	case contextStatusAcquired:
		return nil
	case contextStatusStopped:
		return ErrShardClosed
	default:
		panic("invalid status")
	}
}

func (s *ContextImpl) closeShardLocked() {
	// FIXME: is this enough? should we do anything else here?
	s.logger.Info("Close shard")

	// will cause controller to call s.stop()
	go s.closeCallback(s)

	// fails any writes that may start after this point.
	s.shardInfo.RangeId = -1
}

func (s *ContextImpl) generateTransferTaskIDLocked() (int64, error) {
	if err := s.updateRangeIfNeededLocked(); err != nil {
		return -1, err
	}

	taskID := s.transferSequenceNumber
	s.transferSequenceNumber++

	return taskID, nil
}

func (s *ContextImpl) updateRangeIfNeededLocked() error {
	if s.transferSequenceNumber < s.maxTransferSequenceNumber {
		return nil
	}

	return s.renewRangeLocked(false)
}

func (s *ContextImpl) renewRangeLocked(isStealing bool) error {
	// FIXME: review this carefully

	updatedShardInfo := copyShardInfo(s.shardInfo)
	updatedShardInfo.RangeId++
	if isStealing {
		updatedShardInfo.StolenSinceRenew++
	}

	err := s.GetShardManager().UpdateShard(&persistence.UpdateShardRequest{
		ShardInfo:       updatedShardInfo.ShardInfo,
		PreviousRangeID: s.shardInfo.GetRangeId()})
	if err != nil {
		// Failure in updating shard to grab new RangeID
		s.logger.Error("Persistent store operation failure",
			tag.StoreOperationUpdateShard,
			tag.Error(err),
			tag.ShardRangeID(updatedShardInfo.GetRangeId()),
			tag.PreviousShardRangeID(s.shardInfo.GetRangeId()),
		)
		// Shard is stolen, trigger history engine shutdown
		if IsShardOwnershipLostError(err) {
			s.closeShardLocked()
		}
		return err
	}

	// Range is successfully updated in cassandra now update shard context to reflect new range
	s.logger.Info("Range updated for shardID",
		tag.ShardRangeID(updatedShardInfo.RangeId),
		tag.PreviousShardRangeID(s.shardInfo.RangeId),
		tag.Number(s.transferSequenceNumber),
		tag.NextNumber(s.maxTransferSequenceNumber),
	)

	s.transferSequenceNumber = updatedShardInfo.GetRangeId() << s.config.RangeSizeBits
	s.maxTransferSequenceNumber = (updatedShardInfo.GetRangeId() + 1) << s.config.RangeSizeBits
	s.transferMaxReadLevel = s.transferSequenceNumber - 1
	s.shardInfo = updatedShardInfo

	return nil
}

func (s *ContextImpl) updateMaxReadLevelLocked(rl int64) {
	if rl > s.transferMaxReadLevel {
		s.logger.Debug("Updating MaxTaskID", tag.MaxLevel(rl))
		s.transferMaxReadLevel = rl
	}
}

func (s *ContextImpl) updateShardInfoLocked() error {
	if err := s.errorByStatusLocked(); err != nil {
		return err
	}

	var err error
	now := clock.NewRealTimeSource().Now()
	if s.lastUpdated.Add(s.config.ShardUpdateMinInterval()).After(now) {
		return nil
	}
	updatedShardInfo := copyShardInfo(s.shardInfo)
	s.emitShardInfoMetricsLogsLocked()

	err = s.GetShardManager().UpdateShard(&persistence.UpdateShardRequest{
		ShardInfo:       updatedShardInfo.ShardInfo,
		PreviousRangeID: s.shardInfo.GetRangeId(),
	})

	if err != nil {
		// Shard is stolen, trigger history engine shutdown
		if IsShardOwnershipLostError(err) {
			s.closeShardLocked()
		}
	} else {
		s.lastUpdated = now
	}

	return err
}

func (s *ContextImpl) emitShardInfoMetricsLogsLocked() {
	currentCluster := s.GetClusterMetadata().GetCurrentClusterName()

	minTransferLevel := s.shardInfo.ClusterTransferAckLevel[currentCluster]
	maxTransferLevel := s.shardInfo.ClusterTransferAckLevel[currentCluster]
	for _, v := range s.shardInfo.ClusterTransferAckLevel {
		if v < minTransferLevel {
			minTransferLevel = v
		}
		if v > maxTransferLevel {
			maxTransferLevel = v
		}
	}
	diffTransferLevel := maxTransferLevel - minTransferLevel

	minTimerLevel := timestamp.TimeValue(s.shardInfo.ClusterTimerAckLevel[currentCluster])
	maxTimerLevel := timestamp.TimeValue(s.shardInfo.ClusterTimerAckLevel[currentCluster])
	for _, v := range s.shardInfo.ClusterTimerAckLevel {
		t := timestamp.TimeValue(v)
		if t.Before(minTimerLevel) {
			minTimerLevel = t
		}
		if t.After(maxTimerLevel) {
			maxTimerLevel = t
		}
	}
	diffTimerLevel := maxTimerLevel.Sub(minTimerLevel)

	replicationLag := s.transferMaxReadLevel - s.shardInfo.ReplicationAckLevel
	transferLag := s.transferMaxReadLevel - s.shardInfo.TransferAckLevel
	timerLag := time.Since(timestamp.TimeValue(s.shardInfo.TimerAckLevelTime))

	transferFailoverInProgress := len(s.shardInfo.TransferFailoverLevels)
	timerFailoverInProgress := len(s.shardInfo.TimerFailoverLevels)

	if s.config.EmitShardDiffLog() &&
		(logWarnTransferLevelDiff < diffTransferLevel ||
			logWarnTimerLevelDiff < diffTimerLevel ||
			logWarnTransferLevelDiff < transferLag ||
			logWarnTimerLevelDiff < timerLag) {

		s.logger.Warn("Shard ack levels diff exceeds warn threshold.",
			tag.ShardTime(s.remoteClusterCurrentTime),
			tag.ShardReplicationAck(s.shardInfo.ReplicationAckLevel),
			tag.ShardTimerAcks(s.shardInfo.ClusterTimerAckLevel),
			tag.ShardTransferAcks(s.shardInfo.ClusterTransferAckLevel))
	}

	s.GetMetricsClient().RecordDistribution(metrics.ShardInfoScope, metrics.ShardInfoTransferDiffTimer, int(diffTransferLevel))
	s.GetMetricsClient().RecordTimer(metrics.ShardInfoScope, metrics.ShardInfoTimerDiffTimer, diffTimerLevel)

	s.GetMetricsClient().RecordDistribution(metrics.ShardInfoScope, metrics.ShardInfoReplicationLagTimer, int(replicationLag))
	s.GetMetricsClient().RecordDistribution(metrics.ShardInfoScope, metrics.ShardInfoTransferLagTimer, int(transferLag))
	s.GetMetricsClient().RecordTimer(metrics.ShardInfoScope, metrics.ShardInfoTimerLagTimer, timerLag)

	s.GetMetricsClient().RecordDistribution(metrics.ShardInfoScope, metrics.ShardInfoTransferFailoverInProgressTimer, transferFailoverInProgress)
	s.GetMetricsClient().RecordDistribution(metrics.ShardInfoScope, metrics.ShardInfoTimerFailoverInProgressTimer, timerFailoverInProgress)
}

func (s *ContextImpl) allocateTaskIDsLocked(
	namespaceEntry *namespace.Namespace,
	workflowID string,
	transferTasks []tasks.Task,
	replicationTasks []tasks.Task,
	timerTasks []tasks.Task,
	visibilityTasks []tasks.Task,
	transferMaxReadLevel *int64,
) error {

	if err := s.allocateTransferIDsLocked(
		transferTasks,
		transferMaxReadLevel); err != nil {
		return err
	}
	if err := s.allocateTransferIDsLocked(
		replicationTasks,
		transferMaxReadLevel); err != nil {
		return err
	}
	if err := s.allocateTransferIDsLocked(
		visibilityTasks,
		transferMaxReadLevel); err != nil {
		return err
	}
	return s.allocateTimerIDsLocked(
		namespaceEntry,
		workflowID,
		timerTasks)
}

func (s *ContextImpl) allocateTransferIDsLocked(
	tasks []tasks.Task,
	transferMaxReadLevel *int64,
) error {

	for _, task := range tasks {
		id, err := s.generateTransferTaskIDLocked()
		if err != nil {
			return err
		}
		s.logger.Debug("Assigning task ID", tag.TaskID(id))
		task.SetTaskID(id)
		*transferMaxReadLevel = id
	}
	return nil
}

// NOTE: allocateTimerIDsLocked should always been called after assigning taskID for transferTasks when assigning taskID together,
// because Temporal Indexer assume timer taskID of deleteWorkflowExecution is larger than transfer taskID of closeWorkflowExecution
// for a given workflow.
func (s *ContextImpl) allocateTimerIDsLocked(
	namespaceEntry *namespace.Namespace,
	workflowID string,
	timerTasks []tasks.Task,
) error {

	// assign IDs for the timer tasks. They need to be assigned under shard lock.
	currentCluster := s.GetClusterMetadata().GetCurrentClusterName()
	for _, task := range timerTasks {
		ts := task.GetVisibilityTime()
		if task.GetVersion() != common.EmptyVersion {
			// cannot use version to determine the corresponding cluster for timer task
			// this is because during failover, timer task should be created as active
			// or otherwise, failover + active processing logic may not pick up the task.
			currentCluster = namespaceEntry.ActiveClusterName()
		}
		readCursorTS := s.timerMaxReadLevelMap[currentCluster]
		if ts.Before(readCursorTS) {
			// This can happen if shard move and new host have a time SKU, or there is db write delay.
			// We generate a new timer ID using timerMaxReadLevel.
			s.logger.Debug("New timer generated is less than read level",
				tag.WorkflowNamespaceID(namespaceEntry.ID()),
				tag.WorkflowID(workflowID),
				tag.Timestamp(ts),
				tag.CursorTimestamp(readCursorTS),
				tag.ValueShardAllocateTimerBeforeRead)
			task.SetVisibilityTime(s.timerMaxReadLevelMap[currentCluster].Add(time.Millisecond))
		}

		seqNum, err := s.generateTransferTaskIDLocked()
		if err != nil {
			return err
		}
		task.SetTaskID(seqNum)
		visibilityTs := task.GetVisibilityTime()
		s.logger.Debug("Assigning new timer",
			tag.Timestamp(visibilityTs), tag.TaskID(task.GetTaskID()), tag.AckLevel(s.shardInfo.TimerAckLevelTime))
	}
	return nil
}

func (s *ContextImpl) SetCurrentTime(cluster string, currentTime time.Time) {
	s.lock()
	defer s.unlock()
	if cluster != s.GetClusterMetadata().GetCurrentClusterName() {
		prevTime := s.remoteClusterCurrentTime[cluster]
		if prevTime.Before(currentTime) {
			s.remoteClusterCurrentTime[cluster] = currentTime
		}
	} else {
		panic("Cannot set current time for current cluster")
	}
}

func (s *ContextImpl) GetCurrentTime(cluster string) time.Time {
	s.rlock()
	defer s.runlock()
	if cluster != s.GetClusterMetadata().GetCurrentClusterName() {
		return s.remoteClusterCurrentTime[cluster]
	}
	return s.GetTimeSource().Now().UTC()
}

func (s *ContextImpl) GetLastUpdatedTime() time.Time {
	s.rlock()
	defer s.runlock()
	return s.lastUpdated
}

func (s *ContextImpl) handleErrorLocked(err error) error {
	switch err.(type) {
	case nil:
		return nil

	case *persistence.CurrentWorkflowConditionFailedError,
		*persistence.WorkflowConditionFailedError,
		*persistence.ConditionFailedError,
		*serviceerror.ResourceExhausted:
		// No special handling required for these errors
		return err

	case *persistence.ShardOwnershipLostError:
		// Shard is stolen, trigger shutdown of history engine
		s.closeShardLocked()
		return err

	default:
		// We have no idea if the write failed or will eventually make it to
		// persistence. Increment RangeID to guarantee that subsequent reads
		// will either see that write, or know for certain that it failed.
		// This allows the callers to reliably check the outcome by performing
		// a read.
		s.requestCh <- contextRequestLost
		// FIXME: need to block until we know that status is updated? or more?
		return err
	}
}

func (s *ContextImpl) createEngineLocked() {
	// FIXME: better way thread this value through?
	if s.previousShardOwnerWasDifferent {
		s.GetMetricsClient().RecordTimer(metrics.ShardInfoScope, metrics.ShardContextAcquisitionLatency,
			s.GetCurrentTime(s.GetClusterMetadata().GetCurrentClusterName()).Sub(s.GetLastUpdatedTime()))
	}
	s.logger.Info("", tag.LifeCycleStarting, tag.ComponentShardEngine)
	s.engine = s.engineFactory.CreateEngine(s)
	s.engine.Start()
	s.logger.Info("", tag.LifeCycleStarted, tag.ComponentShardEngine)
}

func (s *ContextImpl) getOrCreateEngine() (Engine, error) {
	s.rlock()
	defer s.runlock()

	switch s.status {
	case contextStatusInitialized:
		s.requestCh <- contextRequestAcquire
		// FIXME: should we try to wait a little while for it to acquire?
		return nil, ErrShardStatusUnknown
	case contextStatusAcquiring:
		s.requestCh <- contextRequestAcquire
		// FIXME: should we try to wait a little while for it to acquire?
		return nil, ErrShardStatusUnknown
	case contextStatusAcquired:
		// engine should be set here, but there might be a benign race where it isn't
		if s.engine == nil {
			s.logger.Error("engine not set in acquired status")
			return nil, ErrShardStatusUnknown
		}
		return s.engine, nil
	case contextStatusStopped:
		return nil, fmt.Errorf("shard %v for host '%v' is shut down", s.shardID, s.GetHostInfo().Identity())
	default:
		panic("invalid status")
	}
}

func (s *ContextImpl) stop() {
	s.requestCh <- contextRequestStop
}

func (s *ContextImpl) isValid() bool {
	// FIXME: always call callback when stopping so that we get removed from controller and we don't need this?
	s.rlock()
	defer s.runlock()
	return s.status != contextStatusStopped
}

func (s *ContextImpl) lock() {
	scope := metrics.ShardInfoScope
	s.metricsClient.IncCounter(scope, metrics.LockRequests)
	sw := s.metricsClient.StartTimer(scope, metrics.LockLatency)
	defer sw.Stop()

	s.rwLock.Lock()
}

func (s *ContextImpl) rlock() {
	scope := metrics.ShardInfoScope
	s.metricsClient.IncCounter(scope, metrics.LockRequests)
	sw := s.metricsClient.StartTimer(scope, metrics.LockLatency)
	defer sw.Stop()

	s.rwLock.RLock()
}

func (s *ContextImpl) unlock() {
	s.rwLock.Unlock()
}

func (s *ContextImpl) runlock() {
	s.rwLock.RUnlock()
}

func (s *ContextImpl) String() string {
	// use memory address as shard context identity
	return fmt.Sprintf("%p", s)
}

func (s *ContextImpl) lifecycle() {
	/* State transitions:
	Initialized
		on: request to acquire shard -> Acquiring
		on: request to stop -> Stopped
	Acquiring
		on: shard acquired succesfully -> Acquired
		on: request to stop -> Stopped
	Acquired
		on: notification that we might not have shard anymore -> Acquiring
		on: request to stop -> Stopped
		before moving to this state, shardInfo, engine, and various other fields must be set
	Stopped
		terminal state, no transitions. this goroutine should not be running.
	*/

	// Context for cancelling acquire goroutine
	ctx, cancel := context.WithCancel(context.Background())

	for request := range s.requestCh {
		// We can transition to stop no matter what state we're in
		if request == contextRequestStop {
			break
		}

		s.lock()

		switch s.status {
		case contextStatusInitialized:
			switch request {
			case contextRequestAcquire:
				go s.acquireShard(ctx)
				s.status = contextStatusAcquiring
			default:
				s.logger.Warn("invalid request for state transition")
			}
		case contextStatusAcquiring:
			switch request {
			case contextRequestAcquire:
				// nothing to do, already acquiring
			case contextRequestAcquired:
				s.status = contextStatusAcquired
			case contextRequestLost:
				// nothing to do, already acquiring
			default:
				s.logger.Warn("invalid request for state transition")
			}
		case contextStatusAcquired:
			switch request {
			case contextRequestAcquire:
				// nothing to to do, already acquired
			case contextRequestLost:
				go s.acquireShard(ctx)
				s.status = contextStatusAcquiring
			default:
				s.logger.Warn("invalid request for state transition")
			}
		default:
			s.logger.Warn("in unexpected status")
		}

		s.unlock()
	}

	// FIXME: maybe this should be responsible for calling the close callback here?

	// Stop the acquire goroutine if it was running
	cancel()

	// Stop the engine if it was running
	s.lock()
	s.logger.Info("", tag.LifeCycleStopping, tag.ComponentShardEngine)
	s.engine.Stop()
	s.engine = nil
	s.logger.Info("", tag.LifeCycleStopped, tag.ComponentShardEngine)
	s.status = contextStatusStopped
	s.unlock()

	// FIXME: should we close this?
	//close(s.requestCh)
}

func (s *ContextImpl) loadOrCreateShardMetadata() (*persistence.ShardInfoWithFailover, error) {
	resp, err := s.GetShardManager().GetShard(&persistence.GetShardRequest{
		ShardID: s.shardID,
	})

	if _, ok := err.(*serviceerror.NotFound); ok {
		// EntityNotExistsError: doesn't exist in db yet, try to create it
		req := &persistence.CreateShardRequest{
			ShardInfo: &persistencespb.ShardInfo{
				ShardId: s.shardID,
			},
		}
		err = s.GetShardManager().CreateShard(req)
		if err != nil {
			return nil, err
		}
		return &persistence.ShardInfoWithFailover{ShardInfo: req.ShardInfo}, nil
	}

	if err != nil {
		return nil, err
	}

	return &persistence.ShardInfoWithFailover{ShardInfo: resp.ShardInfo}, nil
}

func (s *ContextImpl) loadShardMetadata() error {
	// Only have to do this once, we can just re-acquire the lock after that.
	s.rlock()
	if s.shardInfo != nil {
		s.runlock()
		return nil
	}
	s.runlock()

	shardInfo, err := s.loadOrCreateShardMetadata()
	if err != nil {
		s.logger.Error("Failed to load shard", tag.Error(err))
		return err
	}

	updatedShardInfo := copyShardInfo(shardInfo)
	ownershipChanged := shardInfo.Owner != s.GetHostInfo().Identity()
	updatedShardInfo.Owner = s.GetHostInfo().Identity()

	// initialize the cluster current time to be the same as ack level
	remoteClusterCurrentTime := make(map[string]time.Time)
	timerMaxReadLevelMap := make(map[string]time.Time)
	for clusterName, info := range s.GetClusterMetadata().GetAllClusterInfo() {
		if !info.Enabled {
			continue
		}

		currentReadTime := timestamp.TimeValue(shardInfo.TimerAckLevelTime)
		if clusterName != s.GetClusterMetadata().GetCurrentClusterName() {
			if currentTime, ok := shardInfo.ClusterTimerAckLevel[clusterName]; ok {
				currentReadTime = timestamp.TimeValue(currentTime)
			}

			remoteClusterCurrentTime[clusterName] = currentReadTime
			timerMaxReadLevelMap[clusterName] = currentReadTime
		} else { // active cluster
			timerMaxReadLevelMap[clusterName] = currentReadTime
		}

		timerMaxReadLevelMap[clusterName] = timerMaxReadLevelMap[clusterName].Truncate(time.Millisecond)
	}

	s.lock()
	defer s.unlock()

	s.shardInfo = updatedShardInfo
	s.remoteClusterCurrentTime = remoteClusterCurrentTime
	s.timerMaxReadLevelMap = timerMaxReadLevelMap
	s.previousShardOwnerWasDifferent = ownershipChanged

	return nil
}

func (s *ContextImpl) acquireShard(ctx context.Context) {

	// Retry for 5m, with interval up to 10s (default)
	// FIXME2: change values?
	policy := backoff.NewExponentialRetryPolicy(50 * time.Millisecond)
	policy.SetExpirationInterval(5 * time.Minute)

	isRetryable := func(err error) bool {
		if common.IsPersistenceTransientError(err) {
			return true
		}
		// Retry this in case we need to create the shard and race with someone else doing it.
		// FIXME: that really shouldn't happen, should it?
		if _, ok := err.(*persistence.ShardAlreadyExistError); ok {
			return true
		}
		return false
	}

	try := func(ctx context.Context) error {
		// Initial load of shard metadata
		err := s.loadShardMetadata()
		if err != nil {
			return err
		}

		s.lock()
		defer s.unlock()

		// Try to acquire rangeid
		err = s.renewRangeLocked(true)

		// If we got it, create the engine if we don't have it yet
		if err == nil && s.engine == nil {
			s.createEngineLocked()
		}
		return err
	}

	err := backoff.RetryContext(ctx, try, policy, isRetryable)
	if err == nil {
		s.logger.Info("Acquired shard")
		s.requestCh <- contextRequestAcquired
		return
	}

	// We got an unretryable error (perhaps ShardOwnershipLostError, or context cancelled) or timed out.
	// Stop the shard.
	s.logger.Error("Couldn't acquire shard", tag.Error(err))
	s.requestCh <- contextRequestStop
}

func newContext(
	resource resource.Resource,
	shardID int32,
	factory EngineFactory,
	config *configs.Config,
	closeCallback func(*ContextImpl),
) (*ContextImpl, error) { // FIXME2: change back to Context?

	hostIdentity := resource.GetHostInfo().Identity()
	logger := log.With(resource.GetLogger(), tag.ShardID(shardID), tag.Address(hostIdentity))
	throttledLogger := log.With(resource.GetThrottledLogger(), tag.ShardID(shardID), tag.Address(hostIdentity))

	shardContext := &ContextImpl{
		Resource:         resource,
		status:           contextStatusInitialized,
		shardID:          shardID,
		executionManager: resource.GetExecutionManager(),
		metricsClient:    resource.GetMetricsClient(),
		closeCallback:    closeCallback,
		config:           config,
		logger:           logger,
		throttledLogger:  throttledLogger,
		requestCh:        make(chan contextRequest),
		engineFactory:    factory,
	}
	shardContext.eventsCache = events.NewEventsCache(
		shardContext.GetShardID(),
		shardContext.GetConfig().EventsCacheInitialSize(),
		shardContext.GetConfig().EventsCacheMaxSize(),
		shardContext.GetConfig().EventsCacheTTL(),
		shardContext.GetExecutionManager(),
		false,
		shardContext.GetLogger(),
		shardContext.GetMetricsClient(),
	)
	// Add tag for context itself to loggers
	shardContext.logger = log.With(shardContext.logger, tag.ShardContext(shardContext))
	shardContext.throttledLogger = log.With(shardContext.throttledLogger, tag.ShardContext(shardContext))

	// Start goroutine for lifecycle management
	go shardContext.lifecycle()

	return shardContext, nil
}

func copyShardInfo(shardInfo *persistence.ShardInfoWithFailover) *persistence.ShardInfoWithFailover {
	transferFailoverLevels := map[string]persistence.TransferFailoverLevel{}
	for k, v := range shardInfo.TransferFailoverLevels {
		transferFailoverLevels[k] = v
	}
	timerFailoverLevels := map[string]persistence.TimerFailoverLevel{}
	for k, v := range shardInfo.TimerFailoverLevels {
		timerFailoverLevels[k] = v
	}
	clusterTransferAckLevel := make(map[string]int64)
	for k, v := range shardInfo.ClusterTransferAckLevel {
		clusterTransferAckLevel[k] = v
	}
	clusterTimerAckLevel := make(map[string]*time.Time)
	for k, v := range shardInfo.ClusterTimerAckLevel {
		if timestamp.TimeValue(v).IsZero() {
			v = timestamp.TimePtr(defaultTime)
		}
		clusterTimerAckLevel[k] = v
	}
	clusterReplicationLevel := make(map[string]int64)
	for k, v := range shardInfo.ClusterReplicationLevel {
		clusterReplicationLevel[k] = v
	}
	clusterReplicationDLQLevel := make(map[string]int64)
	for k, v := range shardInfo.ReplicationDlqAckLevel {
		clusterReplicationDLQLevel[k] = v
	}
	if timestamp.TimeValue(shardInfo.TimerAckLevelTime).IsZero() {
		shardInfo.TimerAckLevelTime = timestamp.TimePtr(defaultTime)
	}
	shardInfoCopy := &persistence.ShardInfoWithFailover{
		ShardInfo: &persistencespb.ShardInfo{
			ShardId:                      shardInfo.GetShardId(),
			Owner:                        shardInfo.Owner,
			RangeId:                      shardInfo.GetRangeId(),
			StolenSinceRenew:             shardInfo.StolenSinceRenew,
			ReplicationAckLevel:          shardInfo.ReplicationAckLevel,
			TransferAckLevel:             shardInfo.TransferAckLevel,
			TimerAckLevelTime:            shardInfo.TimerAckLevelTime,
			ClusterTransferAckLevel:      clusterTransferAckLevel,
			ClusterTimerAckLevel:         clusterTimerAckLevel,
			NamespaceNotificationVersion: shardInfo.NamespaceNotificationVersion,
			ClusterReplicationLevel:      clusterReplicationLevel,
			ReplicationDlqAckLevel:       clusterReplicationDLQLevel,
			UpdateTime:                   shardInfo.UpdateTime,
			VisibilityAckLevel:           shardInfo.VisibilityAckLevel,
		},
		TransferFailoverLevels: transferFailoverLevels,
		TimerFailoverLevels:    timerFailoverLevels,
	}

	return shardInfoCopy
}

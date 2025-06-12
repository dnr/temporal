package matching

import (
	"context"
	"sync/atomic"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/service/matching/counter"
)

const (
	strideFactor = 10000
	minWeight    = 0.001
	// minWeight * strideFactor should be > 1
)

type (
	// fairTaskWriter writes tasks with stride scheduling
	fairTaskWriter struct {
		backlogMgr         *fairBacklogManagerImpl
		config             *taskQueueConfig
		db                 *taskQueueDB
		counter            counter.Counter
		logger             log.Logger
		appendCh           chan *writeTaskRequest
		taskIDBlock        taskIDBlock
		currentTaskIDBlock taskIDBlock // copy of taskIDBlock for safe concurrent access via getCurrentTaskIDBlock()
	}
)

func newFairTaskWriter(
	backlogMgr *fairBacklogManagerImpl,
) *fairTaskWriter {
	return &fairTaskWriter{
		backlogMgr:  backlogMgr,
		config:      backlogMgr.config,
		db:          backlogMgr.db,
		counter:     counter.NewMapCounter(), // FIXME: configurable
		logger:      backlogMgr.logger,
		appendCh:    make(chan *writeTaskRequest, backlogMgr.config.OutstandingTaskAppendsThreshold()),
		taskIDBlock: noTaskIDs,
	}
}

// Start fairTaskWriter background goroutine.
func (w *fairTaskWriter) Start() {
	go w.taskWriterLoop()
}

func (w *fairTaskWriter) appendTask(
	subqueue int,
	taskInfo *persistencespb.TaskInfo,
) error {
	select {
	case <-w.backlogMgr.tqCtx.Done():
		return errShutdown
	default:
		// noop
	}

	startTime := time.Now().UTC()
	ch := make(chan error)
	req := &writeTaskRequest{
		taskInfo:   taskInfo,
		responseCh: ch,
		subqueue:   subqueue,
	}

	select {
	case w.appendCh <- req:
		select {
		case err := <-ch:
			metrics.TaskWriteLatencyPerTaskQueue.With(w.backlogMgr.metricsHandler).Record(time.Since(startTime))
			return err
		case <-w.backlogMgr.tqCtx.Done():
			// if we are shutting down, this request will never make
			// it to cassandra, just bail out and fail this request
			return errShutdown
		}
	default: // channel is full, throttle
		metrics.TaskWriteThrottlePerTaskQueueCounter.With(w.backlogMgr.metricsHandler).Record(1)
		return &serviceerror.ResourceExhausted{
			Cause:   enumspb.RESOURCE_EXHAUSTED_CAUSE_SYSTEM_OVERLOADED,
			Scope:   enumspb.RESOURCE_EXHAUSTED_SCOPE_NAMESPACE,
			Message: "Too many outstanding appends to the task queue",
		}
	}
}

func (w *fairTaskWriter) allocTaskIDs(count int) ([]int64, error) {
	result := make([]int64, count)
	for i := range result {
		if w.taskIDBlock.start > w.taskIDBlock.end {
			// we ran out of current allocation block
			newBlock, err := w.allocTaskIDBlock(w.taskIDBlock.end)
			if err != nil {
				return nil, err
			}
			w.taskIDBlock = newBlock
		}
		result[i] = w.taskIDBlock.start
		w.taskIDBlock.start++
	}
	return result, nil
}

func (w *fairTaskWriter) allocPasses(tasks []*writeTaskRequest) []int64 {
	out := make([]int64, len(tasks))
	for i, task := range tasks {
		key := task.taskInfo.Priority.GetFairnessKey()
		weight := task.taskInfo.Priority.GetFairnessWeight()
		if weight == 0 {
			weight = 1.0
		}
		// FIXME: check override weights here
		weight = max(minWeight, weight)
		inc := max(1, int64(strideFactor/weight))
		base := int64(0) // FIXME: get ack level
		out[i] = w.counter.GetPass(key, base, inc)
	}
	return out
}

func (w *fairTaskWriter) appendTasks(
	taskIDs []int64,
	passes []int64,
	reqs []*writeTaskRequest,
) error {
	resp, err := w.db.CreateFairTasks(w.backlogMgr.tqCtx, taskIDs, passes, reqs)
	if err != nil {
		w.backlogMgr.signalIfFatal(err)
		w.logger.Error("Persistent store operation failure",
			tag.StoreOperationCreateTask,
			tag.Error(err),
			tag.WorkflowTaskQueueName(w.backlogMgr.queueKey().PersistenceName()),
			tag.WorkflowTaskQueueType(w.backlogMgr.queueKey().TaskType()))
		return err
	}

	w.backlogMgr.signalReaders(resp)
	return nil
}

func (w *fairTaskWriter) initState() error {
	retryForever := backoff.NewExponentialRetryPolicy(1 * time.Second).
		WithMaximumInterval(10 * time.Second).
		WithExpirationInterval(backoff.NoInterval)
	state, err := w.renewLeaseWithRetry(retryForever, common.IsPersistenceTransientError)
	if err != nil {
		w.backlogMgr.initState(taskQueueState{}, err)
		return err
	}
	w.taskIDBlock = rangeIDToTaskIDBlock(state.rangeID, w.config.RangeSize)
	w.currentTaskIDBlock = w.taskIDBlock
	w.backlogMgr.initState(state, nil)
	return nil
}

func (w *fairTaskWriter) taskWriterLoop() {
	if w.initState() != nil {
		return
	}

	var reqs []*writeTaskRequest
	for {
		atomic.StoreInt64(&w.currentTaskIDBlock.start, w.taskIDBlock.start)
		atomic.StoreInt64(&w.currentTaskIDBlock.end, w.taskIDBlock.end)

		select {
		case request := <-w.appendCh:
			// read a batch of requests from the channel
			reqs = append(reqs[:0], request)
			reqs = w.getWriteBatch(reqs)

			taskIDs, err := w.allocTaskIDs(len(reqs))
			if err == nil {
				passes := w.allocPasses(reqs)
				err = w.appendTasks(taskIDs, passes, reqs)
			}
			for _, req := range reqs {
				req.responseCh <- err
			}

		case <-w.backlogMgr.tqCtx.Done():
			return
		}
	}
}

func (w *fairTaskWriter) getWriteBatch(reqs []*writeTaskRequest) []*writeTaskRequest {
	for range w.config.MaxTaskBatchSize() - 1 {
		select {
		case req := <-w.appendCh:
			reqs = append(reqs, req)
		default: // channel is empty, don't block
			return reqs
		}
	}
	return reqs
}

func (w *fairTaskWriter) renewLeaseWithRetry(
	retryPolicy backoff.RetryPolicy,
	retryErrors backoff.IsRetryable,
) (taskQueueState, error) {
	var newState taskQueueState
	op := func(ctx context.Context) (err error) {
		newState, err = w.db.RenewLease(ctx)
		return
	}
	metrics.LeaseRequestPerTaskQueueCounter.With(w.backlogMgr.metricsHandler).Record(1)
	err := backoff.ThrottleRetryContext(w.backlogMgr.tqCtx, op, retryPolicy, retryErrors)
	if err != nil {
		metrics.LeaseFailurePerTaskQueueCounter.With(w.backlogMgr.metricsHandler).Record(1)
		return newState, err
	}
	return newState, nil
}

func (w *fairTaskWriter) allocTaskIDBlock(prevBlockEnd int64) (taskIDBlock, error) {
	currBlock := rangeIDToTaskIDBlock(w.db.RangeID(), w.config.RangeSize)
	if currBlock.end != prevBlockEnd {
		return taskIDBlock{}, errNonContiguousBlocks
	}
	state, err := w.renewLeaseWithRetry(persistenceOperationRetryPolicy, common.IsPersistenceTransientError)
	if err != nil {
		if w.backlogMgr.signalIfFatal(err) {
			return taskIDBlock{}, errShutdown
		}
		return taskIDBlock{}, err
	}
	return rangeIDToTaskIDBlock(state.rangeID, w.config.RangeSize), nil
}

// getCurrentTaskIDBlock returns the current taskIDBlock. Safe to be called concurrently.
func (w *fairTaskWriter) getCurrentTaskIDBlock() taskIDBlock {
	return taskIDBlock{
		start: atomic.LoadInt64(&w.currentTaskIDBlock.start),
		end:   atomic.LoadInt64(&w.currentTaskIDBlock.end),
	}
}

package matching

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/future"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/softassert"
	"go.temporal.io/server/service/matching/counter"
)

type (
	// fairTaskWriter writes tasks with stride scheduling
	fairTaskWriter struct {
		backlogMgr     *fairBacklogManagerImpl
		config         *taskQueueConfig
		db             *taskQueueDB
		logger         log.Logger
		counterFactory func() counter.Counter

		// state protected by lock:
		lock         sync.Mutex
		toWrite      []*writeBatch
		writeTimer   *time.Timer
		writePending bool

		// state maintained by doWrite (only <= 1 should run at once):
		taskIDBlock        taskIDBlock
		currentTaskIDBlock taskIDBlock // copy of taskIDBlock for safe concurrent access via getCurrentTaskIDBlock()
		counters           map[subqueueIndex]counter.Counter
	}

	writeBatch struct {
		fut   future.FutureImpl[error]
		tasks []writeTaskRequest
		start time.Time
	}
)

func newFairTaskWriter(
	backlogMgr *fairBacklogManagerImpl,
	counterFactory func() counter.Counter,
) *fairTaskWriter {
	return &fairTaskWriter{
		backlogMgr:     backlogMgr,
		config:         backlogMgr.config,
		db:             backlogMgr.db,
		logger:         backlogMgr.logger,
		counterFactory: counterFactory,

		taskIDBlock: noTaskIDs,
		counters:    make(map[subqueueIndex]counter.Counter),
	}
}

func (w *fairTaskWriter) Start() {
	go w.initState()
}

func (w *fairTaskWriter) appendTask(
	subqueue subqueueIndex,
	taskInfo *persistencespb.TaskInfo,
) error {
	if w.backlogMgr.tqCtx.Err() != nil {
		return errShutdown
	}

	startTime := time.Now()
	fut := w.submitReq(taskInfo, subqueue)
	if writeErr, ctxErr := fut.Get(w.backlogMgr.tqCtx); ctxErr == nil {
		metrics.TaskWriteLatencyPerTaskQueue.With(w.backlogMgr.metricsHandler).Record(time.Since(startTime))
		return writeErr
	} else {
		// if we are shutting down, this request will never make
		// it to persistence, just bail out and fail this request
		return errShutdown
	}
}

func (w *fairTaskWriter) bufferedTasksLocked() (total int) {
	for _, batch := range w.toWrite {
		total += len(batch.tasks)
	}
	return
}

func (w *fairTaskWriter) findOpenBatchLocked(batchSize int) *writeBatch {
	if l := len(w.toWrite); l > 0 {
		if batch := w.toWrite[l-1]; len(batch.tasks) < batchSize {
			return batch
		}
	}
	batch := &writeBatch{
		fut:   *future.NewFuture[error](),
		start: time.Now(),
	}
	w.toWrite = append(w.toWrite, batch)
	return batch
}

func (w *fairTaskWriter) getReadyBatchLocked(batchSize int, delay time.Duration, now time.Time) *writeBatch {
	if len(w.toWrite) == 0 {
		return nil
	}
	batch := w.toWrite[0]
	if len(batch.tasks) >= batchSize || now.Sub(batch.start) > delay {
		return batch
	}
	return nil
}

func (w *fairTaskWriter) setTimerLocked(delay time.Duration) {
	if !w.writePending && w.writeTimer == nil && len(w.toWrite) > 0 && len(w.toWrite[0].tasks) > 0 {
		w.writeTimer = time.AfterFunc(delay-time.Since(w.toWrite[0].start), w.doWriteTimer)
	}
}

func (w *fairTaskWriter) submitReq(taskInfo *persistencespb.TaskInfo, subqueue subqueueIndex) future.Future[error] {
	batchSize := w.config.MaxWriteBatchSize()
	bufferSize := w.config.OutstandingTaskAppendsThreshold()
	delay := w.config.TaskWriteDelay()

	w.lock.Lock()
	defer w.lock.Unlock()

	if w.bufferedTasksLocked() >= bufferSize {
		metrics.TaskWriteThrottlePerTaskQueueCounter.With(w.backlogMgr.metricsHandler).Record(1)
		return future.NewReadyFuture[error](&serviceerror.ResourceExhausted{
			Cause:   enumspb.RESOURCE_EXHAUSTED_CAUSE_SYSTEM_OVERLOADED,
			Scope:   enumspb.RESOURCE_EXHAUSTED_SCOPE_NAMESPACE,
			Message: "Too many outstanding appends to the task queue",
		}, nil)
	}

	batch := w.findOpenBatchLocked(batchSize)
	batch.tasks = append(batch.tasks, writeTaskRequest{taskInfo: taskInfo, subqueue: subqueue})

	if !w.writePending {
		w.setTimerLocked(delay)
		if w.getReadyBatchLocked(batchSize, delay, time.Now()) != nil {
			w.writeTimer.Reset(0)
		}
	}
	return &batch.fut
}

func (w *fairTaskWriter) doWriteTimer() {
	batchSize := w.config.MaxWriteBatchSize()
	delay := w.config.TaskWriteDelay()

	w.lock.Lock()
	defer w.lock.Unlock()

	w.writeTimer = nil
	softassert.That(w.logger, !w.writePending, "write should not be pending")
	w.writePending = true

	nextBatch := func() *writeBatch {
		batch := w.getReadyBatchLocked(batchSize, delay, time.Now())
		if batch != nil {
			w.toWrite = w.toWrite[1:]
		}
		return batch
	}

	for batch := nextBatch(); batch != nil; batch = nextBatch() {
		w.lock.Unlock()

		atomic.StoreInt64(&w.currentTaskIDBlock.start, w.taskIDBlock.start)
		atomic.StoreInt64(&w.currentTaskIDBlock.end, w.taskIDBlock.end)

		err := w.allocTaskIDs(batch.tasks)
		if err == nil {
			err = w.writeBatch(batch.tasks)
		}
		batch.fut.Set(err, nil)

		w.lock.Lock()
	}

	w.writePending = false
	w.setTimerLocked(delay)
}

func (w *fairTaskWriter) allocTaskIDs(reqs []writeTaskRequest) error {
	for i := range reqs {
		if w.taskIDBlock.start > w.taskIDBlock.end {
			// we ran out of current allocation block
			newBlock, err := w.allocTaskIDBlock(w.taskIDBlock.end)
			if err != nil {
				return err
			}
			w.taskIDBlock = newBlock
		}
		reqs[i].id = w.taskIDBlock.start
		w.taskIDBlock.start++
	}
	return nil
}

func (w *fairTaskWriter) pickPasses(tasks []writeTaskRequest, bases []fairLevel) {
	// TODO(fairness): get this from config
	var overrides fairnessWeightOverrides

	for i, task := range tasks {
		pri := task.taskInfo.Priority
		key := pri.GetFairnessKey()
		weight := getEffectiveWeight(overrides, pri)
		inc := max(1, int64(strideFactor/weight))
		base := bases[task.subqueue].pass
		cntr := w.counters[task.subqueue]
		if cntr == nil {
			cntr = w.counterFactory()
			w.counters[task.subqueue] = cntr
		}
		pass := cntr.GetPass(key, base, inc)
		softassert.That(w.logger, pass >= base, "counter returned pass below base")
		tasks[i].pass = pass
	}
}

func (w *fairTaskWriter) initState() error {
	state, err := w.renewLeaseWithRetry(foreverRetryPolicy, common.IsPersistenceTransientError)
	if err != nil {
		w.backlogMgr.initState(taskQueueState{}, err)
		return err
	}
	w.taskIDBlock = rangeIDToTaskIDBlock(state.rangeID, w.config.RangeSize)
	w.currentTaskIDBlock = w.taskIDBlock
	w.backlogMgr.initState(state, nil)
	return nil
}

func (w *fairTaskWriter) writeBatch(reqs []writeTaskRequest) (retErr error) {
	bases, unpin := w.backlogMgr.getAndPinAckLevels()
	defer func() { unpin(retErr) }()

	w.pickPasses(reqs, bases)
	resp, err := w.db.CreateFairTasks(w.backlogMgr.tqCtx, reqs)
	if err == nil {
		w.backlogMgr.wroteNewTasks(resp) // must be called before unpin()
	} else {
		w.logger.Error("Persistent store operation failure", tag.StoreOperationCreateTask, tag.Error(err))
		w.backlogMgr.signalIfFatal(err)
	}
	return err
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

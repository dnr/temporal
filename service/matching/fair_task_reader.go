package matching

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emirpasic/gods/maps/treemap"
	"go.temporal.io/api/serviceerror"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/clock"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/persistence"
	serviceerrors "go.temporal.io/server/common/serviceerror"
	"go.temporal.io/server/common/softassert"
	"go.temporal.io/server/common/util"
	"golang.org/x/sync/semaphore"
)

type (
	// FIXME: can we just call this dbLevel and use it everywhere?!
	fairLevel struct {
		pass int64
		id   int64
	}

	fairTaskReader struct {
		backlogMgr *fairBacklogManagerImpl
		subqueue   int
		logger     log.Logger

		lock sync.Mutex

		readPending  atomic.Bool
		backoffTimer *time.Timer
		retrier      backoff.Retrier

		backlogAge backlogAgeTracker

		addRetries *semaphore.Weighted

		// ack manager state
		outstandingTasks *treemap.Map // fairLevel -> *internalTask, or nil if acked
		loadedTasks      int          // == number of non-nil entries in outstandingTasks
		readLevel        fairLevel
		ackLevel         fairLevel // FIXME: make this inclusive everywhere
		ackLevelPinned   bool

		// gc state
		inGC       bool
		numToGC    int       // counts approximately how many tasks we can delete with a GC
		lastGCTime time.Time // last time GCed
	}
)

func newFairTaskReader(
	backlogMgr *fairBacklogManagerImpl,
	subqueue int,
	initialAckLevel fairLevel,
) *fairTaskReader {
	return &fairTaskReader{
		backlogMgr: backlogMgr,
		subqueue:   subqueue,
		logger:     backlogMgr.logger,
		retrier: backoff.NewRetrier(
			common.CreateReadTaskRetryPolicy(),
			clock.NewRealTimeSource(),
		),
		backlogAge: newBacklogAgeTracker(),
		addRetries: semaphore.NewWeighted(concurrentAddRetries),

		// ack manager
		outstandingTasks: treemap.NewWith(fairLevelComparator),
		readLevel:        fairLevelMax(initialAckLevel, fairLevel{pass: 1}), // FIXME: this is awkward, can we improve it?
		ackLevel:         initialAckLevel,

		// gc state
		lastGCTime: time.Now(),
	}
}

func (tr *fairTaskReader) Start() {
	tr.readTasks()
}

func (tr *fairTaskReader) getOldestBacklogTime() time.Time {
	tr.lock.Lock()
	defer tr.lock.Unlock()
	return tr.backlogAge.oldestTime()
}

func (tr *fairTaskReader) completeTask(task *internalTask, res taskResponse) {
	err := res.startErr
	if res.forwarded {
		err = res.forwardErr
	}

	// We can handle some transient errors by just putting the task back in the matcher to
	// match again. Note that for forwarded tasks, it's expected to get DeadlineExceeded when
	// the task doesn't match on the root after backlogTaskForwardTimeout, and also expected to
	// get errRemoteSyncMatchFailed, which is a serviceerror.Canceled error.
	if err != nil && (common.IsServiceClientTransientError(err) ||
		common.IsContextDeadlineExceededErr(err) ||
		common.IsContextCanceledErr(err)) {
		// TODO(pri): if this was a start error (not a forwarding error): consider adding a
		// per-task backoff here, in case the error was workflow busy, we don't want to end up
		// trying the same task immediately. maybe also: after a few attempts on the same task,
		// let it get cycled to the end of the queue, in case there's some task/wf-specific
		// thing.
		tr.addTaskToMatcher(task)
		return
	}

	// On other errors: ask backlog manager to re-spool to persistence
	if err != nil {
		if tr.backlogMgr.respoolTaskAfterError(task.event.Data) != nil {
			return // task queue will unload now
		}
	}

	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.backlogAge.record(task.event.Data.CreateTime, -1)
	tr.ackTaskLocked(allocatedTaskFairLevel(task.event.AllocatedTaskInfo))

	// use == so we just signal once when we cross this threshold
	// TODO(pri): is this safe? maybe we need to improve this
	if tr.loadedTasks == tr.backlogMgr.config.GetTasksReloadAt() {
		tr.readTasks()
	}
}

func (tr *fairTaskReader) readTasks() {
	if tr.readPending.CompareAndSwap(false, true) {
		go tr.readTasksImpl()
	}
}

func (tr *fairTaskReader) readTasksImpl() {
	defer tr.readPending.Store(false)

	reloadAt := tr.backlogMgr.config.GetTasksReloadAt()

	tr.lock.Lock()
	if tr.loadedTasks > reloadAt {
		// Too many loaded already. We'll get called again when loadedTasks drops low enough.
		tr.lock.Unlock()
		return
	}
	readLevel := tr.readLevel
	tr.lock.Unlock()

	maxReadLevel := tr.backlogMgr.db.GetMaxFairReadLevel(tr.subqueue)

	if fairLevelLess(maxReadLevel, readLevel) {
		// we're at the end, don't need to actually do a read
		return
	}

	batch := tr.backlogMgr.config.GetTasksBatchSize()
	res, err := tr.backlogMgr.db.GetFairTasks(tr.backlogMgr.tqCtx, tr.subqueue, readLevel, batch)
	if err != nil {
		tr.backlogMgr.signalIfFatal(err)
		// TODO: Should we ever stop retrying on db errors?
		if common.IsResourceExhausted(err) {
			tr.backoffSignal(taskReaderThrottleRetryDelay)
		} else {
			tr.backoffSignal(tr.retrier.NextBackOff(err))
		}
		return
	}
	tr.retrier.Reset()

	// filter out deleted
	tasks := slices.DeleteFunc(res.Tasks, func(t *persistencespb.AllocatedTaskInfo) bool {
		if IsTaskExpired(t) {
			metrics.ExpiredTasksPerTaskQueueCounter.With(tr.backlogMgr.metricsHandler).Record(1)
			return true
		}
		return false
	})

	if len(res.Tasks) > 0 {
		tr.mergeTasks(tasks)
	}
}

// call with_out_ lock held
func (tr *fairTaskReader) addTaskToMatcher(task *internalTask) {
	err := tr.backlogMgr.addSpooledTask(task)
	if err == nil {
		return
	}

	if drop, retry := tr.addErrorBehavior(err); drop {
		task.finish(nil, false)
	} else if retry {
		// This should only be due to persistence problems. Retry in a new goroutine
		// to not block other tasks, up to some concurrency limit.
		if tr.addRetries.Acquire(tr.backlogMgr.tqCtx, 1) != nil {
			return
		}
		go tr.retryAddAfterError(task)
	}
}

func (tr *fairTaskReader) addErrorBehavior(err error) (drop, retry bool) {
	// addSpooledTask can only fail due to:
	// - the task queue is closed (errTaskQueueClosed or context.Canceled)
	// - ValidateDeployment failed (InvalidArgument)
	// - versioning wants to get a versioned queue and it can't be initialized
	// - versioning wants to re-spool the task on a different queue and that failed
	// - versioning says StickyWorkerUnavailable
	if errors.Is(err, errTaskQueueClosed) || common.IsContextCanceledErr(err) {
		return false, false
	}
	var stickyUnavailable *serviceerrors.StickyWorkerUnavailable
	if errors.As(err, &stickyUnavailable) {
		return true, false // drop the task
	}
	var invalid *serviceerror.InvalidArgument
	var internal *serviceerror.Internal
	if errors.As(err, &invalid) || errors.As(err, &internal) {
		tr.backlogMgr.throttledLogger.Error("nonretryable error processing spooled task", tag.Error(err))
		return true, false // drop the task
	}
	// For any other error (this should be very rare), we can retry.
	tr.backlogMgr.throttledLogger.Error("retryable error processing spooled task", tag.Error(err))
	return false, true
}

func (tr *fairTaskReader) retryAddAfterError(task *internalTask) {
	defer tr.addRetries.Release(1)
	metrics.BufferThrottlePerTaskQueueCounter.With(tr.backlogMgr.metricsHandler).Record(1)

	// initial sleep since we just tried once
	util.InterruptibleSleep(tr.backlogMgr.tqCtx, time.Second)

	_ = backoff.ThrottleRetryContext(
		tr.backlogMgr.tqCtx,
		func(context.Context) error {
			if IsTaskExpired(task.event.AllocatedTaskInfo) {
				task.finish(nil, false)
				return nil
			}
			err := tr.backlogMgr.addSpooledTask(task)
			if drop, retry := tr.addErrorBehavior(err); drop {
				task.finish(nil, false)
			} else if retry {
				metrics.BufferThrottlePerTaskQueueCounter.With(tr.backlogMgr.metricsHandler).Record(1)
				return err
			}
			return nil
		},
		addErrorRetryPolicy,
		nil,
	)
}

func (tr *fairTaskReader) signalNewTasks(tasks []*persistencespb.AllocatedTaskInfo) {
	tr.mergeTasks(tasks)
}

func (tr *fairTaskReader) mergeTasks(tasks []*persistencespb.AllocatedTaskInfo) {
	tr.lock.Lock()

	// Take the tasks in the buffer plus the tasks that were just written and sort them by level:

	// Get outstanding tasks. Note these values are *internalTask.
	merged := tr.outstandingTasks.Select(func(k, v any) bool {
		_, ok := v.(*internalTask)
		return ok
	})
	// Add the tasks we just wrote. Note these values are *AllocatedTaskInfo.
	for _, t := range tasks {
		level := allocatedTaskFairLevel(t)
		if _, have := merged.Get(level); have {
			// duplicate: we write something we just read, or read something we just wrote.
			// either way we have it in the buffer already. FIXME: is this right?
			continue
		}
		merged.Put(level, t)
	}

	// Take as many of those as we want to keep in memory. The ones that are not already in the
	// matcher, we have to add to the matcher.
	batchSize := tr.backlogMgr.config.GetTasksBatchSize()
	it := merged.Iterator()
	var lastLevel fairLevel
	tasks = tasks[:0]
	canBuffer := 0
	for b := 0; it.Next() && b < batchSize; b++ {
		lastLevel = it.Key().(fairLevel) // nolint:revive
		if t, ok := it.Value().(*persistencespb.AllocatedTaskInfo); ok {
			// new task we need to add to the matcher
			tasks = append(tasks, t)
		}
		canBuffer++
	}

	// Set read level to the maximum level in that set.
	tr.readLevel = lastLevel

	// If there are remaining tasks in the merged set, they can't fit in memory. If they came
	// from the tasks we just wrote, ignore them. If they came from matcher, remove them.
	toRemove := make([]*internalTask, 0, merged.Size()-canBuffer)
	for it.Next() {
		if task, ok := it.Value().(*internalTask); ok {
			// task that was in the matcher before that we have to remove
			tr.backlogAge.record(task.event.Data.CreateTime, -1)
			tr.loadedTasks--
			softassert.That(tr.logger, tr.loadedTasks >= 0, "loadedTasks went negative")
			tr.outstandingTasks.Remove(it.Key().(fairLevel))

			// do remove from matcher below
			toRemove = append(toRemove, task)
		}
	}

	internalTasks := make([]*internalTask, len(tasks))
	for i, t := range tasks {
		level := allocatedTaskFairLevel(t)
		internalTasks[i] = newInternalTaskFromBacklog(t, tr.completeTask)
		// After we get to this point, we must eventually call task.finish or
		// task.finishForwarded, which will call tr.completeTask.
		tr.outstandingTasks.Put(level, internalTasks[i])
		tr.loadedTasks++
		tr.backlogAge.record(t.Data.CreateTime, 1)
	}

	// unlock before calling addTaskToMatcher/removeSpooledTask
	tr.lock.Unlock()

	for _, task := range toRemove {
		task.removeFromMatcher()
	}

	for _, task := range internalTasks {
		tr.addTaskToMatcher(task)
	}
}

func (tr *fairTaskReader) backoffSignal(duration time.Duration) {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	if tr.backoffTimer == nil {
		tr.backoffTimer = time.AfterFunc(duration, func() {
			tr.lock.Lock()
			tr.backoffTimer = nil
			tr.lock.Unlock()

			tr.readTasks()
		})
	}
}

// ack manager

func (tr *fairTaskReader) getLoadedTasks() int {
	tr.lock.Lock()
	defer tr.lock.Unlock()
	return tr.loadedTasks
}

func (tr *fairTaskReader) ackTaskLocked(level fairLevel) {
	if task, found := tr.outstandingTasks.Get(level); !softassert.That(tr.logger, found, "completed task not found in oustandingTasks") {
		return
	} else if _, ok := task.(*internalTask); !softassert.That(tr.logger, ok, "completed task was already acked") {
		return
	}

	tr.outstandingTasks.Put(level, nil)
	tr.loadedTasks--

	tr.advanceAckLevelLocked()
}

func (tr *fairTaskReader) advanceAckLevelLocked() {
	if tr.ackLevelPinned {
		return
	}

	// Adjust the ack level as far as we can
	var numAcked int64
	for {
		minLevel, v := tr.outstandingTasks.Min()
		if minLevel == nil {
			break
		} else if _, ok := v.(*internalTask); ok {
			break
		}
		tr.ackLevel = minLevel.(fairLevel) // nolint:revive
		tr.outstandingTasks.Remove(minLevel)
		numAcked += 1
	}

	if numAcked > 0 {
		tr.numToGC += int(numAcked)
		tr.maybeGCLocked()

		tr.backlogMgr.db.updateFairAckLevelAndBacklogStats(tr.subqueue, tr.ackLevel, -numAcked, tr.backlogAge.oldestTime())
	}
}

func (tr *fairTaskReader) getAndPinAckLevel() fairLevel {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	softassert.That(tr.logger, !tr.ackLevelPinned, "ack level already pinned")
	tr.ackLevelPinned = true
	return tr.ackLevel
}

func (tr *fairTaskReader) unpinAckLevel() {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	softassert.That(tr.logger, tr.ackLevelPinned, "ack level wasn't pinned")
	tr.ackLevelPinned = false
	tr.advanceAckLevelLocked()
}

func (tr *fairTaskReader) getLevels() (readLevel, ackLevel fairLevel) {
	tr.lock.Lock()
	defer tr.lock.Unlock()
	return tr.readLevel, tr.ackLevel
}

// gc

func (tr *fairTaskReader) maybeGCLocked() {
	if !tr.shouldGCLocked() {
		return
	}
	tr.inGC = true
	tr.lastGCTime = time.Now()
	// gc in new goroutine so poller doesn't have to wait
	go tr.doGC(tr.ackLevel)
}

func (tr *fairTaskReader) shouldGCLocked() bool {
	if tr.inGC || tr.numToGC == 0 {
		return false
	}
	return tr.numToGC >= tr.backlogMgr.config.MaxTaskDeleteBatchSize() ||
		time.Since(tr.lastGCTime) > tr.backlogMgr.config.TaskDeleteInterval()
}

// called in new goroutine
func (tr *fairTaskReader) doGC(ackLevel fairLevel) {
	batchSize := tr.backlogMgr.config.MaxTaskDeleteBatchSize()

	ctx, cancel := context.WithTimeout(tr.backlogMgr.tqCtx, ioTimeout)
	defer cancel()

	n, err := tr.backlogMgr.db.CompleteFairTasksLessThan(ctx, ackLevel, batchSize, tr.subqueue)

	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.inGC = false
	if err != nil {
		return
	}
	// implementation behavior for CompleteTasksLessThan:
	// - unit test, cassandra: always return UnknownNumRowsAffected (in this case means "all")
	// - sql: return number of rows affected (should be <= batchSize)
	// if we get UnknownNumRowsAffected or a smaller number than our limit, we know we got
	// everything <= ackLevel, so we can reset ours. if not, we may have to try again.
	if n == persistence.UnknownNumRowsAffected {
		tr.numToGC = 0
	} else {
		tr.numToGC = max(0, tr.numToGC-n)
	}
}

func (l fairLevel) String() string {
	return fmt.Sprintf("<%d,%d>", l.pass, l.id)
}

func fairLevelLess(a, b fairLevel) bool {
	return a.pass < b.pass || a.pass == b.pass && a.id < b.id
}

func fairLevelComparator(aany, bany any) int {
	a := aany.(fairLevel) // nolint:revive
	b := bany.(fairLevel) // nolint:revive
	if fairLevelLess(a, b) {
		return -1
	} else if fairLevelLess(b, a) {
		return 1
	}
	return 0
}

func fairLevelMax(a, b fairLevel) fairLevel {
	if fairLevelLess(a, b) {
		return b
	}
	return a
}

func fairLevelPlusOne(a fairLevel) fairLevel {
	return fairLevel{pass: a.pass, id: a.id + 1}
}

func allocatedTaskFairLevel(t *persistencespb.AllocatedTaskInfo) fairLevel {
	return fairLevel{pass: t.PassNumber, id: t.TaskId}
}

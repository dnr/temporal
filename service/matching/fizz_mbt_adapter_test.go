// FizzBee model-based-testing adapter for the fair task queue.
//
// This drives the REAL fairBacklogManagerImpl (fairTaskReader +
// fairTaskWriter + taskQueueDB over testTaskManager) through the action
// sequences generated from docs/models/fairness_queue_fizzbee/mbt/
// queue_mbt.fizz, and reports the reader's internal state back for
// comparison against the spec after every action.
//
// Control points (see the spec header for the corresponding collapsed
// actions):
//   - gatedTaskManager parks GetTasks (reader) calls until the trace
//     releases them, and applies a pre-armed outcome (success / timeout,
//     applied or not) to the next CreateTasks call.
//   - the injected counter.Counter parks the writer in pickPasses (which
//     runs before CreateFairTasks and holds no locks — parking the store
//     write itself would hold the taskQueueDB mutex and block reads) and,
//     on release, returns exactly the spec-chosen level as the pass.
//   - AddSpooledTask captures *internalTask ("the matcher"); AckTask
//     finishes one; eviction is observed via setRemoveFunc.
//   - ExpireTask rewrites the stored task's ExpiryTime to the past.
//
// Between actions the SUT quiesces: every background goroutine is either
// parked at the gate or idle, which makes state comparison deterministic.

package matching

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mbt "github.com/fizzbee-io/fizzbee/mbt/lib/go"
	enumspb "go.temporal.io/api/enums/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/metrics/metricstest"
	"go.temporal.io/server/common/persistence"
	"go.temporal.io/server/common/primitives/timestamp"
	"go.temporal.io/server/common/testing/testlogger"
	"go.temporal.io/server/common/tqid"
	"go.temporal.io/server/service/matching/counter"
	"go.uber.org/mock/gomock"
)

const fizzMbtQuiesceTimeout = 10 * time.Second

var fizzActionStart atomic.Int64 // unix nanos; 0 = no action in flight
var fizzWatchdogOnce sync.Once

func fizzTimeAction(name string) func() {
	fizzWatchdogOnce.Do(func() {
		go func() {
			dumped := false
			for {
				time.Sleep(2 * time.Second)
				st := fizzActionStart.Load()
				if st != 0 && time.Since(time.Unix(0, st)) > 15*time.Second && !dumped {
					buf := make([]byte, 1<<20)
					n := runtime.Stack(buf, true)
					fmt.Printf("FIZZDBG STALL DUMP:\n%s\nFIZZDBG STALL DUMP END\n", buf[:n])
					dumped = true
				}
			}
		}()
	})
	fizzActionStart.Store(time.Now().UnixNano())
	return func() {
		fizzActionStart.Store(0)
	}
}

// ---------------------------------------------------------------------------
// gated persistence
// ---------------------------------------------------------------------------

type gateDecision struct {
	apply bool  // actually perform the operation on the underlying store
	err   error // error to return to the caller (after applying, if apply)
}

type gatedCall struct {
	decide  chan gateDecision
	readReq *persistence.GetTasksRequest // set for parked GetTasks calls
}

// gatedTaskManager parks GetTasks until the test releases it, and applies a
// pre-set one-shot outcome to the next CreateTasks call. Writes must NOT be
// parked here: taskQueueDB.CreateFairTasks holds the taskQueueDB mutex
// across the store call, and a completing read's merge needs that mutex
// (setKnownFairBacklogCount/updateFairAckLevel) while holding the reader
// lock — parking a write would block reads and task completions. The write
// is parked upstream instead, in the scripted counter (pickPasses runs
// before CreateFairTasks and holds no locks).
type gatedTaskManager struct {
	persistence.TaskManager
	mu            sync.Mutex
	parkedRead    *gatedCall
	nextWriteDcsn *gateDecision // consumed by the next CreateTasks call
}

func newGatedTaskManager(underlying persistence.TaskManager) *gatedTaskManager {
	return &gatedTaskManager{TaskManager: underlying}
}

func (g *gatedTaskManager) park(ctx context.Context, slot **gatedCall, readReq *persistence.GetTasksRequest) (gateDecision, error) {
	call := &gatedCall{decide: make(chan gateDecision, 1), readReq: readReq}
	g.mu.Lock()
	if *slot != nil {
		g.mu.Unlock()
		return gateDecision{}, errors.New("fizz mbt: second call parked on the same gate slot")
	}
	*slot = call
	g.mu.Unlock()

	select {
	case d := <-call.decide:
		return d, nil
	case <-ctx.Done():
		g.mu.Lock()
		if *slot == call {
			*slot = nil
		}
		g.mu.Unlock()
		return gateDecision{}, ctx.Err()
	}
}

func (g *gatedTaskManager) release(slot **gatedCall, d gateDecision) error {
	g.mu.Lock()
	call := *slot
	*slot = nil
	g.mu.Unlock()
	if call == nil {
		return errors.New("fizz mbt: no call parked on gate slot")
	}
	call.decide <- d
	return nil
}

// setNextWriteDecision arms the outcome for the next CreateTasks call.
func (g *gatedTaskManager) setNextWriteDecision(d gateDecision) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nextWriteDcsn = &d
}

func (g *gatedTaskManager) hasParkedRead() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.parkedRead != nil
}

// peekParkedReadReq returns the request of the parked GetTasks call, or nil.
func (g *gatedTaskManager) peekParkedReadReq() *persistence.GetTasksRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.parkedRead == nil {
		return nil
	}
	return g.parkedRead.readReq
}

func (g *gatedTaskManager) GetTasks(ctx context.Context, req *persistence.GetTasksRequest) (*persistence.GetTasksResponse, error) {
	d, err := g.park(ctx, &g.parkedRead, req)
	if err != nil {
		return nil, err
	}
	if d.err != nil {
		return nil, d.err
	}
	return g.TaskManager.GetTasks(ctx, req)
}

func (g *gatedTaskManager) CreateTasks(ctx context.Context, req *persistence.CreateTasksRequest) (*persistence.CreateTasksResponse, error) {
	g.mu.Lock()
	dp := g.nextWriteDcsn
	g.nextWriteDcsn = nil
	g.mu.Unlock()
	if dp == nil {
		return nil, errors.New("fizz mbt: unexpected CreateTasks call (no decision armed)")
	}
	d := *dp
	if d.apply {
		resp, err := g.TaskManager.CreateTasks(ctx, req)
		if d.err != nil {
			return nil, d.err // applied, but the response was "lost"
		}
		return resp, err
	}
	return nil, d.err
}

// ---------------------------------------------------------------------------
// scripted counter: pickPasses returns exactly the level the trace chose
// ---------------------------------------------------------------------------

// fizzScriptedCounter is where a write parks: pickPasses (fairTaskWriter)
// calls GetPass before CreateFairTasks, holding no locks, so blocking here
// leaves reads and completions fully operational. The release value is the
// pass (spec level) the trace chose for this write.
type fizzScriptedCounter struct {
	mu      sync.Mutex
	armed   bool
	parked  bool
	release chan int64
}

func newFizzScriptedCounter() *fizzScriptedCounter {
	return &fizzScriptedCounter{release: make(chan int64)}
}

func (c *fizzScriptedCounter) GetPass(key string, base, inc int64) int64 {
	c.mu.Lock()
	if !c.armed {
		// e.g. top-K replay on counter creation: keep the code's invariant
		c.mu.Unlock()
		return base
	}
	c.armed = false
	c.parked = true
	c.mu.Unlock()
	pass := <-c.release
	c.mu.Lock()
	c.parked = false
	c.mu.Unlock()
	if pass < base {
		return base // defensive; the adapter guards enabledness upstream
	}
	return pass
}

func (c *fizzScriptedCounter) EstimateDistinctKeys() int { return 1 }
func (c *fizzScriptedCounter) TopK() []counter.TopKEntry { return nil }

func (c *fizzScriptedCounter) arm() {
	c.mu.Lock()
	c.armed = true
	c.mu.Unlock()
}

func (c *fizzScriptedCounter) isParked() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.parked
}

var _ counter.Counter = (*fizzScriptedCounter)(nil)

// ---------------------------------------------------------------------------
// the model / role adapter
// ---------------------------------------------------------------------------

type fizzQueueMbtModel struct {
	t *testing.T
	q *fizzQueueRole
}

func newFizzQueueMbtModel(t *testing.T) *fizzQueueMbtModel {
	return &fizzQueueMbtModel{t: t}
}

var _ QueueMbtModel = (*fizzQueueMbtModel)(nil)

func (m *fizzQueueMbtModel) GetState() (map[string]any, error) { return nil, mbt.ErrNotImplemented }

func (m *fizzQueueMbtModel) GetRoles() (map[mbt.RoleId]mbt.Role, error) {
	return map[mbt.RoleId]mbt.Role{{RoleName: "Queue", Index: 0}: m.q}, nil
}

func (m *fizzQueueMbtModel) Init() error {
	q, err := newFizzQueueRole(m.t)
	if err != nil {
		return err
	}
	m.q = q
	return nil
}

func (m *fizzQueueMbtModel) Cleanup() error {
	if m.q != nil {
		m.q.stop()
		m.q = nil
	}
	return nil
}

type fizzQueueRole struct {
	t        *testing.T
	taskMgr  *testTaskManager
	gate     *gatedTaskManager
	blm      *fairBacklogManagerImpl
	queueKey *PhysicalTaskQueueKey
	cancel   context.CancelFunc
	counter  *fizzScriptedCounter

	mu         sync.Mutex
	captured   map[fairLevel]*internalTask // "the matcher": dispatched, not completed
	confirmed  map[int64]bool              // ghosts (pass values)
	everAcked  map[int64]bool
	expired    map[int64]bool
	pendingWriteLevel int64 // spec write_req; -1 if none
	spoolDone  chan error   // result of the in-flight SpoolTask, if any
}

var _ QueueRole = (*fizzQueueRole)(nil)

func newFizzQueueRole(t *testing.T) (*fizzQueueRole, error) {
	r := &fizzQueueRole{
		t:                 t,
		captured:          make(map[fairLevel]*internalTask),
		confirmed:         make(map[int64]bool),
		everAcked:         make(map[int64]bool),
		expired:           make(map[int64]bool),
		pendingWriteLevel: -1,
		counter:           newFizzScriptedCounter(),
	}

	controller := gomock.NewController(t)
	logger := testlogger.NewTestLogger(t, testlogger.FailOnAnyUnexpectedError)
	// timed-out writes are logged as errors by writeBatch
	logger.Expect(testlogger.Error, "Persistent store operation failure")

	r.taskMgr = newTestFairTaskManager(logger)
	r.gate = newGatedTaskManager(r.taskMgr)

	cfgcli := dynamicconfig.NewMemoryClient()
	// must match the constants in queue_mbt.fizz
	cfgcli.OverrideValue(dynamicconfig.MatchingGetTasksBatchSize.Key(), 2)
	cfgcli.OverrideValue(dynamicconfig.MatchingGetTasksReloadAt.Key(), 1)
	// one task per write batch, so one spec-level == one CreateTasks call
	cfgcli.OverrideValue(dynamicconfig.MatchingMaxTaskBatchSize.Key(), 1)
	// suppress GC and periodic syncs (not modeled)
	cfgcli.OverrideValue(dynamicconfig.MatchingMaxTaskDeleteBatchSize.Key(), 1<<30)
	cfgcli.OverrideValue(dynamicconfig.MatchingTaskDeleteInterval.Key(), 24*time.Hour)
	cfgcli.OverrideValue(dynamicconfig.MatchingUpdateAckInterval.Key(), 24*time.Hour)
	cfgcol := dynamicconfig.NewCollection(cfgcli, logger)

	f, err := tqid.NewTaskQueueFamily("", "fizz-mbt-queue")
	if err != nil {
		return nil, err
	}
	prtn := f.TaskQueue(enumspb.TASK_QUEUE_TYPE_WORKFLOW).NormalPartition(0)
	r.queueKey = UnversionedQueueKey(prtn)
	tlCfg := newTaskQueueConfig(prtn.TaskQueue(), NewConfig(cfgcol), "fizz-mbt-namespace")

	ptqMgr := NewMockphysicalTaskQueueManager(controller)
	ptqMgr.EXPECT().QueueKey().Return(r.queueKey).AnyTimes()
	ptqMgr.EXPECT().GetFairnessWeightOverrides().AnyTimes().Return(fairnessWeightOverrides{})
	ptqMgr.EXPECT().StartScaleManager(gomock.Any()).AnyTimes()
	ptqMgr.EXPECT().UnloadFromPartitionManager(gomock.Any()).AnyTimes()
	ptqMgr.EXPECT().AddSpooledTask(gomock.Any()).DoAndReturn(func(task *internalTask) error {
		lvl := task.fairLevel()
		r.mu.Lock()
		defer r.mu.Unlock()
		if !task.setRemoveFunc(func() {
			// task evicted from the "matcher" by the reader
			r.mu.Lock()
			delete(r.captured, lvl)
			r.mu.Unlock()
		}) {
			return nil // already evicted before it reached the matcher
		}
		r.captured[lvl] = task
		return nil
	}).AnyTimes()

	var ctx context.Context
	ctx, r.cancel = context.WithCancel(context.Background())

	r.blm = newFairBacklogManager(
		ctx,
		ptqMgr,
		tlCfg,
		r.gate,
		logger,
		logger,
		nil,
		metricstest.NewCaptureHandler(),
		func() counter.Counter { return r.counter },
		false,
	)
	r.blm.Start()
	if err := r.blm.WaitUntilInitialized(ctx); err != nil {
		return nil, err
	}
	// the reader immediately issues its first read, which parks at the gate
	if err := r.quiesce(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *fizzQueueRole) stop() {
	r.blm.skipFinalUpdate.Store(true)
	if r.counter.isParked() {
		// unstick the writer goroutine; the armed decision fails the write
		r.gate.setNextWriteDecision(gateDecision{apply: false, err: context.Canceled})
		r.mu.Lock()
		level := r.pendingWriteLevel
		r.mu.Unlock()
		if level < 1 {
			level = 1
		}
		r.counter.release <- level
		<-r.spoolDone
	}
	r.cancel() // unparks any gated reads with context.Canceled
	r.blm.Stop()
}

func (r *fizzQueueRole) reader() *fairTaskReader {
	r.blm.subqueueLock.Lock()
	defer r.blm.subqueueLock.Unlock()
	return r.blm.subqueues[subqueueZero]
}

// quiesce waits until all SUT goroutines are parked at the gate or idle:
// the reader either has a GetTasks call parked or readPending == false, and
// there is no half-processed write (writes are awaited by their action).
func (r *fizzQueueRole) quiesce() error {
	tr := r.reader()
	deadline := time.Now().Add(fizzMbtQuiesceTimeout)
	for {
		if r.gate.hasParkedRead() {
			return nil
		}
		tr.lock.Lock()
		pending := tr.readPending
		tr.lock.Unlock()
		if !pending {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("fizz mbt: reader did not quiesce")
		}
		time.Sleep(200 * time.Microsecond)
	}
}

// ---------------------------------------------------------------------------
// actions
// ---------------------------------------------------------------------------

func argInt(args []mbt.Arg, name string) (int64, error) {
	for _, a := range args {
		if a.Name == name {
			if v, ok := a.Value.(int); ok {
				return int64(v), nil
			}
			return 0, fmt.Errorf("fizz mbt: arg %q has type %T", name, a.Value)
		}
	}
	return 0, fmt.Errorf("fizz mbt: missing arg %q in %v", name, args)
}

func argBool(args []mbt.Arg, name string) (bool, error) {
	for _, a := range args {
		if a.Name == name {
			if v, ok := a.Value.(bool); ok {
				return v, nil
			}
			return false, fmt.Errorf("fizz mbt: arg %q has type %T", name, a.Value)
		}
	}
	return false, fmt.Errorf("fizz mbt: missing arg %q in %v", name, args)
}

func (r *fizzQueueRole) ActionWriterWrite(args []mbt.Arg) (any, error) {
	defer fizzTimeAction("ActionWriterWrite")()
	level, err := argInt(args, "t")
	if err != nil {
		return nil, err
	}
	// reject spec-disabled invocations WITHOUT side effects (the runner
	// fires random actions; trace validation skips them, so the SUT state
	// must not change)
	r.mu.Lock()
	pending := r.pendingWriteLevel
	r.mu.Unlock()
	if pending != -1 || r.counter.isParked() {
		return nil, errors.New("fizz mbt: write already in flight (disabled action)")
	}
	tr := r.reader()
	tr.lock.Lock()
	ackPass := tr.ackLevel.pass
	tr.lock.Unlock()
	if level <= ackPass {
		return nil, errors.New("fizz mbt: level at or below ack level (disabled action)")
	}
	if r.dbHasLevel(level) {
		return nil, errors.New("fizz mbt: level already in db (disabled action)")
	}
	r.counter.arm()
	r.mu.Lock()
	r.pendingWriteLevel = level
	r.mu.Unlock()
	r.spoolDone = make(chan error, 1)
	go func() {
		r.spoolDone <- r.blm.SpoolTask(&persistencespb.TaskInfo{
			NamespaceId: r.queueKey.NamespaceId(),
			CreateTime:  timestamp.TimeNowPtrUtc(),
			ExpiryTime:  timestamp.TimeNowPtrUtcAddSeconds(3600),
		})
	}()
	// the write is "in flight" once the writer goroutine parks in GetPass
	// (ack level already pinned by writeBatch at this point)
	deadline := time.Now().Add(fizzMbtQuiesceTimeout)
	for !r.counter.isParked() {
		if time.Now().After(deadline) {
			return nil, errors.New("fizz mbt: write did not reach the counter gate")
		}
		time.Sleep(200 * time.Microsecond)
	}
	return nil, nil
}

func (r *fizzQueueRole) ActionWriterWriteOk(args []mbt.Arg) (any, error) {
	defer fizzTimeAction("ActionWriterWriteOk")()
	r.mu.Lock()
	level := r.pendingWriteLevel
	r.mu.Unlock()
	if level == -1 || !r.counter.isParked() {
		return nil, errors.New("fizz mbt: no write in flight (disabled action)")
	}
	r.gate.setNextWriteDecision(gateDecision{apply: true})
	r.counter.release <- level
	if err := <-r.spoolDone; err != nil {
		return nil, fmt.Errorf("fizz mbt: SpoolTask failed on ok-write: %w", err)
	}
	r.mu.Lock()
	r.confirmed[r.pendingWriteLevel] = true
	r.pendingWriteLevel = -1
	r.mu.Unlock()
	return nil, r.quiesce()
}

func (r *fizzQueueRole) ActionWriterWriteTimeout(args []mbt.Arg) (any, error) {
	defer fizzTimeAction("ActionWriterWriteTimeout")()
	applied, err := argBool(args, "applied")
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	level := r.pendingWriteLevel
	r.mu.Unlock()
	if level == -1 || !r.counter.isParked() {
		return nil, errors.New("fizz mbt: no write in flight (disabled action)")
	}
	r.gate.setNextWriteDecision(gateDecision{apply: applied, err: context.DeadlineExceeded})
	r.counter.release <- level
	if err := <-r.spoolDone; err == nil {
		return nil, errors.New("fizz mbt: SpoolTask unexpectedly succeeded on timed-out write")
	}
	r.mu.Lock()
	r.pendingWriteLevel = -1
	r.mu.Unlock()
	return nil, r.quiesce()
}

func (r *fizzQueueRole) ActionDbReadOk(args []mbt.Arg) (any, error) {
	defer fizzTimeAction("ActionDbReadOk")()
	if err := r.gate.release(&r.gate.parkedRead, gateDecision{apply: true}); err != nil {
		return nil, err
	}
	return nil, r.quiesce()
}

func (r *fizzQueueRole) ActionAckTask(args []mbt.Arg) (any, error) {
	defer fizzTimeAction("ActionAckTask")()
	level, err := argInt(args, "t")
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	var task *internalTask
	for lvl, t := range r.captured {
		if lvl.pass == level {
			task = t
			delete(r.captured, lvl)
			break
		}
	}
	r.mu.Unlock()
	if task == nil {
		return nil, fmt.Errorf("fizz mbt: no dispatched task at level %d to ack", level)
	}
	r.mu.Lock()
	r.everAcked[level] = true
	r.mu.Unlock()
	task.finish(taskFinishResult{consumedToken: true}) // synchronous completeTask
	return nil, r.quiesce()
}

func (r *fizzQueueRole) ActionExpireTask(args []mbt.Arg) (any, error) {
	defer fizzTimeAction("ActionExpireTask")()
	level, err := argInt(args, "t")
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.everAcked[level] || r.expired[level] {
		r.mu.Unlock()
		return nil, errors.New("fizz mbt: task acked or already expired (disabled action)")
	}
	r.mu.Unlock()
	tqd := r.taskMgr.getQueueDataByKey(r.queueKey)
	tqd.Lock()
	found := false
	it := tqd.tasks.Iterator()
	for it.Next() {
		task := it.Value().(*persistencespb.AllocatedTaskInfo)
		if task.TaskPass == level {
			task.Data.ExpiryTime = timestamp.TimePtr(time.Now().UTC().Add(-time.Hour))
			found = true
			break
		}
	}
	tqd.Unlock()
	if !found {
		return nil, fmt.Errorf("fizz mbt: no stored task at level %d to expire", level)
	}
	r.mu.Lock()
	r.expired[level] = true
	r.mu.Unlock()
	return nil, nil
}

func (r *fizzQueueRole) dbHasLevel(level int64) bool {
	tqd := r.taskMgr.getQueueDataByKey(r.queueKey)
	tqd.Lock()
	defer tqd.Unlock()
	it := tqd.tasks.Iterator()
	for it.Next() {
		if it.Key().(fairLevel).pass == level {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// state
// ---------------------------------------------------------------------------

func sortedInts(m map[int64]bool) any {
	out := []any{}
	for k := range m {
		out = append(out, int(k))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].(int) < out[j].(int) })
	return out
}

func (r *fizzQueueRole) GetState() (map[string]any, error) {
	tr := r.reader()

	tr.lock.Lock()
	outstanding := map[any]any{}
	it := tr.outstandingTasks.Iterator()
	for it.Next() {
		lvl := it.Key().(fairLevel)
		status := "ACKED"
		if _, ok := it.Value().(*internalTask); ok {
			status = "LOADED"
		}
		outstanding[strconv.FormatInt(lvl.pass, 10)] = status
	}
	readLevel := int(tr.readLevel.pass)
	ackLevel := int(tr.ackLevel.pass)
	atEnd := tr.atEnd
	pinned := tr.ackLevelPinnedByWriter
	newlyWritten := []any{}
	for _, t := range tr.newlyWrittenTasks {
		newlyWritten = append(newlyWritten, int(t.TaskPass))
	}
	sort.Slice(newlyWritten, func(i, j int) bool { return newlyWritten[i].(int) < newlyWritten[j].(int) })
	evictedAcks := []any{}
	tr.evictedAcks.Scan(func(lvl fairLevel) bool {
		evictedAcks = append(evictedAcks, int(lvl.pass))
		return true
	})
	readParked := r.gate.hasParkedRead()
	tr.lock.Unlock()

	// db rows (levels present in the fake store)
	db := []any{}
	tqd := r.taskMgr.getQueueDataByKey(r.queueKey)
	tqd.Lock()
	dit := tqd.tasks.Iterator()
	for dit.Next() {
		db = append(db, int(dit.Key().(fairLevel).pass))
	}
	tqd.Unlock()
	sort.Slice(db, func(i, j int) bool { return db[i].(int) < db[j].(int) })

	// pending read request: while a GetTasks call is parked, readLevel can't
	// change, so the captured exclusive-min == the current readLevel; the
	// batch size is BATCH_SIZE - loadedTasks at capture time, which is what
	// the spec computed when it issued the read. Rather than re-deriving it
	// from the parked request proto, mirror the spec's bookkeeping.
	// While a GetTasks call is parked, readLevel cannot change (the read
	// loop owns it and mergeWrite is buffered while readPending), so the
	// spec's captured read_from == the current readLevel. The batch size is
	// taken from the parked request itself (loadedTasks may have changed
	// since capture).
	readFrom, readBatch := -1, 0
	if req := r.gate.peekParkedReadReq(); req != nil && readParked {
		readFrom = readLevel
		readBatch = req.PageSize
	}

	r.mu.Lock()
	writeReq := int(r.pendingWriteLevel)
	confirmed := sortedInts(r.confirmed)
	everAcked := sortedInts(r.everAcked)
	expired := sortedInts(r.expired)
	r.mu.Unlock()

	return map[string]any{
		"db":            db,
		"outstanding":   outstanding,
		"read_level":    readLevel,
		"ack_level":     ackLevel,
		"at_end":        atEnd,
		"newly_written": newlyWritten,
		"pinned":        pinned,
		"evicted_acks":  evictedAcks,
		"read_from":     readFrom,
		"read_batch":    readBatch,
		"write_req":     writeReq,
		"confirmed":     confirmed,
		"ever_acked":    everAcked,
		"expired":       expired,
	}, nil
}

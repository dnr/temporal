package matching

import (
	"container/list"
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/metrics/metricstest"
	testutil "go.temporal.io/server/common/testing"
	"go.temporal.io/server/common/testing/testlogger"
	"go.temporal.io/server/common/util"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// These tests try to reproduce the production "stuck reader" symptom: a partition builds up a
// backlog of ~GetTasksBatchSize tasks from nothing, most partitions drain it, but one ends up stuck
// (atEnd=false, loadedTasks=0, tasks still in the DB, nothing scheduling a read). The pattern that
// triggered it in prod was bursty: large add bursts from zero with little draining, which forces
// heavy eviction, followed by a full drain. We model that with a square-wave target (fill to a high
// watermark, then drain to zero) repeated for many cycles, with multiple writer/poller goroutines, a
// skewed fairness-key distribution (to provoke read-level drops and ack eviction), and delay/fault
// injection (to provoke concurrent read/write and retry/backoff paths).
//
// Detection: after each drain phase we wait for inflight to reach zero. If it doesn't within
// drainTimeout, the reader is stuck/stalled; we dump every subqueue reader's state and fail. We also
// log the eviction and stuck-detector metrics across the run.

type burstBacklogParams struct {
	burstHigh    int64         // fill the backlog to this many inflight tasks before draining
	cycles       int           // number of fill/drain cycles; if 0, run until duration elapses
	duration     time.Duration // wall-clock budget when cycles == 0
	drainTimeout time.Duration // max time to drain one cycle to zero before declaring stuck
	dwell        time.Duration // time to hold at the peak (churning eviction) before draining
	gap          int64         // writers/pollers chase target within this gap
	writers      int
	pollers      int
	keys         int
	zipfS, zipfV float64
	seed         int64 // 0 => time-based (nondeterministic, good for fuzzing)
	cfg          map[dynamicconfig.Key]any

	delayInjection time.Duration
	faultInjection float32

	// recover mode: enable ForceReadTasksOnWrite so a stuck reader is woken by a (prober) write
	// instead of hanging. This makes stalls non-fatal so the test runs the full duration and we can
	// *count* how many times the reader entered the stuck state (FairReaderStuckDetected) rather
	// than failing on the first rare hit. Used to compare stuck frequency before/after a fix.
	recover bool
}

var defaultBurstBacklogParams = burstBacklogParams{
	burstHigh:    600,
	cycles:       3,
	drainTimeout: 30 * time.Second,
	dwell:        50 * time.Millisecond,
	gap:          2,
	writers:      3,
	pollers:      2,
	keys:         50,
	zipfS:        2,
	zipfV:        1,
	cfg: map[dynamicconfig.Key]any{
		dynamicconfig.MatchingGetTasksBatchSize.Key(): 100,
		dynamicconfig.MatchingGetTasksReloadAt.Key():  40,
		dynamicconfig.MatchingMaxTaskBatchSize.Key():  50,
	},
	delayInjection: 1 * time.Millisecond,
	faultInjection: 0.02,
}

// TestBurstBacklog_Short is a quick smoke run (a few cycles).
func (s *BacklogManagerTestSuite) TestBurstBacklog_Short() {
	s.testBurstBacklog(defaultBurstBacklogParams)
}

// TestBurstBacklog_Long runs many bursty cycles for a few minutes; closest to the prod pattern.
//
// For an overnight run, suppress the per-op debug logging and lift timeouts, e.g.:
//
//	TEMPORAL_TEST_LONG=1 TEMPORAL_TEST_LOG_LEVEL=error BURST_DURATION=8h BURST_SEED=0 \
//	  go test ./service/matching/ -run \
//	  'TestBacklogManager_Fair_Suite/TestBurstBacklog_Long' -count=1 -timeout 0
//
// On a stall it dumps every subqueue reader's state and fails fast, so the failure output isn't
// buried. BURST_SEED pins the RNG (0 = time-based); the chosen seed is logged so a hit can be
// replayed.
func (s *BacklogManagerTestSuite) TestBurstBacklog_Long() {
	testutil.LongTest(s)
	p := defaultBurstBacklogParams
	p.burstHigh = 1500
	p.cycles = 0
	p.duration = 3 * time.Minute
	if d := os.Getenv("BURST_DURATION"); d != "" {
		parsed, err := time.ParseDuration(d)
		s.Require().NoError(err, "invalid BURST_DURATION")
		p.duration = parsed
	}
	if v := os.Getenv("BURST_SEED"); v != "" {
		seed, err := strconv.ParseInt(v, 10, 64)
		s.Require().NoError(err, "invalid BURST_SEED")
		p.seed = seed
	}
	s.testBurstBacklog(p)
}

// TestBurstBacklog_Count runs the bursty workload with ForceReadTasksOnWrite enabled so a stuck
// reader is woken by a prober write instead of hanging. It then *counts* how often the reader
// entered the stuck state and asserts that count is zero. Pre-fix this fails with a positive count;
// after the evicted-ack fix it should pass. This is the before/after fix-validation test, and is far
// more reliable than waiting for the rare fatal stall. Tune duration via BURST_DURATION.
func (s *BacklogManagerTestSuite) TestBurstBacklog_Count() {
	testutil.LongTest(s)
	p := defaultBurstBacklogParams
	p.burstHigh = 1500
	p.cycles = 0
	p.duration = 3 * time.Minute
	p.recover = true
	if d := os.Getenv("BURST_DURATION"); d != "" {
		parsed, err := time.ParseDuration(d)
		s.Require().NoError(err, "invalid BURST_DURATION")
		p.duration = parsed
	}
	if v := os.Getenv("BURST_SEED"); v != "" {
		seed, err := strconv.ParseInt(v, 10, 64)
		s.Require().NoError(err, "invalid BURST_SEED")
		p.seed = seed
	}
	s.testBurstBacklog(p)
}

// TestBurstBacklog_HeavyFault leans on retries/backoff (a prime suspect for atEnd=false getting
// stuck) with a higher fault rate and bigger bursts.
func (s *BacklogManagerTestSuite) TestBurstBacklog_HeavyFault() {
	testutil.LongTest(s)
	p := defaultBurstBacklogParams
	p.burstHigh = 1200
	p.cycles = 0
	p.duration = 3 * time.Minute
	p.faultInjection = 0.05
	p.delayInjection = 2 * time.Millisecond
	s.testBurstBacklog(p)
}

func (s *BacklogManagerTestSuite) testBurstBacklog(p burstBacklogParams) {
	if !s.newMatcher && !s.fairness {
		s.T().Skip("burst backlog test is for priority + fairness backlog manager only")
	}

	seed := p.seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	s.T().Logf("burst backlog: seed=%d burstHigh=%d writers=%d pollers=%d keys=%d zipfS=%.2f fault=%.3f",
		seed, p.burstHigh, p.writers, p.pollers, p.keys, p.zipfS, p.faultInjection)
	rng := rand.New(rand.NewSource(seed))
	var zipfLock sync.Mutex
	zipf := rand.NewZipf(rng, p.zipfS, p.zipfV, uint64(p.keys-1))
	nextKey := func() uint64 {
		zipfLock.Lock()
		defer zipfLock.Unlock()
		return zipf.Uint64()
	}

	for k, v := range p.cfg {
		s.cfgcli.OverrideValue(k, v)
	}
	if p.recover {
		s.cfgcli.OverrideValue(dynamicconfig.MatchingForceReadTasksOnWrite.Key(), true)
	}

	s.taskMgr.delayInjection = p.delayInjection
	if p.faultInjection > 0 {
		s.taskMgr.addFault("GetTasks", "Unavailable", p.faultInjection)
		s.taskMgr.addFault("CreateTasks", "Unavailable", p.faultInjection)
		s.logger.Expect(testlogger.Error, "Persistent store operation failure")
	}

	overall := p.duration
	if p.cycles > 0 {
		overall = time.Duration(p.cycles)*(p.drainTimeout+5*time.Second) + 30*time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), overall+30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var lock sync.Mutex
	var tasks list.List // in-memory buffer (mock for the matcher)
	var target, inflight, processed, index, duplicates, stalls, probes atomic.Int64
	var tracker sync.Map
	const testIsOver = int64(-1000000)
	target.Store(0)

	s.ptqMgr.EXPECT().AddSpooledTask(gomock.Any()).DoAndReturn(func(t *internalTask) error {
		lock.Lock()
		defer lock.Unlock()
		e := tasks.PushBack(t)
		if !t.setRemoveFunc(func() {
			lock.Lock()
			defer lock.Unlock()
			tasks.Remove(e)
		}) {
			tasks.Remove(e)
			return nil
		}
		return nil
	}).AnyTimes()

	getTask := func() *internalTask {
		lock.Lock()
		defer lock.Unlock()
		e := tasks.Front()
		if e == nil {
			return nil
		}
		return tasks.Remove(e).(*internalTask)
	}
	makeNewTask := func() *persistencespb.TaskInfo {
		return &persistencespb.TaskInfo{
			CreateTime:       timestamppb.Now(),
			ScheduledEventId: index.Add(1),
			Priority: &commonpb.Priority{
				FairnessKey: fmt.Sprintf("fkey-%02d", nextKey()),
			},
		}
	}
	delta := func() int64 { return inflight.Load() - target.Load() }
	sleep := func() {
		d := time.Millisecond + time.Duration(rng.Float32()*float32(3*time.Millisecond))
		_ = util.InterruptibleSleep(ctx, d)
	}
	finished := func() bool {
		return ctx.Err() != nil || (target.Load() == testIsOver && inflight.Load() == 0)
	}
	sleepUntil := func(cond func() bool) bool {
		for !finished() && !cond() {
			sleep()
		}
		return !finished()
	}

	capture := s.metricsCap.StartCapture()
	defer s.metricsCap.StopCapture(capture)

	start := time.Now()
	s.blm.Start()
	defer s.blm.Stop()
	s.NoError(s.blm.WaitUntilInitialized(context.Background()))
	blm := s.blm.(*fairBacklogManagerImpl)

	// spoolOne writes one tracked task. Used by the writer goroutines and (in recover mode) by the
	// drain prober to wake a stuck reader via the write path.
	spoolOne := func() bool {
		info := makeNewTask()
		tracker.Store(info.ScheduledEventId, info.Priority.FairnessKey)
		inflight.Add(1)
		if s.blm.SpoolTask(info) != nil {
			tracker.Delete(info.ScheduledEventId)
			inflight.Add(-1)
			return false
		}
		return true
	}

	// drainToZero polls the backlog down to zero. In recover mode, if inflight stops making progress
	// it spools a prober task (which, with ForceReadTasksOnWrite, wakes a stuck reader) and keeps
	// going. In non-recover mode, a stall past drainTimeout is treated as a stuck reader: dump and
	// fail. Returns false only if ctx expired first.
	drainToZero := func() bool {
		deadline := time.Now().Add(p.drainTimeout)
		var last int64 = -1
		lastChange := time.Now()
		for ctx.Err() == nil {
			cur := inflight.Load()
			if cur == 0 {
				return true
			}
			if cur != last {
				last = cur
				lastChange = time.Now()
				deadline = time.Now().Add(p.drainTimeout)
			}
			if p.recover {
				if time.Since(lastChange) > 2*time.Second {
					stalls.Add(1)
					if spoolOne() { // wake the (possibly stuck) reader; ForceReadTasksOnWrite -> read
						probes.Add(1)
					}
					lastChange = time.Now()
				}
			} else if time.Now().After(deadline) {
				s.T().Logf("STUCK: inflight=%d did not drain in %v (last change %v ago); seed=%d",
					cur, p.drainTimeout, time.Since(lastChange), seed)
				s.dumpBurstState(blm, &tracker, capture)
				s.FailNow("backlog did not drain after burst cycle: likely stuck reader")
			}
			_ = util.InterruptibleSleep(ctx, 25*time.Millisecond)
		}
		return false
	}

	// writers: add tasks while we're below target
	for range p.writers {
		wg.Go(func() {
			for sleepUntil(func() bool { return delta() <= p.gap }) {
				if !spoolOne() {
					sleep()
				}
			}
		})
	}

	// pollers: drain tasks while we're above target
	for range p.pollers {
		wg.Go(func() {
			for sleepUntil(func() bool { return delta() >= -p.gap }) {
				if t := getTask(); t != nil {
					t.finish(taskFinishResult{consumedToken: true})
					tindex := t.event.Data.ScheduledEventId
					if _, loaded := tracker.LoadAndDelete(tindex); loaded {
						inflight.Add(-1)
					} else {
						duplicates.Add(1)
					}
					processed.Add(1)
				} else {
					sleep()
				}
			}
		})
	}

	// drive the square wave: fill to burstHigh, dwell, drain to 0, verify drained.
	cycle := 0
	lastProgress := start
	for {
		if ctx.Err() != nil {
			break
		}
		if p.cycles > 0 && cycle >= p.cycles {
			break
		}
		if p.cycles == 0 && time.Since(start) >= p.duration {
			break
		}
		cycle++

		// FILL: writers race up to burstHigh; pollers stay mostly idle (delta-gated).
		target.Store(p.burstHigh)
		if !sleepUntil(func() bool { return inflight.Load() >= p.burstHigh-p.gap }) {
			break // ctx done
		}
		_ = util.InterruptibleSleep(ctx, p.dwell) // churn eviction at the peak

		// DRAIN: writers stop, pollers empty the backlog (recover mode probes a stuck reader).
		target.Store(0)
		if !drainToZero() {
			break // ctx done
		}
		// throttle progress logging so overnight runs don't buffer huge t.Log output.
		if ctx.Err() == nil && (p.cycles > 0 || time.Since(lastProgress) >= 15*time.Second) {
			lastProgress = time.Now()
			s.T().Logf("cycle %d ok (%.0fs elapsed, processed %d, stalls %d, dups %d)",
				cycle, time.Since(start).Seconds(), processed.Load(), stalls.Load(), duplicates.Load())
		}
	}

	// final drain: bring inflight to zero (probing in recover mode) before stopping the goroutines.
	s.T().Log("final drain")
	drainToZero()
	target.Store(testIsOver)
	wg.Wait()

	if !s.Zero(inflight.Load(), "did not drain all tasks at end of test") {
		s.dumpBurstState(blm, &tracker, capture)
	}
	s.logBurstMetrics(capture, processed.Load(), duplicates.Load(), time.Since(start))

	if p.recover {
		var stuck int64
		for _, r := range capture.Snapshot()[metrics.FairReaderStuckDetected.Name()] {
			stuck += r.Value.(int64)
		}
		s.T().Logf("recover mode: cycles=%d stalls=%d probes=%d stuckDetected=%d (seed=%d)",
			cycle, stalls.Load(), probes.Load(), stuck, seed)
		// stuckDetected counts transient entries into the stuck state. With the write-path read
		// trigger, each is recovered in the same critical section, so the queue must never actually
		// stall (the prober must never need to fire) and must fully drain. We don't assert
		// stuckDetected==0: the detector fires before recovery, so transient entries are expected.
		s.Zero(stalls.Load(), "drain stalled despite write-path recovery: needed %d prober writes", stalls.Load())
	}
}

// dumpBurstState logs every subqueue reader's internal state plus DB counts and outstanding tasks,
// so we can see exactly what "stuck" looks like when we catch it.
func (s *BacklogManagerTestSuite) dumpBurstState(blm *fairBacklogManagerImpl, tracker *sync.Map, capture *metricstest.Capture) {
	blm.subqueueLock.Lock()
	readers := append([]*fairTaskReader(nil), blm.subqueues...)
	blm.subqueueLock.Unlock()
	db := blm.getDB()
	for i, tr := range readers {
		tr.lock.Lock()
		readLevel, ackLevel := tr.readLevel, tr.ackLevel
		loaded, atEnd := tr.loadedTasks, tr.atEnd
		readPending, hasBackoff := tr.readPending, tr.backoffTimer != nil
		evicted := tr.evictedAcks.Len()
		newlyWritten := len(tr.newlyWrittenTasks)
		ackPinned := tr.ackLevelPinnedByWriter
		tr.lock.Unlock()
		dbCount, dbMaxRead := db.getApproximateBacklogCountAndMaxReadLevel(subqueueIndex(i))
		s.T().Logf("  subqueue %d: read=%v ack=%v loaded=%d atEnd=%v readPending=%v backoff=%v "+
			"evictedAcks=%d newlyWritten=%d ackPinned=%v | db: approxCount=%d maxRead=%v",
			i, readLevel, ackLevel, loaded, atEnd, readPending, hasBackoff,
			evicted, newlyWritten, ackPinned, dbCount, dbMaxRead)
	}
	n := 0
	tracker.Range(func(k, v any) bool {
		if n < 30 {
			s.T().Logf("  outstanding task: id=%d key=%s", k.(int64), v.(string))
		}
		n++
		return true
	})
	s.T().Logf("  total outstanding (tracker): %d", n)
	s.logBurstMetrics(capture, 0, 0, 0)
}

func (s *BacklogManagerTestSuite) logBurstMetrics(capture *metricstest.Capture, processed, duplicates int64, elapsed time.Duration) {
	snap := capture.Snapshot()
	sum := func(name string) int64 {
		var t int64
		for _, r := range snap[name] {
			t += r.Value.(int64)
		}
		return t
	}
	s.T().Logf("metrics: evictedTasks=%d evictedAcks=%d reinsertedAcks=%d stuckDetected=%d duplicates=%d processed=%d (%.0fs)",
		sum(metrics.FairReaderEvictedTasks.Name()),
		sum(metrics.FairReaderEvictedAcks.Name()),
		sum(metrics.FairReaderReinsertedAcks.Name()),
		sum(metrics.FairReaderStuckDetected.Name()),
		duplicates, processed, elapsed.Seconds())
}

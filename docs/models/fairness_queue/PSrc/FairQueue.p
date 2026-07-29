// P model of service/matching/fair_task_{reader,writer}.go
//
// Milestone M2: out-of-order writes and the full merge.
//  - Database, Writer, Reader, Acker as separate machines (all DB calls succeed).
//  - Writer picks nondeterministic unique levels above the pinned ack level,
//    possibly below the reader's readLevel, in batches of 1-2.
//  - Ack level pinning during writes (getAndPinAckLevel / unpinAckLevel).
//  - Full mergeTasksLocked semantics: merged set of loaded + incoming tasks,
//    keep first batchSize, eviction of loaded tasks and of acks above the new
//    readLevel, readLevel left alone when the merged set is empty (f534e74e),
//    newlyWrittenTasks held while a read is pending (12e7c43a).
//  - Acker stands in for matcher + pollers; eviction can race an in-flight
//    completion, so completions for missing/already-acked tasks are no-ops.
//
// Levels are single unique integers (abstracting the <pass, id> fair level).
// Level 0 is the initial ack/read level; real tasks have level >= 1.

enum tMergeMode { MERGE_READ_MIDDLE, MERGE_READ_TO_END, MERGE_WRITE }

// ---- events ----

// Writer <-> Database (CreateFairTasks)
event eWriteTasksReq: (writer: machine, tasks: seq[int]);
event eWriteTasksResp: seq[int];

// Reader <-> Database (GetFairTasks): return up to `count` tasks with level > gtLevel
event eGetTasksReq: (reader: machine, gtLevel: int, count: int);
event eGetTasksResp: seq[int];
// Database internal: a computed read result that hasn't been delivered yet.
// Models the window in Go between the db computing a read result and the
// reader merging it (the reader lock is not held during the db call): without
// this hop, P's atomic send + FIFO queues would deliver read results to the
// reader before any eWroteTasks from later db operations, hiding real races.
event eReadServed: (reader: machine, tasks: seq[int]);

// Writer <-> Reader: getAndPinAckLevel / wroteNewTasks / unpinAckLevel
event eGetPinReq: machine;
event ePinResp: int;
event eWroteTasks: seq[int];
event eUnpin;

// Reader -> Acker (addTaskToMatcher / setEvicted) and Acker -> Reader (completeTask)
event eTaskLoaded: int;
event eTaskEvicted: int;
event eAckTask: int;
event eAckKick;

// Reader <-> Database (CompleteFairTasksLessThan): delete tasks <= upTo
event eGcReq: (reader: machine, upTo: int);
event eGcResp;

// Driver -> Reader (to close the Reader <-> Acker reference cycle)
event eBindAcker: machine;

// announcements for spec monitors
event eTaskConfirmed: int; // writer observed a successful write of this level
event eTaskCompleted: int; // reader marked this level acked (completeTask)
event eTaskDeleted: int;   // db deleted this level (GC)

// ---- database ----

machine Database {
  var tasks: seq[int]; // sorted ascending

  start state Serving {
    on eWriteTasksReq do (req: (writer: machine, tasks: seq[int])) {
      var i: int;
      i = 0;
      while (i < sizeof(req.tasks)) {
        insertTask(req.tasks[i]);
        i = i + 1;
      }
      send req.writer, eWriteTasksResp, req.tasks;
    }

    on eGetTasksReq do (req: (reader: machine, gtLevel: int, count: int)) {
      var res: seq[int];
      var i: int;
      i = 0;
      while (i < sizeof(tasks) && sizeof(res) < req.count) {
        if (tasks[i] > req.gtLevel) {
          res += (sizeof(res), tasks[i]);
        }
        i = i + 1;
      }
      send this, eReadServed, (reader = req.reader, tasks = res);
    }

    on eReadServed do (r: (reader: machine, tasks: seq[int])) {
      send r.reader, eGetTasksResp, r.tasks;
    }

    // GC: delete everything <= upTo (Cassandra range-delete semantics; the
    // SQL batch-limited variant only deletes less, which is strictly safer)
    on eGcReq do (req: (reader: machine, upTo: int)) {
      while (sizeof(tasks) > 0 && tasks[0] <= req.upTo) {
        announce eTaskDeleted, tasks[0];
        tasks -= (0);
      }
      send req.reader, eGcResp;
    }
  }

  fun insertTask(lvl: int) {
    var i: int;
    i = 0;
    while (i < sizeof(tasks) && tasks[i] < lvl) {
      i = i + 1;
    }
    assert i == sizeof(tasks) || tasks[i] != lvl, "duplicate task level written";
    tasks += (i, lvl);
  }
}

// ---- reader (fairTaskReader) ----

machine FairReader {
  var db: machine;
  var acker: machine;
  var batchSize: int; // GetTasksBatchSize
  var reloadAt: int;  // GetTasksReloadAt: read more when loaded <= reloadAt

  var outstanding: map[int, bool]; // level -> acked? (false = loaded, true = acked/nil)
  var loaded: int;                 // number of unacked entries in outstanding
  var readLevel: int;              // we have (ackLevel, readLevel] in outstanding
  var ackLevel: int;               // inclusive: task at ackLevel has been acked
  var atEnd: bool;                 // outstanding covers the whole queue right now
  var readPending: bool;           // a read is in flight
  var lastReqCount: int;           // batch size of the in-flight read
  var newlyWritten: seq[int];      // tasks written while readPending
  var pinnedByWriter: bool;        // ackLevelPinnedByWriter

  // gc state
  var inGC: bool;
  var numToGC: int;

  start state Init {
    entry (cfg: (db: machine, batchSize: int, reloadAt: int, initLevel: int)) {
      db = cfg.db;
      batchSize = cfg.batchSize;
      reloadAt = cfg.reloadAt;
      readLevel = cfg.initLevel;
      ackLevel = cfg.initLevel;
    }
    on eBindAcker do (a: machine) {
      acker = a;
      goto Running;
    }
    // the writer may start before we're bound
    defer eWroteTasks;
    defer eGetPinReq;
  }

  state Running {
    entry {
      maybeRead(); // Start()
    }

    // one batch of readTasksImpl's loop
    on eGetTasksResp do (batch: seq[int]) {
      var reachedEnd: bool;
      reachedEnd = sizeof(batch) < lastReqCount;
      if (reachedEnd) {
        mergeTasks(batch, MERGE_READ_TO_END);
      } else {
        mergeTasks(batch, MERGE_READ_MIDDLE);
      }
      if (shouldReadMore()) {
        sendRead(); // loop again; readPending stays true
      } else {
        readPending = false;
        // process tasks that were written while the read was pending; note
        // sizeof(newlyWritten) > 0 keeps the ack level pinned during the merge
        if (sizeof(newlyWritten) > 0) {
          mergeTasks(newlyWritten, MERGE_WRITE);
          newlyWritten = default(seq[int]);
          advanceAckLevel();
        }
        // re-check before finishing (the 26d9a561 fix; trivial until we model
        // the backoff timer, but keep the structure)
        maybeRead();
      }
    }

    // wroteNewTasks -> mergeTasks(mergeWrite)
    on eWroteTasks do (batch: seq[int]) {
      var i: int;
      if (readPending) {
        i = 0;
        while (i < sizeof(batch)) {
          newlyWritten += (sizeof(newlyWritten), batch[i]);
          i = i + 1;
        }
        return;
      }
      mergeTasks(batch, MERGE_WRITE);
      // Go's "fair reader stuck" softassert state; modeled as a hard assertion
      // (we deliberately do not model the defensive repair read)
      assert !(loaded == 0 && !atEnd && !readPending), "fair reader stuck";
    }

    // getAndPinAckLevel
    on eGetPinReq do (w: machine) {
      assert !pinnedByWriter, "ack level already pinned";
      pinnedByWriter = true;
      send w, ePinResp, ackLevel;
    }

    // unpinAckLevel (no write errors yet in M2)
    on eUnpin do {
      assert pinnedByWriter, "ack level wasn't pinned";
      pinnedByWriter = false;
      advanceAckLevel();
    }

    // doGC completed (Cassandra: everything <= upTo is gone)
    on eGcResp do {
      inGC = false;
      numToGC = 0;
    }

    // completeTask
    on eAckTask do (lvl: int) {
      if (!(lvl in outstanding)) {
        // the task was evicted while this completion was in flight; it will be
        // re-read and re-dispatched later (Go: TaskCompletedMissing)
        return;
      }
      if (outstanding[lvl]) {
        // completion for a task that was already completed: an eviction /
        // re-read / re-dispatch race (Go softasserts "completed task was
        // already acked" here and returns)
        return;
      }
      outstanding[lvl] = true;
      loaded = loaded - 1;
      assert loaded >= 0, "loadedTasks went negative";
      announce eTaskCompleted, lvl;
      advanceAckLevel();
      maybeRead();
      checkInvariants();
    }
  }

  // mergeTasksLocked
  fun mergeTasks(incoming: seq[int], mode: tMergeMode) {
    var merged: seq[int]; // sorted: unacked loaded levels + accepted new levels
    var isNew: set[int];  // subset of merged not yet in outstanding
    var ks: seq[int];
    var i: int;
    var lvl: int;
    var kept: int; // how many of merged we keep in memory
    var evictedAny: bool;

    // (1) currently loaded (unacked) tasks
    ks = keys(outstanding);
    i = 0;
    while (i < sizeof(ks)) {
      if (!outstanding[ks[i]]) {
        merged = insertSorted(merged, ks[i]);
      }
      i = i + 1;
    }

    // (2) the tasks we just read/wrote
    i = 0;
    while (i < sizeof(incoming)) {
      lvl = incoming[i];
      if (lvl <= ackLevel) {
        // reads may race with acks; ignore tasks already acked
      } else if (mode == MERGE_WRITE && !atEnd && lvl > readLevel) {
        // writing while not at the end: we don't know what's between
        // readLevel and lvl, so ignore tasks above readLevel
      } else if (lvl in outstanding) {
        // already have it (loaded or acked); ignore
      } else {
        assert !(lvl in isNew), "duplicate level in merge batch";
        merged = insertSorted(merged, lvl);
        isNew += (lvl);
      }
      i = i + 1;
    }

    // keep the first batchSize of the merged set
    kept = sizeof(merged);
    if (kept > batchSize) {
      kept = batchSize;
    }
    if (kept > 0) {
      // if we have any tasks at all in memory, readLevel is the max of that set
      readLevel = merged[kept - 1];
    }
    // else: merged is empty (only acks in memory); leave readLevel unchanged
    // (f534e74e: collapsing it to ackLevel would evict all acks and strand the
    // reader)

    // evict whatever doesn't fit: loaded tasks are removed from memory and the
    // matcher; new tasks are simply not taken (they stay in the db)
    evictedAny = false;
    i = kept;
    while (i < sizeof(merged)) {
      lvl = merged[i];
      evictedAny = true;
      if (!(lvl in isNew)) {
        outstanding -= (lvl);
        loaded = loaded - 1;
        send acker, eTaskEvicted, lvl;
      }
      i = i + 1;
    }

    // also evict acked (nil) entries above the new readLevel, otherwise we'd
    // use them to jump the ack level across ranges we just dropped
    ks = keys(outstanding);
    i = 0;
    while (i < sizeof(ks)) {
      if (outstanding[ks[i]] && ks[i] > readLevel) {
        outstanding -= (ks[i]);
        evictedAny = true;
        // M6 will cache these (evictedAcks)
      }
      i = i + 1;
    }

    // take the new tasks that made the cut
    i = 0;
    while (i < kept) {
      lvl = merged[i];
      if (lvl in isNew) {
        outstanding[lvl] = false;
        loaded = loaded + 1;
        send acker, eTaskLoaded, lvl;
      }
      i = i + 1;
    }

    // pre-acked entries may now be at the bottom (no-op until M5/M6; also
    // normally pinned during writes)
    advanceAckLevel();

    // update atEnd
    if (mode == MERGE_READ_MIDDLE || evictedAny) {
      atEnd = false;
    } else if (mode == MERGE_READ_TO_END) {
      atEnd = true;
    }
    // on write: leave unchanged

    checkInvariants();
  }

  fun insertSorted(s: seq[int], v: int): seq[int] {
    var i: int;
    i = 0;
    while (i < sizeof(s) && s[i] < v) {
      i = i + 1;
    }
    assert i == sizeof(s) || s[i] != v, "insertSorted: duplicate";
    s += (i, v);
    return s;
  }

  fun ackLevelPinned(): bool {
    return pinnedByWriter || sizeof(newlyWritten) > 0;
  }

  fun advanceAckLevel() {
    var mn: int;
    var moving: bool;
    var numAcked: int;
    if (ackLevelPinned()) {
      return;
    }
    moving = true;
    while (moving && sizeof(outstanding) > 0) {
      mn = minKey();
      if (outstanding[mn]) {
        ackLevel = mn;
        outstanding -= (mn);
        numAcked = numAcked + 1;
      } else {
        moving = false;
      }
    }
    if (numAcked > 0) {
      numToGC = numToGC + numAcked;
      maybeGC();
    }
  }

  // maybeGCLocked: Go triggers on a count threshold or elapsed time; both are
  // abstracted into a nondeterministic choice
  fun maybeGC() {
    if (inGC || numToGC == 0) {
      return;
    }
    if (choose()) {
      inGC = true;
      send db, eGcReq, (reader = this, upTo = ackLevel);
    }
  }

  fun minKey(): int {
    var ks: seq[int];
    var i: int;
    var mn: int;
    ks = keys(outstanding);
    mn = ks[0];
    i = 1;
    while (i < sizeof(ks)) {
      if (ks[i] < mn) {
        mn = ks[i];
      }
      i = i + 1;
    }
    return mn;
  }

  fun shouldReadMore(): bool {
    if (atEnd) {
      return false;
    }
    if (loaded > reloadAt) {
      return false;
    }
    return true;
  }

  fun maybeRead() {
    if (readPending || !shouldReadMore()) {
      return;
    }
    readPending = true;
    sendRead();
  }

  fun sendRead() {
    lastReqCount = batchSize - loaded;
    assert lastReqCount > 0, "read batch size must be positive";
    send db, eGetTasksReq, (reader = this, gtLevel = readLevel, count = lastReqCount);
  }

  fun checkInvariants() {
    var ks: seq[int];
    var i: int;
    var cnt: int;
    assert ackLevel <= readLevel, "ackLevel above readLevel";
    ks = keys(outstanding);
    i = 0;
    cnt = 0;
    while (i < sizeof(ks)) {
      assert ks[i] > ackLevel, format("outstanding level {0} <= ackLevel {1}", ks[i], ackLevel);
      assert ks[i] <= readLevel, format("outstanding level {0} > readLevel {1}", ks[i], readLevel);
      if (!outstanding[ks[i]]) {
        cnt = cnt + 1;
      }
      i = i + 1;
    }
    assert cnt == loaded, "loaded count mismatch";
  }
}

// ---- writer (fairTaskWriter) ----

machine FairWriter {
  var db: machine;
  var reader: machine;
  var numTasks: int; // total tasks to write
  var maxLevel: int; // level universe is 1..maxLevel
  var used: set[int]; // levels ever allocated (task ids are never reused)
  var written: int;

  start state Writing {
    entry (cfg: (db: machine, reader: machine, numTasks: int, maxLevel: int)) {
      db = cfg.db;
      reader = cfg.reader;
      numTasks = cfg.numTasks;
      maxLevel = cfg.maxLevel;
      startWrite();
    }

    // writeBatch: pin ack level, pick levels above it, write
    on ePinResp do (pinnedAck: int) {
      var candidates: seq[int];
      var batch: seq[int];
      var target: int;
      var lvl: int;
      var i: int;

      candidates = default(seq[int]);
      lvl = pinnedAck + 1;
      while (lvl <= maxLevel) {
        if (!(lvl in used)) {
          candidates += (sizeof(candidates), lvl);
        }
        lvl = lvl + 1;
      }

      target = 1 + choose(2); // batch of 1 or 2 (getWriteBatch)
      if (target > numTasks - written) {
        target = numTasks - written;
      }
      while (sizeof(batch) < target && sizeof(candidates) > 0) {
        i = choose(sizeof(candidates));
        lvl = candidates[i];
        candidates -= (i);
        used += (lvl);
        batch += (sizeof(batch), lvl);
      }

      if (sizeof(batch) == 0) {
        // no usable levels left (level universe exhausted): stop writing
        send reader, eUnpin;
        return;
      }
      written = written + sizeof(batch);
      send db, eWriteTasksReq, (writer = this, tasks = batch);
    }

    on eWriteTasksResp do (batch: seq[int]) {
      var i: int;
      i = 0;
      while (i < sizeof(batch)) {
        announce eTaskConfirmed, batch[i];
        i = i + 1;
      }
      // wroteNewTasks must be called before unpin
      send reader, eWroteTasks, batch;
      send reader, eUnpin;
      startWrite();
    }
  }

  fun startWrite() {
    if (written >= numTasks) {
      return;
    }
    send reader, eGetPinReq, this;
  }
}

// ---- acker (stands in for matcher + poller: eventually completes every loaded task) ----

machine Acker {
  var reader: machine;
  var pending: set[int];

  start state Acking {
    entry (r: machine) {
      reader = r;
    }
    on eTaskLoaded do (lvl: int) {
      pending += (lvl);
      send this, eAckKick; // one kick per task: everything acks eventually
    }
    on eTaskEvicted do (lvl: int) {
      // removed from the matcher before matching; if it's not in pending, the
      // completion is already in flight and the reader will ignore it
      if (lvl in pending) {
        pending -= (lvl);
      }
    }
    on eAckKick do {
      var lvl: int;
      if (sizeof(pending) == 0) {
        return; // its task was evicted or acked by an earlier kick
      }
      lvl = choose(pending);
      pending -= (lvl);
      send reader, eAckTask, lvl;
    }
  }
}

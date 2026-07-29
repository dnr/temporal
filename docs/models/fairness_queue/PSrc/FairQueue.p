// P model of service/matching/fair_task_{reader,writer}.go
//
// Milestone M1: happy path.
//  - Database, Writer, Reader, Acker as separate machines (all DB calls succeed).
//  - Writer writes tasks one at a time with increasing integer levels.
//  - Reader keeps a bounded buffer, tracks readLevel/ackLevel/atEnd, and holds
//    tasks written during a pending read (newlyWrittenTasks).
//  - Acker acks loaded tasks in nondeterministic order.
//
// Levels are single unique integers (abstracting the <pass, id> fair level).
// Level 0 is the initial ack/read level; real tasks have level >= 1.

// ---- events ----

// Writer <-> Database (CreateFairTasks)
event eWriteTasksReq: (writer: machine, tasks: seq[int]);
event eWriteTasksResp: seq[int];

// Reader <-> Database (GetFairTasks): return up to `count` tasks with level > gtLevel
event eGetTasksReq: (reader: machine, gtLevel: int, count: int);
event eGetTasksResp: seq[int];

// Writer -> Reader (wroteNewTasks)
event eWroteTasks: seq[int];

// Reader -> Acker (addTaskToMatcher) and Acker -> Reader (completeTask)
event eTaskLoaded: int;
event eAckTask: int;
event eAckKick;

// Driver -> Reader (to close the Reader <-> Acker reference cycle)
event eBindAcker: machine;

// announcements for spec monitors
event eTaskConfirmed: int; // writer observed a successful write of this level
event eTaskCompleted: int; // reader marked this level acked (completeTask)

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
      send req.reader, eGetTasksResp, res;
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
    // the writer may start writing before we're bound
    defer eWroteTasks;
  }

  state Running {
    entry {
      maybeRead(); // Start()
    }

    // one batch of readTasksImpl's loop
    on eGetTasksResp do (batch: seq[int]) {
      mergeRead(batch, sizeof(batch) < lastReqCount);
      if (shouldReadMore()) {
        sendRead(); // loop again; readPending stays true
      } else {
        readPending = false;
        // process tasks that were written while the read was pending
        if (sizeof(newlyWritten) > 0) {
          mergeWrite(newlyWritten);
          newlyWritten = default(seq[int]);
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
      mergeWrite(batch);
    }

    // completeTask (happy path only in M1)
    on eAckTask do (lvl: int) {
      assert lvl in outstanding, "completed task missing from outstanding";
      assert outstanding[lvl] == false, "completed task was already acked";
      outstanding[lvl] = true;
      loaded = loaded - 1;
      assert loaded >= 0, "loadedTasks went negative";
      announce eTaskCompleted, lvl;
      advanceAckLevel();
      maybeRead();
      checkInvariants();
    }
  }

  // merge a batch that came from a db read. M1 simplification of
  // mergeTasksLocked: reads never overflow the buffer (we only ask for
  // batchSize - loaded), so no eviction; readLevel only moves up.
  fun mergeRead(batch: seq[int], reachedEnd: bool) {
    var i: int;
    var lvl: int;
    i = 0;
    while (i < sizeof(batch)) {
      lvl = batch[i];
      if (lvl <= ackLevel) {
        // raced with ack level movement; ignore
      } else if (lvl in outstanding) {
        // already have it (loaded or acked); ignore
      } else {
        takeTask(lvl);
      }
      i = i + 1;
    }
    atEnd = reachedEnd;
    checkInvariants();
  }

  // merge a batch that came from a write. M1 simplification: writes are
  // always above readLevel (monotone levels), so a written task is either
  // taken at the top of the buffer or ignored.
  fun mergeWrite(batch: seq[int]) {
    var i: int;
    var lvl: int;
    i = 0;
    while (i < sizeof(batch)) {
      lvl = batch[i];
      assert lvl > readLevel || lvl in outstanding || lvl <= ackLevel,
        "M1 expects monotone writes";
      if (lvl <= ackLevel) {
        // ignore
      } else if (lvl in outstanding) {
        // ignore
      } else if (!atEnd && lvl > readLevel) {
        // not at the end: there may be tasks between readLevel and lvl, so we
        // can't take it; a read will pick it up
      } else if (loaded < batchSize) {
        // at the end with room: take it directly (bypass optimization)
        takeTask(lvl);
      } else {
        // at the end but no room: the task stays in the db beyond our buffer,
        // so we're no longer at the end
        atEnd = false;
      }
      i = i + 1;
    }
    // Go's "fair reader stuck" softassert state; modeled as a hard assertion
    // (we deliberately do not model the defensive repair read)
    assert !(loaded == 0 && !atEnd && !readPending), "fair reader stuck";
    checkInvariants();
  }

  fun takeTask(lvl: int) {
    outstanding[lvl] = false;
    loaded = loaded + 1;
    if (lvl > readLevel) {
      readLevel = lvl;
    }
    send acker, eTaskLoaded, lvl;
  }

  fun advanceAckLevel() {
    var mn: int;
    var moving: bool;
    moving = true;
    while (moving && sizeof(outstanding) > 0) {
      mn = minKey();
      if (outstanding[mn]) {
        ackLevel = mn;
        outstanding -= (mn);
      } else {
        moving = false;
      }
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

// ---- writer (fairTaskWriter, heavily simplified for M1) ----

machine FairWriter {
  var db: machine;
  var reader: machine;
  var numTasks: int;
  var nextLevel: int;
  var written: int;

  start state Writing {
    entry (cfg: (db: machine, reader: machine, numTasks: int, startLevel: int)) {
      db = cfg.db;
      reader = cfg.reader;
      numTasks = cfg.numTasks;
      nextLevel = cfg.startLevel;
      writeNext();
    }
    on eWriteTasksResp do (batch: seq[int]) {
      var i: int;
      i = 0;
      while (i < sizeof(batch)) {
        announce eTaskConfirmed, batch[i];
        i = i + 1;
      }
      send reader, eWroteTasks, batch;
      writeNext();
    }
  }

  fun writeNext() {
    var lvl: int;
    var batch: seq[int];
    if (written >= numTasks) {
      return;
    }
    // monotonically increasing levels, nondeterministically with a gap
    lvl = nextLevel + choose(2);
    nextLevel = lvl + 1;
    batch += (0, lvl);
    written = written + 1;
    send db, eWriteTasksReq, (writer = this, tasks = batch);
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
    on eAckKick do {
      var lvl: int;
      if (sizeof(pending) == 0) {
        return;
      }
      lvl = choose(pending);
      pending -= (lvl);
      send reader, eAckTask, lvl;
    }
  }
}

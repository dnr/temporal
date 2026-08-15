---------------------------- MODULE PriQueue ----------------------------
(***************************************************************************)
(* PlusCal model of matching's fifo (non-fair) task queue reader/writer.    *)
(* Source: service/matching/pri_task_{reader,writer}.go and the ack-level  *)
(* bookkeeping in service/matching/db.go.                                  *)
(*                                                                         *)
(* The reader is called priTaskReader for historical reasons; it serves    *)
(* one subqueue of a fifo (task-id ordered) backlog.                       *)
(*                                                                         *)
(* Key abstractions:                                                       *)
(*                                                                         *)
(* - Task ids are 1..MaxLevel. Ids 1..InitLevels are the initial backlog   *)
(*   (any subset of them, so id gaps are covered); the writer allocates    *)
(*   ids above the db's maxReadLevel, in increasing order, possibly with   *)
(*   gaps (assignTaskIDs burns ids on failed writes, block boundaries      *)
(*   leave gaps, and ids of other subqueues are gaps for this one).        *)
(*                                                                         *)
(* - Go guards reader state with tr.lock; every locked critical section is *)
(*   one atomic PlusCal step (one label). Lock release points are label    *)
(*   boundaries, which is where processes interleave. The two snapshots    *)
(*   getTaskBatch takes -- tr.readLevel (under tr.lock) and                *)
(*   db.GetMaxReadLevel (under db.Lock) -- are separate steps, which is    *)
(*   where the read/write race modeled here lives.                         *)
(*                                                                         *)
(* - outstandingTasks (treemap id -> acked) is split into two sets:        *)
(*   "loaded" (acked=false entries) and "ackedInMem" (acked=true entries   *)
(*   that the ack level has not passed yet). loadedTasks = |loaded|.       *)
(*                                                                         *)
(* - A task is ackable as soon as it is added to loaded; the matcher       *)
(*   handoff, addSpooledTask errors and respooling are not modeled.        *)
(*                                                                         *)
(* - The db is reached through request/response channels (rdState,         *)
(*   wrState, gcState) served by separate processes, so RPCs interleave    *)
(*   with everything. db.CreateTasks holds db.Lock across the store call,  *)
(*   and updates maxReadLevel before returning *whether or not the write   *)
(*   succeeded*, so that is one atomic db step here.                       *)
(*                                                                         *)
(* - Writes may time out with the row landed (incoming timeout) or not     *)
(*   (outgoing timeout). Only "committed" tasks (initial backlog + writes  *)
(*   whose RPC succeeded) carry a delivery guarantee: on error the caller  *)
(*   (history) re-submits with a fresh task id, so a landed row from a     *)
(*   failed write is an unguaranteed duplicate. Note that on write error   *)
(*   priTaskWriter does NOT call signalReaders at all (findings.md #1).    *)
(*                                                                         *)
(* - Task expiry (IsTaskExpired in processTaskBatch) is a nondeterministic *)
(*   per-read choice over Expirable: any subset of the tasks a read        *)
(*   returns may be expired by the time the reader looks at them. Expired  *)
(*   tasks leave the committed set (deciding a task is expired ends its    *)
(*   delivery obligation). signalNewTasks does not check expiry, matching  *)
(*   the code.                                                             *)
(*                                                                         *)
(* - GC may fire whenever the ack level is above zero (the numToGC/time    *)
(*   triggers are abstracted away); it is unfair, so liveness cannot       *)
(*   depend on GC running. The delete batch size is not modeled.           *)
(*                                                                         *)
(* - Not modeled: multiple subqueues (ids of other subqueues appear as     *)
(*   gaps), ownership/fencing and rangeid renewal, the approximate backlog *)
(*   counter, throttling/backoff durations, task forwarding.               *)
(*                                                                         *)
(* Mutation flags "Mut...": each TRUE re-introduces a bug, to validate     *)
(* that the properties catch it. All FALSE models the code as of           *)
(* e0a4751c ("Fix read and ack levels moving backwards in priTaskReader"); *)
(* the three historical flags together model the code before that fix.     *)
(* See run.sh.                                                             *)
(***************************************************************************)
EXTENDS Naturals, FiniteSets, TLC

CONSTANTS
  MaxLevel,      \* task ids are 1..MaxLevel
  InitLevels,    \* the initial backlog is a subset of 1..InitLevels, and the
                 \* db's maxReadLevel starts there (a fresh id block starts
                 \* above every existing task id)
  BatchSize,     \* config.GetTasksBatchSize: read page size, and the cap on
                 \* direct-adds in signalNewTasks
  ReloadAt,      \* config.GetTasksReloadAt: the pump ignores a signal while
                 \* more than this many tasks are loaded
  RangeSize,     \* config.RangeSize: how much of the id space one GetTasks
                 \* call scans
  ScanIters,     \* getTaskBatch's "i < 10" cap on scans per batch
  WBatchMax,     \* max tasks per CreateTasks batch
  AckableLevels, \* ids the acker may ack (all of Levels normally; a subset
                 \* models a task with no available poller)
  Expirable,     \* ids that a read may find expired ({} = no expiry)
  WriteErrors,   \* TRUE: CreateTasks may fail (with or without the rows
                 \* landing). FALSE: writes always succeed, which is what
                 \* EventuallyCaughtUp needs (see findings.md #1)
  ReadErrors,    \* TRUE: GetTasks may fail, until the network heals
  \* --- mutation flags ---
  \* the three hunks of e0a4751c (all TRUE = the code before that fix):
  MutStaleGapLevels,     \* setReadLevelAfterGap applies stale read/ack levels
  MutNoAckedTaskFilter,  \* processTaskBatch does not skip tasks <= ackLevel
  MutDbAckBackwards,     \* db.updateAckLevelAndBacklogStats does not clamp
  \* seeded:
  MutSignalAlwaysOnGap,  \* setReadLevelAfterGap signals even when not stale
  MutNoSignalOnStale,    \* setReadLevelAfterGap discards stale levels but does
                         \* not signal itself (keeps hunk 3's check, drops its
                         \* SignalTaskLoading)
  MutNoDedup,            \* processTaskBatch drops the outstandingTasks dedup
  MutNoDirectAddLevelCheck, \* signalNewTasks skips readLevel==maxReadLevelBefore
  MutNoRoomCheck,        \* signalNewTasks skips the "room in memory" check
  MutGcReadLevel,        \* GC deletes up to readLevel instead of ackLevel
  MutNoGapSignal,        \* pump skips the !isReadBatchDone signal
  MutNoReloadSignal,     \* completeTask skips the loadedTasks==ReloadAt signal
  MutDrainedAckIgnoresOutstanding \* isDrained ignores outstandingTasks

ASSUME ReloadAt < BatchSize
ASSUME MaxLevel >= 1 /\ InitLevels <= MaxLevel
ASSUME RangeSize >= 1 /\ ScanIters >= 1 /\ WBatchMax >= 1
ASSUME AckableLevels \subseteq 1..MaxLevel
ASSUME Expirable \subseteq 1..MaxLevel

Levels  == 1..MaxLevel
NoLevel == 0

Min2(a, b) == IF a < b THEN a ELSE b
SetMax(S)  == CHOOSE x \in S : \A y \in S : y <= x
MaxOr(S, d) == IF S = {} THEN d ELSE SetMax(S)
\* The n lowest elements of S (all of S if it has <= n elements).
KeepLowest(S, n) == {l \in S : Cardinality({m \in S : m <= l}) <= n}

(* --algorithm PriQueue

variables
  \* ---- database ----
  dbTasks \in SUBSET (1..InitLevels), \* nondeterministic initial backlog
  dbMaxReadLevel = InitLevels,        \* db.subqueues[i].maxReadLevel
  dbAckLevel     = NoLevel,           \* db.subqueues[i].AckLevel (persisted)
  \* ---- read RPC channel: reader -> db (GetTasks) ----
  rdState  = "idle",           \* idle -> req -> resp -> idle
  rdFrom   = NoLevel,          \* inclusiveMinTaskID
  rdTo     = NoLevel,          \* exclusiveMaxTaskID - 1
  rdResult = {},
  rdOk     = TRUE,             \* FALSE: read failed (no side effects)
  \* ---- write RPC channel: writer -> db (CreateTasks) ----
  wrState  = "idle",           \* idle -> req -> resp -> idle
  wrBatch  = {},
  wrBefore = NoLevel,          \* maxReadLevelBefore
  wrOk     = TRUE,             \* FALSE: write failed (may still have applied)
  \* ---- gc RPC channel ----
  gcState  = "idle",
  gcFrom   = NoLevel,
  \* ---- reader state (fields of priTaskReader, guarded by tr.lock) ----
  loaded       = {},           \* outstandingTasks entries with acked=false
  ackedInMem   = {},           \* outstandingTasks entries with acked=true
  readLevel    = NoLevel,
  ackLevel     = NoLevel,
  notify       = TRUE,         \* notifyC holds an event (Start primes the pump)
  backoffTimer = FALSE,        \* a read-retry backoff timer is armed
  \* ---- environment ----
  netFlaky     = ReadErrors,   \* reads may fail while this holds; the "heal"
                               \* process eventually clears it ("the network
                               \* eventually stops timing out"). Without this,
                               \* nothing rules out a read failing on every
                               \* single retry forever.
  \* ---- ghost state (not part of the implementation) ----
  everInDb     = dbTasks,      \* every id ever present in dbTasks
  committed    = dbTasks,      \* initial backlog + writes whose RPC succeeded;
                               \* only these carry a delivery guarantee
  ackedGhost   = {},           \* every id ever acked
  dispatched   = {},           \* every id ever handed to the matcher
  expiredGhost = {},           \* every id ever dropped as expired
  gcVictims    = {},           \* every id ever deleted by GC
  reDispatch   = FALSE,        \* an id was dispatched twice
  dbAckAssert  = FALSE,        \* db's "ack level should not move backwards"
                               \* softassert fired
  writeDupAssert = FALSE;      \* signalNewTasks' "newly-written task already
                               \* present in outstanding tasks" softassert fired

define
  Outstanding == loaded \cup ackedInMem
  \* the writer allocates above the current max read level; ids are never
  \* reused (even by a failed write)
  AvailableLevels == {l \in Levels : l > dbMaxReadLevel}
  WriteBatches    == {B \in SUBSET AvailableLevels :
                        B /= {} /\ Cardinality(B) <= WBatchMax}
end define;

\* db.updateAckLevelAndBacklogStats: flag a backwards move, and (since
\* e0a4751c) keep the higher level. MutDbAckBackwards restores the old
\* "log and persist it anyway" behavior.
macro dbUpdateAck(newAck) begin
  dbAckAssert := dbAckAssert \/ newAck < dbAckLevel;
  dbAckLevel  := IF newAck < dbAckLevel /\ ~MutDbAckBackwards
                 THEN dbAckLevel ELSE newAck;
end macro;

\* getTasksPump: wait for a signal, read a batch, process it.
fair process reader = "reader"
variables scanRl = NoLevel, scanMax = NoLevel, scanIter = 0, batchTasks = {};
begin
RWait:
  while TRUE do
    await notify;
    notify := FALSE;
RCheckLoaded:
    \* "Too many loaded already, ignore this signal. We'll get another signal
    \* when loadedTasks drops low enough."
    if Cardinality(loaded) <= ReloadAt then
RSnapRl:
      \* getTaskBatch: capture tr.readLevel under tr.lock, then release it
      scanRl   := readLevel;
      scanIter := 0;
RSnapMax:
      \* db.GetMaxReadLevel: taken without tr.lock, so it can already be
      \* newer than the readLevel we captured
      scanMax := dbMaxReadLevel;
RScan:
      while scanIter < ScanIters /\ scanRl < scanMax do
        \* GetTasks(readLevel+1, upper+1) where upper = min(readLevel+RangeSize, maxReadLevel)
        rdFrom  := scanRl + 1;
        rdTo    := Min2(scanRl + RangeSize, scanMax);
        rdState := "req";
RScanResp:
        await rdState = "resp";
        rdState := "idle";
        if ~rdOk then
          \* backoffSignal: arm the retry timer and go back to waiting
          backoffTimer := TRUE;
          goto RWait;
        elsif rdResult /= {} then
          \* "return as long as it grabs any tasks"
          batchTasks := rdResult;
          goto RProcess;
        else
          scanRl   := Min2(scanRl + RangeSize, scanMax);
          scanIter := scanIter + 1;
        end if;
      end while;
RGap:
      \* len(batch.tasks) == 0: setReadLevelAfterGap(batch.readLevel), then
      \* signal ourselves again if the scan did not reach the max read level.
      if /\ ~MutStaleGapLevels
         /\ \/ scanRl < readLevel
            \/ (MutSignalAlwaysOnGap /\ scanRl = readLevel)
      then
        \* e0a4751c: the levels this call is based on are stale (signalNewTasks
        \* advanced readLevel past the scanned range while the read was in
        \* flight). Applying them would move readLevel (and possibly ackLevel)
        \* backwards; isReadBatchDone is stale too, so signal ourselves.
        if ~MutNoSignalOnStale then
          notify := TRUE;
        end if;
      else
        if ackLevel = readLevel then
          \* nothing outstanding and nothing in the scanned range: everything
          \* up to scanRl is acked
          ackLevel := scanRl;
          dbUpdateAck(scanRl);
        end if;
        readLevel := scanRl;
      end if;
RGapSignal:
      \* back in getTasksPump, without tr.lock, on the batch we captured
      if scanRl /= scanMax /\ ~MutNoGapSignal then \* !batch.isReadBatchDone
        notify := TRUE;
      end if;
      goto RWait;
RProcess:
      \* processTaskBatch + addNewTasks. readLevel is raised by every task in
      \* the batch, including the ones filtered out below.
      with exp \in SUBSET (batchTasks \cap Expirable),
           dedup    = IF MutNoDedup THEN {} ELSE Outstanding,
           belowAck = IF MutNoAckedTaskFilter THEN {}
                      ELSE {l \in batchTasks : l <= ackLevel},
           keep     = ((batchTasks \ exp) \ dedup) \ belowAck
      do
        readLevel    := SetMax(batchTasks \cup {readLevel});
        loaded       := loaded \cup keep;
        committed    := committed \ exp;
        expiredGhost := expiredGhost \cup exp;
        reDispatch   := reDispatch \/ (keep \cap dispatched) /= {};
        dispatched   := dispatched \cup keep;
      end with;
      notify := TRUE;   \* "There may be more tasks."
    end if;
  end while;
end process;

\* The read-retry backoff timer (time.AfterFunc callback).
fair process timer = "timer"
begin
TimerLoop:
  while TRUE do
    await backoffTimer;
    backoffTimer := FALSE;
    notify := TRUE;
  end while;
end process;

\* taskWriterLoop: assign ids, CreateTasks, then signalReaders on success.
\* One write is in flight at a time (a single goroutine), and signalNewTasks
\* runs before the next write starts.
fair process writer = "writer"
begin
WLoop:
  while TRUE do
    either
      with B \in WriteBatches do
        wrBatch  := B;
        \* maxReadLevelBefore, captured under db.Lock inside CreateTasks
        wrBefore := dbMaxReadLevel;
        wrState  := "req";
      end with;
WResp:
      await wrState = "resp";
      wrState := "idle";
      \* on error appendTasks returns without calling signalReaders at all
      if wrOk then
        \* signalNewTasks
        writeDupAssert := writeDupAssert
          \/ /\ (readLevel = wrBefore \/ MutNoDirectAddLevelCheck)
             /\ (Cardinality(loaded) + Cardinality(wrBatch) <= BatchSize
                 \/ MutNoRoomCheck)
             /\ wrBatch \cap Outstanding /= {};
        if /\ (readLevel = wrBefore \/ MutNoDirectAddLevelCheck)
           /\ (Cardinality(loaded) + Cardinality(wrBatch) <= BatchSize
               \/ MutNoRoomCheck)
           /\ wrBatch \cap Outstanding = {}
        then
          \* canAddDirect: skip the db read, hand the tasks straight over
          readLevel  := SetMax(wrBatch);   \* maxReadLevelAfter
          loaded     := loaded \cup wrBatch;
          reDispatch := reDispatch \/ (wrBatch \cap dispatched) /= {};
          dispatched := dispatched \cup wrBatch;
        else
          notify := TRUE;
        end if;
      end if;
    or
      \* no more tasks will be written
      goto WDone;
    end either;
  end while;
WDone:
  skip;
end process;

\* GetTasks: the tasks in [rdFrom, rdTo], up to the page size.
fair process dbRead = "dbRead"
begin
DbReadLoop:
  while TRUE do
    await rdState = "req";
    either
      rdResult := KeepLowest({l \in dbTasks : l >= rdFrom /\ l <= rdTo}, BatchSize);
      rdOk     := TRUE;
    or
      \* reads have no side effects, so one error kind suffices
      await netFlaky;
      rdOk := FALSE;
    end either;
    rdState := "resp";
  end while;
end process;

\* The network eventually stops timing out reads.
fair process heal = "heal"
begin
HealNet:
  await netFlaky;
  netFlaky := FALSE;
end process;

\* CreateTasks: db.Lock is held across the store call, and maxReadLevel is
\* updated before returning whether or not the write succeeded.
fair process dbWrite = "dbWrite"
begin
DbWriteLoop:
  while TRUE do
    await wrState = "req";
    either
      \* success
      dbTasks   := dbTasks \cup wrBatch;
      everInDb  := everInDb \cup wrBatch;
      committed := committed \cup wrBatch;
      wrOk      := TRUE;
    or
      \* incoming timeout: the write applied, but the writer sees an error
      await WriteErrors;
      dbTasks  := dbTasks \cup wrBatch;
      everInDb := everInDb \cup wrBatch;
      wrOk     := FALSE;
    or
      \* outgoing timeout: the write did not apply
      await WriteErrors;
      wrOk := FALSE;
    end either;
    dbMaxReadLevel := SetMax(wrBatch);
    wrState := "resp";
  end while;
end process;

\* CompleteTasksLessThan(ackLevel+1): delete tasks <= gcFrom. The batch size
\* limit is not modeled (it only splits the delete across calls). The delete
\* may or may not apply; GC ignores errors.
fair process dbGc = "dbGc"
begin
DbGcLoop:
  while TRUE do
    await gcState = "req";
    either
      with victims = {l \in dbTasks : l <= gcFrom} do
        dbTasks   := dbTasks \ victims;
        gcVictims := gcVictims \cup victims;
      end with;
    or
      skip;
    end either;
    gcState := "resp";
  end while;
end process;

\* maybeGCLocked/doGC: triggered nondeterministically whenever the ack level
\* has moved (the numToGC/time triggers are abstracted to "may run at any such
\* time"). inGC (one outstanding GC) is implied by this being one sequential
\* process. Deliberately NOT fair: liveness must not depend on GC running.
process gc = "gc"
begin
GcLoop:
  while TRUE do
    await ackLevel > NoLevel;
    gcFrom  := IF MutGcReadLevel THEN readLevel ELSE ackLevel;
    gcState := "req";
GcResp:
    await gcState = "resp";
    gcState := "idle";
  end while;
end process;

\* completeTask: ackTaskLocked (mark acked, advance the ack level, and jump it
\* to the read level if drained), the reload signal, then push the ack level
\* to the db -- all under tr.lock.
fair process acker = "acker"
begin
AckLoop:
  while TRUE do
    await loaded \cap AckableLevels /= {};
    with l     \in loaded \cap AckableLevels,
         ld    =   loaded \ {l},
         ackd  =   ackedInMem \cup {l},
         \* pop acked entries from the front of outstandingTasks
         clear =   {c \in ackd : \A m \in ld : c < m},
         rest  =   ackd \ clear,
         al1   =   IF clear /= {} THEN SetMax(clear) ELSE ackLevel,
         \* isDrainedLocked: outstandingTasks empty and read to the end
         drained = /\ ((ld = {} /\ rest = {}) \/ MutDrainedAckIgnoresOutstanding)
                   /\ readLevel >= dbMaxReadLevel,
         al2   =   IF drained THEN readLevel ELSE al1
    do
      loaded     := ld;
      ackedInMem := rest;
      ackLevel   := al2;
      ackedGhost := ackedGhost \cup {l};
      \* "use == so we just signal once when we cross this threshold"
      if Cardinality(ld) = ReloadAt /\ ~MutNoReloadSignal then
        notify := TRUE;
      end if;
      dbUpdateAck(al2);
    end with;
  end while;
end process;

end algorithm; *)
\* BEGIN TRANSLATION (chksum(pcal) = "5fdbbbe" /\ chksum(tla) = "ffefa082")
VARIABLES pc, dbTasks, dbMaxReadLevel, dbAckLevel, rdState, rdFrom, rdTo, 
          rdResult, rdOk, wrState, wrBatch, wrBefore, wrOk, gcState, gcFrom, 
          loaded, ackedInMem, readLevel, ackLevel, notify, backoffTimer, 
          netFlaky, everInDb, committed, ackedGhost, dispatched, expiredGhost, 
          gcVictims, reDispatch, dbAckAssert, writeDupAssert

(* define statement *)
Outstanding == loaded \cup ackedInMem


AvailableLevels == {l \in Levels : l > dbMaxReadLevel}
WriteBatches    == {B \in SUBSET AvailableLevels :
                      B /= {} /\ Cardinality(B) <= WBatchMax}

VARIABLES scanRl, scanMax, scanIter, batchTasks

vars == << pc, dbTasks, dbMaxReadLevel, dbAckLevel, rdState, rdFrom, rdTo, 
           rdResult, rdOk, wrState, wrBatch, wrBefore, wrOk, gcState, gcFrom, 
           loaded, ackedInMem, readLevel, ackLevel, notify, backoffTimer, 
           netFlaky, everInDb, committed, ackedGhost, dispatched, 
           expiredGhost, gcVictims, reDispatch, dbAckAssert, writeDupAssert, 
           scanRl, scanMax, scanIter, batchTasks >>

ProcSet == {"reader"} \cup {"timer"} \cup {"writer"} \cup {"dbRead"} \cup {"heal"} \cup {"dbWrite"} \cup {"dbGc"} \cup {"gc"} \cup {"acker"}

Init == (* Global variables *)
        /\ dbTasks \in SUBSET (1..InitLevels)
        /\ dbMaxReadLevel = InitLevels
        /\ dbAckLevel = NoLevel
        /\ rdState = "idle"
        /\ rdFrom = NoLevel
        /\ rdTo = NoLevel
        /\ rdResult = {}
        /\ rdOk = TRUE
        /\ wrState = "idle"
        /\ wrBatch = {}
        /\ wrBefore = NoLevel
        /\ wrOk = TRUE
        /\ gcState = "idle"
        /\ gcFrom = NoLevel
        /\ loaded = {}
        /\ ackedInMem = {}
        /\ readLevel = NoLevel
        /\ ackLevel = NoLevel
        /\ notify = TRUE
        /\ backoffTimer = FALSE
        /\ netFlaky = ReadErrors
        /\ everInDb = dbTasks
        /\ committed = dbTasks
        /\ ackedGhost = {}
        /\ dispatched = {}
        /\ expiredGhost = {}
        /\ gcVictims = {}
        /\ reDispatch = FALSE
        /\ dbAckAssert = FALSE
        /\ writeDupAssert = FALSE
        (* Process reader *)
        /\ scanRl = NoLevel
        /\ scanMax = NoLevel
        /\ scanIter = 0
        /\ batchTasks = {}
        /\ pc = [self \in ProcSet |-> CASE self = "reader" -> "RWait"
                                        [] self = "timer" -> "TimerLoop"
                                        [] self = "writer" -> "WLoop"
                                        [] self = "dbRead" -> "DbReadLoop"
                                        [] self = "heal" -> "HealNet"
                                        [] self = "dbWrite" -> "DbWriteLoop"
                                        [] self = "dbGc" -> "DbGcLoop"
                                        [] self = "gc" -> "GcLoop"
                                        [] self = "acker" -> "AckLoop"]

RWait == /\ pc["reader"] = "RWait"
         /\ notify
         /\ notify' = FALSE
         /\ pc' = [pc EXCEPT !["reader"] = "RCheckLoaded"]
         /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, rdFrom, 
                         rdTo, rdResult, rdOk, wrState, wrBatch, wrBefore, 
                         wrOk, gcState, gcFrom, loaded, ackedInMem, readLevel, 
                         ackLevel, backoffTimer, netFlaky, everInDb, committed, 
                         ackedGhost, dispatched, expiredGhost, gcVictims, 
                         reDispatch, dbAckAssert, writeDupAssert, scanRl, 
                         scanMax, scanIter, batchTasks >>

RCheckLoaded == /\ pc["reader"] = "RCheckLoaded"
                /\ IF Cardinality(loaded) <= ReloadAt
                      THEN /\ pc' = [pc EXCEPT !["reader"] = "RSnapRl"]
                      ELSE /\ pc' = [pc EXCEPT !["reader"] = "RWait"]
                /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, 
                                rdFrom, rdTo, rdResult, rdOk, wrState, wrBatch, 
                                wrBefore, wrOk, gcState, gcFrom, loaded, 
                                ackedInMem, readLevel, ackLevel, notify, 
                                backoffTimer, netFlaky, everInDb, committed, 
                                ackedGhost, dispatched, expiredGhost, 
                                gcVictims, reDispatch, dbAckAssert, 
                                writeDupAssert, scanRl, scanMax, scanIter, 
                                batchTasks >>

RSnapRl == /\ pc["reader"] = "RSnapRl"
           /\ scanRl' = readLevel
           /\ scanIter' = 0
           /\ pc' = [pc EXCEPT !["reader"] = "RSnapMax"]
           /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, 
                           rdFrom, rdTo, rdResult, rdOk, wrState, wrBatch, 
                           wrBefore, wrOk, gcState, gcFrom, loaded, ackedInMem, 
                           readLevel, ackLevel, notify, backoffTimer, netFlaky, 
                           everInDb, committed, ackedGhost, dispatched, 
                           expiredGhost, gcVictims, reDispatch, dbAckAssert, 
                           writeDupAssert, scanMax, batchTasks >>

RSnapMax == /\ pc["reader"] = "RSnapMax"
            /\ scanMax' = dbMaxReadLevel
            /\ pc' = [pc EXCEPT !["reader"] = "RScan"]
            /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, 
                            rdFrom, rdTo, rdResult, rdOk, wrState, wrBatch, 
                            wrBefore, wrOk, gcState, gcFrom, loaded, 
                            ackedInMem, readLevel, ackLevel, notify, 
                            backoffTimer, netFlaky, everInDb, committed, 
                            ackedGhost, dispatched, expiredGhost, gcVictims, 
                            reDispatch, dbAckAssert, writeDupAssert, scanRl, 
                            scanIter, batchTasks >>

RScan == /\ pc["reader"] = "RScan"
         /\ IF scanIter < ScanIters /\ scanRl < scanMax
               THEN /\ rdFrom' = scanRl + 1
                    /\ rdTo' = Min2(scanRl + RangeSize, scanMax)
                    /\ rdState' = "req"
                    /\ pc' = [pc EXCEPT !["reader"] = "RScanResp"]
               ELSE /\ pc' = [pc EXCEPT !["reader"] = "RGap"]
                    /\ UNCHANGED << rdState, rdFrom, rdTo >>
         /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdResult, rdOk, 
                         wrState, wrBatch, wrBefore, wrOk, gcState, gcFrom, 
                         loaded, ackedInMem, readLevel, ackLevel, notify, 
                         backoffTimer, netFlaky, everInDb, committed, 
                         ackedGhost, dispatched, expiredGhost, gcVictims, 
                         reDispatch, dbAckAssert, writeDupAssert, scanRl, 
                         scanMax, scanIter, batchTasks >>

RScanResp == /\ pc["reader"] = "RScanResp"
             /\ rdState = "resp"
             /\ rdState' = "idle"
             /\ IF ~rdOk
                   THEN /\ backoffTimer' = TRUE
                        /\ pc' = [pc EXCEPT !["reader"] = "RWait"]
                        /\ UNCHANGED << scanRl, scanIter, batchTasks >>
                   ELSE /\ IF rdResult /= {}
                              THEN /\ batchTasks' = rdResult
                                   /\ pc' = [pc EXCEPT !["reader"] = "RProcess"]
                                   /\ UNCHANGED << scanRl, scanIter >>
                              ELSE /\ scanRl' = Min2(scanRl + RangeSize, scanMax)
                                   /\ scanIter' = scanIter + 1
                                   /\ pc' = [pc EXCEPT !["reader"] = "RScan"]
                                   /\ UNCHANGED batchTasks
                        /\ UNCHANGED backoffTimer
             /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdFrom, rdTo, 
                             rdResult, rdOk, wrState, wrBatch, wrBefore, wrOk, 
                             gcState, gcFrom, loaded, ackedInMem, readLevel, 
                             ackLevel, notify, netFlaky, everInDb, committed, 
                             ackedGhost, dispatched, expiredGhost, gcVictims, 
                             reDispatch, dbAckAssert, writeDupAssert, scanMax >>

RGap == /\ pc["reader"] = "RGap"
        /\ IF /\ ~MutStaleGapLevels
              /\ \/ scanRl < readLevel
                 \/ (MutSignalAlwaysOnGap /\ scanRl = readLevel)
              THEN /\ IF ~MutNoSignalOnStale
                         THEN /\ notify' = TRUE
                         ELSE /\ TRUE
                              /\ UNCHANGED notify
                   /\ UNCHANGED << dbAckLevel, readLevel, ackLevel, 
                                   dbAckAssert >>
              ELSE /\ IF ackLevel = readLevel
                         THEN /\ ackLevel' = scanRl
                              /\ dbAckAssert' = (dbAckAssert \/ scanRl < dbAckLevel)
                              /\ dbAckLevel' = (IF scanRl < dbAckLevel /\ ~MutDbAckBackwards
                                                THEN dbAckLevel ELSE scanRl)
                         ELSE /\ TRUE
                              /\ UNCHANGED << dbAckLevel, ackLevel, 
                                              dbAckAssert >>
                   /\ readLevel' = scanRl
                   /\ UNCHANGED notify
        /\ pc' = [pc EXCEPT !["reader"] = "RGapSignal"]
        /\ UNCHANGED << dbTasks, dbMaxReadLevel, rdState, rdFrom, rdTo, 
                        rdResult, rdOk, wrState, wrBatch, wrBefore, wrOk, 
                        gcState, gcFrom, loaded, ackedInMem, backoffTimer, 
                        netFlaky, everInDb, committed, ackedGhost, dispatched, 
                        expiredGhost, gcVictims, reDispatch, writeDupAssert, 
                        scanRl, scanMax, scanIter, batchTasks >>

RGapSignal == /\ pc["reader"] = "RGapSignal"
              /\ IF scanRl /= scanMax /\ ~MutNoGapSignal
                    THEN /\ notify' = TRUE
                    ELSE /\ TRUE
                         /\ UNCHANGED notify
              /\ pc' = [pc EXCEPT !["reader"] = "RWait"]
              /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, 
                              rdFrom, rdTo, rdResult, rdOk, wrState, wrBatch, 
                              wrBefore, wrOk, gcState, gcFrom, loaded, 
                              ackedInMem, readLevel, ackLevel, backoffTimer, 
                              netFlaky, everInDb, committed, ackedGhost, 
                              dispatched, expiredGhost, gcVictims, reDispatch, 
                              dbAckAssert, writeDupAssert, scanRl, scanMax, 
                              scanIter, batchTasks >>

RProcess == /\ pc["reader"] = "RProcess"
            /\ \E exp \in SUBSET (batchTasks \cap Expirable):
                 LET dedup == IF MutNoDedup THEN {} ELSE Outstanding IN
                   LET belowAck == IF MutNoAckedTaskFilter THEN {}
                                   ELSE {l \in batchTasks : l <= ackLevel} IN
                     LET keep == ((batchTasks \ exp) \ dedup) \ belowAck IN
                       /\ readLevel' = SetMax(batchTasks \cup {readLevel})
                       /\ loaded' = (loaded \cup keep)
                       /\ committed' = committed \ exp
                       /\ expiredGhost' = (expiredGhost \cup exp)
                       /\ reDispatch' = (reDispatch \/ (keep \cap dispatched) /= {})
                       /\ dispatched' = (dispatched \cup keep)
            /\ notify' = TRUE
            /\ pc' = [pc EXCEPT !["reader"] = "RWait"]
            /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, 
                            rdFrom, rdTo, rdResult, rdOk, wrState, wrBatch, 
                            wrBefore, wrOk, gcState, gcFrom, ackedInMem, 
                            ackLevel, backoffTimer, netFlaky, everInDb, 
                            ackedGhost, gcVictims, dbAckAssert, writeDupAssert, 
                            scanRl, scanMax, scanIter, batchTasks >>

reader == RWait \/ RCheckLoaded \/ RSnapRl \/ RSnapMax \/ RScan
             \/ RScanResp \/ RGap \/ RGapSignal \/ RProcess

TimerLoop == /\ pc["timer"] = "TimerLoop"
             /\ backoffTimer
             /\ backoffTimer' = FALSE
             /\ notify' = TRUE
             /\ pc' = [pc EXCEPT !["timer"] = "TimerLoop"]
             /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, 
                             rdFrom, rdTo, rdResult, rdOk, wrState, wrBatch, 
                             wrBefore, wrOk, gcState, gcFrom, loaded, 
                             ackedInMem, readLevel, ackLevel, netFlaky, 
                             everInDb, committed, ackedGhost, dispatched, 
                             expiredGhost, gcVictims, reDispatch, dbAckAssert, 
                             writeDupAssert, scanRl, scanMax, scanIter, 
                             batchTasks >>

timer == TimerLoop

WLoop == /\ pc["writer"] = "WLoop"
         /\ \/ /\ \E B \in WriteBatches:
                    /\ wrBatch' = B
                    /\ wrBefore' = dbMaxReadLevel
                    /\ wrState' = "req"
               /\ pc' = [pc EXCEPT !["writer"] = "WResp"]
            \/ /\ pc' = [pc EXCEPT !["writer"] = "WDone"]
               /\ UNCHANGED <<wrState, wrBatch, wrBefore>>
         /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, rdFrom, 
                         rdTo, rdResult, rdOk, wrOk, gcState, gcFrom, loaded, 
                         ackedInMem, readLevel, ackLevel, notify, backoffTimer, 
                         netFlaky, everInDb, committed, ackedGhost, dispatched, 
                         expiredGhost, gcVictims, reDispatch, dbAckAssert, 
                         writeDupAssert, scanRl, scanMax, scanIter, batchTasks >>

WResp == /\ pc["writer"] = "WResp"
         /\ wrState = "resp"
         /\ wrState' = "idle"
         /\ IF wrOk
               THEN /\ writeDupAssert' =                 writeDupAssert
                                         \/ /\ (readLevel = wrBefore \/ MutNoDirectAddLevelCheck)
                                            /\ (Cardinality(loaded) + Cardinality(wrBatch) <= BatchSize
                                                \/ MutNoRoomCheck)
                                            /\ wrBatch \cap Outstanding /= {}
                    /\ IF /\ (readLevel = wrBefore \/ MutNoDirectAddLevelCheck)
                          /\ (Cardinality(loaded) + Cardinality(wrBatch) <= BatchSize
                              \/ MutNoRoomCheck)
                          /\ wrBatch \cap Outstanding = {}
                          THEN /\ readLevel' = SetMax(wrBatch)
                               /\ loaded' = (loaded \cup wrBatch)
                               /\ reDispatch' = (reDispatch \/ (wrBatch \cap dispatched) /= {})
                               /\ dispatched' = (dispatched \cup wrBatch)
                               /\ UNCHANGED notify
                          ELSE /\ notify' = TRUE
                               /\ UNCHANGED << loaded, readLevel, dispatched, 
                                               reDispatch >>
               ELSE /\ TRUE
                    /\ UNCHANGED << loaded, readLevel, notify, dispatched, 
                                    reDispatch, writeDupAssert >>
         /\ pc' = [pc EXCEPT !["writer"] = "WLoop"]
         /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, rdFrom, 
                         rdTo, rdResult, rdOk, wrBatch, wrBefore, wrOk, 
                         gcState, gcFrom, ackedInMem, ackLevel, backoffTimer, 
                         netFlaky, everInDb, committed, ackedGhost, 
                         expiredGhost, gcVictims, dbAckAssert, scanRl, scanMax, 
                         scanIter, batchTasks >>

WDone == /\ pc["writer"] = "WDone"
         /\ TRUE
         /\ pc' = [pc EXCEPT !["writer"] = "Done"]
         /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, rdFrom, 
                         rdTo, rdResult, rdOk, wrState, wrBatch, wrBefore, 
                         wrOk, gcState, gcFrom, loaded, ackedInMem, readLevel, 
                         ackLevel, notify, backoffTimer, netFlaky, everInDb, 
                         committed, ackedGhost, dispatched, expiredGhost, 
                         gcVictims, reDispatch, dbAckAssert, writeDupAssert, 
                         scanRl, scanMax, scanIter, batchTasks >>

writer == WLoop \/ WResp \/ WDone

DbReadLoop == /\ pc["dbRead"] = "DbReadLoop"
              /\ rdState = "req"
              /\ \/ /\ rdResult' = KeepLowest({l \in dbTasks : l >= rdFrom /\ l <= rdTo}, BatchSize)
                    /\ rdOk' = TRUE
                 \/ /\ netFlaky
                    /\ rdOk' = FALSE
                    /\ UNCHANGED rdResult
              /\ rdState' = "resp"
              /\ pc' = [pc EXCEPT !["dbRead"] = "DbReadLoop"]
              /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdFrom, 
                              rdTo, wrState, wrBatch, wrBefore, wrOk, gcState, 
                              gcFrom, loaded, ackedInMem, readLevel, ackLevel, 
                              notify, backoffTimer, netFlaky, everInDb, 
                              committed, ackedGhost, dispatched, expiredGhost, 
                              gcVictims, reDispatch, dbAckAssert, 
                              writeDupAssert, scanRl, scanMax, scanIter, 
                              batchTasks >>

dbRead == DbReadLoop

HealNet == /\ pc["heal"] = "HealNet"
           /\ netFlaky
           /\ netFlaky' = FALSE
           /\ pc' = [pc EXCEPT !["heal"] = "Done"]
           /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, 
                           rdFrom, rdTo, rdResult, rdOk, wrState, wrBatch, 
                           wrBefore, wrOk, gcState, gcFrom, loaded, ackedInMem, 
                           readLevel, ackLevel, notify, backoffTimer, everInDb, 
                           committed, ackedGhost, dispatched, expiredGhost, 
                           gcVictims, reDispatch, dbAckAssert, writeDupAssert, 
                           scanRl, scanMax, scanIter, batchTasks >>

heal == HealNet

DbWriteLoop == /\ pc["dbWrite"] = "DbWriteLoop"
               /\ wrState = "req"
               /\ \/ /\ dbTasks' = (dbTasks \cup wrBatch)
                     /\ everInDb' = (everInDb \cup wrBatch)
                     /\ committed' = (committed \cup wrBatch)
                     /\ wrOk' = TRUE
                  \/ /\ WriteErrors
                     /\ dbTasks' = (dbTasks \cup wrBatch)
                     /\ everInDb' = (everInDb \cup wrBatch)
                     /\ wrOk' = FALSE
                     /\ UNCHANGED committed
                  \/ /\ WriteErrors
                     /\ wrOk' = FALSE
                     /\ UNCHANGED <<dbTasks, everInDb, committed>>
               /\ dbMaxReadLevel' = SetMax(wrBatch)
               /\ wrState' = "resp"
               /\ pc' = [pc EXCEPT !["dbWrite"] = "DbWriteLoop"]
               /\ UNCHANGED << dbAckLevel, rdState, rdFrom, rdTo, rdResult, 
                               rdOk, wrBatch, wrBefore, gcState, gcFrom, 
                               loaded, ackedInMem, readLevel, ackLevel, notify, 
                               backoffTimer, netFlaky, ackedGhost, dispatched, 
                               expiredGhost, gcVictims, reDispatch, 
                               dbAckAssert, writeDupAssert, scanRl, scanMax, 
                               scanIter, batchTasks >>

dbWrite == DbWriteLoop

DbGcLoop == /\ pc["dbGc"] = "DbGcLoop"
            /\ gcState = "req"
            /\ \/ /\ LET victims == {l \in dbTasks : l <= gcFrom} IN
                       /\ dbTasks' = dbTasks \ victims
                       /\ gcVictims' = (gcVictims \cup victims)
               \/ /\ TRUE
                  /\ UNCHANGED <<dbTasks, gcVictims>>
            /\ gcState' = "resp"
            /\ pc' = [pc EXCEPT !["dbGc"] = "DbGcLoop"]
            /\ UNCHANGED << dbMaxReadLevel, dbAckLevel, rdState, rdFrom, rdTo, 
                            rdResult, rdOk, wrState, wrBatch, wrBefore, wrOk, 
                            gcFrom, loaded, ackedInMem, readLevel, ackLevel, 
                            notify, backoffTimer, netFlaky, everInDb, 
                            committed, ackedGhost, dispatched, expiredGhost, 
                            reDispatch, dbAckAssert, writeDupAssert, scanRl, 
                            scanMax, scanIter, batchTasks >>

dbGc == DbGcLoop

GcLoop == /\ pc["gc"] = "GcLoop"
          /\ ackLevel > NoLevel
          /\ gcFrom' = IF MutGcReadLevel THEN readLevel ELSE ackLevel
          /\ gcState' = "req"
          /\ pc' = [pc EXCEPT !["gc"] = "GcResp"]
          /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, rdFrom, 
                          rdTo, rdResult, rdOk, wrState, wrBatch, wrBefore, 
                          wrOk, loaded, ackedInMem, readLevel, ackLevel, 
                          notify, backoffTimer, netFlaky, everInDb, committed, 
                          ackedGhost, dispatched, expiredGhost, gcVictims, 
                          reDispatch, dbAckAssert, writeDupAssert, scanRl, 
                          scanMax, scanIter, batchTasks >>

GcResp == /\ pc["gc"] = "GcResp"
          /\ gcState = "resp"
          /\ gcState' = "idle"
          /\ pc' = [pc EXCEPT !["gc"] = "GcLoop"]
          /\ UNCHANGED << dbTasks, dbMaxReadLevel, dbAckLevel, rdState, rdFrom, 
                          rdTo, rdResult, rdOk, wrState, wrBatch, wrBefore, 
                          wrOk, gcFrom, loaded, ackedInMem, readLevel, 
                          ackLevel, notify, backoffTimer, netFlaky, everInDb, 
                          committed, ackedGhost, dispatched, expiredGhost, 
                          gcVictims, reDispatch, dbAckAssert, writeDupAssert, 
                          scanRl, scanMax, scanIter, batchTasks >>

gc == GcLoop \/ GcResp

AckLoop == /\ pc["acker"] = "AckLoop"
           /\ loaded \cap AckableLevels /= {}
           /\ \E l \in loaded \cap AckableLevels:
                LET ld == loaded \ {l} IN
                  LET ackd == ackedInMem \cup {l} IN
                    LET clear == {c \in ackd : \A m \in ld : c < m} IN
                      LET rest == ackd \ clear IN
                        LET al1 == IF clear /= {} THEN SetMax(clear) ELSE ackLevel IN
                          LET drained == /\ ((ld = {} /\ rest = {}) \/ MutDrainedAckIgnoresOutstanding)
                                         /\ readLevel >= dbMaxReadLevel IN
                            LET al2 == IF drained THEN readLevel ELSE al1 IN
                              /\ loaded' = ld
                              /\ ackedInMem' = rest
                              /\ ackLevel' = al2
                              /\ ackedGhost' = (ackedGhost \cup {l})
                              /\ IF Cardinality(ld) = ReloadAt /\ ~MutNoReloadSignal
                                    THEN /\ notify' = TRUE
                                    ELSE /\ TRUE
                                         /\ UNCHANGED notify
                              /\ dbAckAssert' = (dbAckAssert \/ al2 < dbAckLevel)
                              /\ dbAckLevel' = (IF al2 < dbAckLevel /\ ~MutDbAckBackwards
                                                THEN dbAckLevel ELSE al2)
           /\ pc' = [pc EXCEPT !["acker"] = "AckLoop"]
           /\ UNCHANGED << dbTasks, dbMaxReadLevel, rdState, rdFrom, rdTo, 
                           rdResult, rdOk, wrState, wrBatch, wrBefore, wrOk, 
                           gcState, gcFrom, readLevel, backoffTimer, netFlaky, 
                           everInDb, committed, dispatched, expiredGhost, 
                           gcVictims, reDispatch, writeDupAssert, scanRl, 
                           scanMax, scanIter, batchTasks >>

acker == AckLoop

Next == reader \/ timer \/ writer \/ dbRead \/ heal \/ dbWrite \/ dbGc
           \/ gc \/ acker

Spec == /\ Init /\ [][Next]_vars
        /\ WF_vars(reader)
        /\ WF_vars(timer)
        /\ WF_vars(writer)
        /\ WF_vars(dbRead)
        /\ WF_vars(heal)
        /\ WF_vars(dbWrite)
        /\ WF_vars(dbGc)
        /\ WF_vars(acker)

\* END TRANSLATION

---------------------------------------------------------------------------
(* Fairness *)

(* The acker step that acks specifically level l. *)
AckOf(l) == AckLoop /\ l \in loaded /\ l \notin loaded'

(* Weak fairness on the acker process is not enough: it would let the acker
   ack a re-dispatched task forever and starve another loaded one. The
   intended assumption is that every loaded task is eventually acked, which
   is per-level strong fairness. *)
AckerFairness == \A l \in AckableLevels : SF_vars(AckOf(l))

(* "The network eventually stops timing out" is modeled by the netFlaky flag
   and the (weakly fair) heal process, not by fairness on the read action:
   strong fairness on read success would still allow a behavior where every
   other read fails forever, which is enough to keep the reader retrying and
   makes ReaderQuiesce unprovable. Write and GC success are NOT assumed. *)
PriSpec == Spec /\ AckerFairness

---------------------------------------------------------------------------
(* Invariants *)

TypeInv ==
  /\ loaded \subseteq Levels
  /\ ackedInMem \subseteq Levels
  /\ loaded \cap ackedInMem = {}
  /\ readLevel \in NoLevel..MaxLevel
  /\ ackLevel \in NoLevel..MaxLevel
  /\ dbAckLevel \in NoLevel..MaxLevel
  /\ dbMaxReadLevel \in NoLevel..MaxLevel
  /\ dbTasks \subseteq everInDb
  /\ committed \subseteq everInDb
  /\ wrBatch \subseteq Levels

\* in-memory entries are exactly within (ackLevel, readLevel]
MemWindow == \A l \in Outstanding : ackLevel < l /\ l <= readLevel

AckBelowRead == ackLevel <= readLevel

\* The reader must never move its read level (and hence its ack level) past
\* an id that could still land in the db: everything at or below the db's max
\* read level has already been written (or its write has failed and its id is
\* burned). This is the invariant that makes reading a range and finding it
\* empty a safe conclusion.
ReadBelowMaxRead == readLevel <= dbMaxReadLevel /\ dbAckLevel <= dbMaxReadLevel

\* tasks we hold in memory are still in the db (GC hasn't deleted them)
LoadedInDb == loaded \subseteq dbTasks

\* processTaskBatch reads at most a page when at most ReloadAt are loaded, and
\* signalNewTasks direct-adds only up to BatchSize.
LoadedBounded == Cardinality(loaded) <= BatchSize + ReloadAt

\* Safety: the ack level never passes a committed task that was not acked --
\* neither in memory nor as persisted. (Rows from failed writes carry no
\* guarantee: the caller re-submits with a new id. Tasks dropped as expired
\* leave the committed set when the reader decides they expired.)
NoAckSkipped ==
  /\ \A l \in committed : (l <= ackLevel) => (l \in ackedGhost)
  /\ \A l \in committed : (l <= dbAckLevel) => (l \in ackedGhost)

\* Safety: a committed task is never deleted from the db unless it was acked.
GCOnlyAcked == (gcVictims \cap committed) \subseteq ackedGhost

\* No task is ever handed to the matcher twice: the reader dedups against
\* outstandingTasks, and (since e0a4751c) against the ack level for entries
\* outstandingTasks has already dropped. Duplicate dispatch is allowed by the
\* at-least-once contract, but it is wasted work and it is what the read/ack
\* levels moving backwards causes.
NoReDispatch == ~reDispatch

\* The two softasserts on this path never fire:
\* db.go: "ack level in subqueue should not move backwards"
NoDbAckBackwards == ~dbAckAssert
\* pri_task_reader.go: "newly-written task already present in outstanding tasks"
NoWriteDup == ~writeDupAssert

---------------------------------------------------------------------------
(* Temporal properties *)

\* The levels never move backwards. Not required for at-least-once delivery,
\* but a backwards move means re-reading and re-dispatching tasks we already
\* handled -- and, persisted, it means the next load of this queue re-reads
\* everything back to the regressed level.
ReadLevelMonotonic   == [][readLevel'  >= readLevel]_vars
AckLevelMonotonic    == [][ackLevel'   >= ackLevel]_vars
DbAckLevelMonotonic  == [][dbAckLevel' >= dbAckLevel]_vars

\* Liveness: every committed task is eventually acked.
AllTasksAcked == <>(\A l \in committed : l \in ackedGhost)

\* Liveness: the reader eventually holds nothing and everything committed is
\* acked, and stays that way.
EventuallyDrained ==
  <>[](loaded = {} /\ ackedInMem = {} /\ \A l \in committed : l \in ackedGhost)

\* Liveness: the reader eventually stops issuing reads and signalling itself.
\* Catches both a stuck reader and a busy re-read loop.
ReaderQuiesce == <>[](rdState = "idle" /\ ~notify /\ ~backoffTimer)

\* Liveness: the reader eventually knows it has read to the end, and pushes
\* that ack level to the db (so the next load of this queue starts at the end
\* instead of re-scanning). Only holds if writes don't fail: nothing signals
\* the reader after a failed CreateTasks, even though it moved the db's max
\* read level (findings.md #1), so it is checked with WriteErrors = FALSE.
EventuallyCaughtUp ==
  <>[](readLevel = dbMaxReadLevel /\ dbAckLevel = dbMaxReadLevel)

===========================================================================

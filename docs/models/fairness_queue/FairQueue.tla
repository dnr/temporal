---------------------------- MODULE FairQueue ----------------------------
(***************************************************************************)
(* PlusCal model of matching's fair task queue reader/writer.              *)
(* Source: service/matching/fair_task_{reader,writer}.go                   *)
(* Milestones and scope decisions: see plan.md. Currently at: M3.          *)
(*                                                                         *)
(* Key abstractions:                                                       *)
(*                                                                         *)
(* - Fair levels <pass,id> are modeled as plain integers 1..MaxLevel.      *)
(*   The reader/writer logic is order-invariant: it only ever compares     *)
(*   levels, so any real execution maps to an integer execution that       *)
(*   preserves order. The stride counter is abstracted: the writer picks   *)
(*   any unused levels above the pinned ack level, which is the only       *)
(*   property the reader relies on (writes may land below or above         *)
(*   readLevel, never at-or-below the pinned ackLevel).                    *)
(*                                                                         *)
(* - Go guards reader state with tr.lock; every locked critical section    *)
(*   is one atomic PlusCal step (one label). Lock release points (network  *)
(*   calls) are label boundaries, which is where processes interleave.     *)
(*   mergeTasksLocked is the pure operator MergeResult so that each call   *)
(*   site can atomically compose it with its own pre/post logic.           *)
(*                                                                         *)
(* - outstandingTasks (treemap level -> task|nil) is split into two sets:  *)
(*   "loaded" (unacked *internalTask entries) and "ackedInMem" (nil        *)
(*   entries). loadedTasks == Cardinality(loaded).                         *)
(*                                                                         *)
(* - A task is ackable as soon as it is merged into "loaded"; the matcher  *)
(*   handoff is not modeled (see plan.md M7).                              *)
(*                                                                         *)
(* - The DB is reached via request/response channels (rdState.., wrState..,*)
(*   gcState..) modeled as separate processes. All calls succeed for now.  *)
(*                                                                         *)
(* - GC may fire whenever the ack level is above zero (the numToGC/time    *)
(*   trigger conditions are abstracted away); it is unfair, so liveness    *)
(*   cannot depend on GC running. The delete batch size is not modeled.    *)
(*                                                                         *)
(* - The defensive "fair reader stuck" softassert+repair in mergeTasks is  *)
(*   modeled as a detection flag + invariant (NoStuck), not as a repair,   *)
(*   so TLC reports the stuck state instead of the backstop masking it.    *)
(*                                                                         *)
(* Mutation flags "Mut...": each TRUE re-introduces a (mostly historical)  *)
(* bug to validate that the properties catch it; all FALSE models the      *)
(* current code. See run.sh.                                               *)
(***************************************************************************)
EXTENDS Naturals, FiniteSets, Sequences, TLC

CONSTANTS
  MaxLevel,        \* task levels are 1..MaxLevel
  BatchTarget,     \* config.GetTasksBatchSize: max tasks to keep loaded
  ReloadAt,        \* config.GetTasksReloadAt: read more when loaded <= this
  WBatchMax,       \* max tasks per write batch
  \* --- mutation flags ---
  MutAtEndOnMiddleRead,  \* seeded: treat every read as reaching the end
  MutAckPastLoaded,      \* seeded: ack level advance ignores loaded tasks
  MutNoWriteBuffering,   \* 12e7c43a: merge writes directly during a pending read
  MutResetReadLevelOnEmptyMerge, \* f534e74e: readLevel := ackLevel when merge is empty
  MutKeepEvictedAcks,    \* 8ca7b640 #4: don't evict acks above new readLevel
  MutNoPin,              \* fairness.md problem 1: no ack level pinning during writes
  MutGcReadLevel         \* seeded: GC deletes up to readLevel instead of ackLevel

ASSUME ReloadAt < BatchTarget
ASSUME MaxLevel >= 1

Levels  == 1..MaxLevel
NoLevel == 0

\* merge modes (mergeMode in fair_task_reader.go)
MMiddle == "readMiddle"
MToEnd  == "readToEnd"
MWrite  == "write"

SetMax(S) == CHOOSE x \in S : \A y \in S : y <= x
\* The n lowest elements of S (all of S if it has <= n elements).
KeepLowest(S, n) == {l \in S : Cardinality({m \in S : m <= l}) <= n}

\* ackLevelPinnedLocked, with the MutNoPin mutation applied
PinActive(p) == p /\ ~MutNoPin

(***************************************************************************)
(* mergeTasksLocked as a pure function: given the current ack-manager      *)
(* state and a set of incoming task levels, compute the new state.         *)
(*   inc:       levels just read or written                                *)
(*   pinnedNow: ackLevelPinnedLocked() at the time of the merge            *)
(* Returns a record; .stuck is the condition of the defensive "fair        *)
(* reader stuck" check (the call site must additionally rule out a         *)
(* pending read / backoff timer).                                          *)
(***************************************************************************)
MergeResult(loaded0, acks0, rl0, al0, atEnd0, inc, mode, pinnedNow) ==
  LET
    \* filter incoming: skip at-or-below ackLevel (raced with acks); on
    \* write when not atEnd, skip above readLevel (unknown range in
    \* between); skip levels we already have
    filtered == {l \in inc :
                   /\ l > al0
                   /\ ~(mode = MWrite /\ ~atEnd0 /\ l > rl0)
                   /\ l \notin (loaded0 \cup acks0)}
    \* loaded tasks plus incoming, keep the BatchTarget lowest
    merged == loaded0 \cup filtered
    kept   == KeepLowest(merged, BatchTarget)
    \* "if we have any tasks at all in memory, set readLevel to the max of
    \* that set"; if merged is empty leave readLevel unchanged (f534e74e)
    newRL == IF kept /= {} THEN SetMax(kept)
             ELSE IF MutResetReadLevelOnEmptyMerge THEN al0
             ELSE rl0
    \* evict acked (nil) entries above the new read level (8ca7b640 #4)
    keptAcks == IF MutKeepEvictedAcks THEN acks0
                ELSE {l \in acks0 : l <= newRL}
    \* evictedAnyTasks: any merged entry beyond BatchTarget (loaded
    \* evictions and ignored new tasks), or any evicted ack
    evictedAny == \/ (merged \ kept) /= {}
                  \/ keptAcks /= acks0
    \* advanceAckLevelLocked: pop acked entries below the lowest loaded
    \* task, unless the ack level is pinned
    clear == IF PinActive(pinnedNow) THEN {}
             ELSE {c \in keptAcks : MutAckPastLoaded \/ \A m \in kept : c < m}
    newAtEnd == IF MutAtEndOnMiddleRead /\ mode \in {MMiddle, MToEnd} THEN TRUE
                ELSE IF (mode = MMiddle) \/ evictedAny THEN FALSE
                ELSE IF mode = MToEnd THEN TRUE
                ELSE atEnd0
  IN [
    loaded |-> kept,
    acks   |-> keptAcks \ clear,
    rl     |-> newRL,
    al     |-> IF clear = {} THEN al0 ELSE SetMax(clear),
    atEnd  |-> newAtEnd,
    stuck  |-> mode = MWrite /\ ~newAtEnd /\ kept = {}
  ]

(* --algorithm FairQueue

variables
  \* ---- database ----
  dbTasks \in SUBSET Levels,   \* nondeterministic initial backlog
  \* ---- read RPC channel: reader -> db ----
  rdState  = "idle",           \* idle -> req -> resp -> idle
  rdFrom   = NoLevel,
  rdMax    = 0,
  rdResult = {},
  \* ---- write RPC channel: writer -> db ----
  wrState  = "idle",           \* idle -> req -> resp -> idle
  wrBatch  = {},
  \* ---- gc RPC channel: gc -> db ----
  gcState  = "idle",           \* idle -> req -> resp -> idle
  gcFrom   = NoLevel,
  \* ---- reader state (fields of fairTaskReader, guarded by tr.lock) ----
  loaded       = {},           \* levels of unacked tasks in memory
  ackedInMem   = {},           \* levels of acked (nil) placeholder entries
  readLevel    = NoLevel,
  ackLevel     = NoLevel,
  atEnd        = FALSE,
  readPending  = TRUE,         \* Start() calls maybeReadTasksLocked
  newlyWritten = {},           \* newlyWrittenTasks: writes held during a read
  pinned       = FALSE,        \* ackLevelPinnedByWriter
  \* ---- writer state ----
  usedLevels = dbTasks,        \* levels ever allocated to a task (ids are unique)
  \* ---- ghost state (not part of the implementation) ----
  everInDb   = dbTasks,        \* every level ever present in dbTasks
  ackedGhost = {},             \* every level ever acked
  gcVictims  = {},             \* every level ever deleted by GC
  stuckFlag  = FALSE;          \* defensive "fair reader stuck" check fired

define
  Outstanding    == loaded \cup ackedInMem
  LoadedCount    == Cardinality(loaded)
  \* shouldReadMoreLocked
  ShouldReadMore == ~atEnd /\ LoadedCount <= ReloadAt
  \* levels the writer may still allocate: unused, above the (pinned) ack level
  AvailableLevels == {l \in Levels : l \notin usedLevels /\ l > ackLevel}
  WriteBatches    == {B \in SUBSET AvailableLevels :
                        B /= {} /\ Cardinality(B) <= WBatchMax}
end define;

\* Assign the results of MergeResult r to the reader state.
macro applyMerge(r) begin
  loaded     := r.loaded;
  ackedInMem := r.acks;
  readLevel  := r.rl;
  ackLevel   := r.al;
  atEnd      := r.atEnd;
end macro;

\* readTasksImpl: loop reading batches while shouldReadMoreLocked.
fair process reader = "reader"
begin
RWait:
  while TRUE do
    await readPending;
RCheck:
    \* top of readTasksImpl loop: check and capture under lock, then
    \* release the lock for the network call
    if ShouldReadMore then
      rdFrom  := readLevel + 1;      \* readLevel.max(minFairLevel).inc()
      rdMax   := BatchTarget - LoadedCount;
      rdState := "req";
RResp:
      await rdState = "resp";
      rdState := "idle";
      \* got fewer than asked for => we hit the end (mergeReadToEnd)
      with mode = IF Cardinality(rdResult) < rdMax THEN MToEnd ELSE MMiddle,
           r    = MergeResult(loaded, ackedInMem, readLevel, ackLevel, atEnd,
                              rdResult, mode, pinned \/ newlyWritten /= {})
      do
        applyMerge(r);
      end with;
      goto RCheck;
    else
      \* exit path of readTasksImpl, one critical section: clear
      \* readPending, merge tasks written while the read was pending,
      \* advance ack level, then re-check (final maybeReadTasksLocked,
      \* added in 26d9a561)
      if newlyWritten /= {} then
        with r = MergeResult(loaded, ackedInMem, readLevel, ackLevel, atEnd,
                             newlyWritten, MWrite, pinned)
        do
          applyMerge(r);
          newlyWritten := {};
          readPending  := ~r.atEnd /\ Cardinality(r.loaded) <= ReloadAt;
        end with;
      else
        readPending := FALSE;
      end if;
    end if;
  end while;
end process;

\* taskWriterLoop/writeBatch: pin ack level, pick levels, write to db,
\* merge (or buffer) the written tasks, unpin.
fair process writer = "writer"
begin
WLoop:
  while TRUE do
    either
      \* getAndPinAckLevels + pickPasses: levels must be above the pinned
      \* ack level; ids are never reused
      with B \in WriteBatches do
        pinned     := TRUE;
        wrBatch    := B;
        usedLevels := usedLevels \cup B;
        wrState    := "req";
      end with;
WResp:
      await wrState = "resp";
      wrState := "idle";
      \* wroteNewTasks -> mergeTasks(mergeWrite): if a read is pending,
      \* hold the tasks in newlyWrittenTasks (12e7c43a); else merge
      \* directly (this call site has the defensive stuck check)
      if readPending /\ ~MutNoWriteBuffering then
        newlyWritten := newlyWritten \cup wrBatch;
      else
        with r = MergeResult(loaded, ackedInMem, readLevel, ackLevel, atEnd,
                             wrBatch, MWrite, TRUE)
        do
          applyMerge(r);
          stuckFlag := stuckFlag \/ (r.stuck /\ ~readPending);
        end with;
      end if;
WUnpin:
      \* unpinAckLevel (separate critical section, via defer): clear the
      \* pin and advance the ack level (still pinned if a read is holding
      \* newlyWritten tasks)
      pinned := FALSE;
      with clear = IF PinActive(newlyWritten /= {}) THEN {}
                   ELSE {c \in ackedInMem :
                           MutAckPastLoaded \/ \A m \in loaded : c < m}
      do
        ackedInMem := ackedInMem \ clear;
        if clear /= {} then
          ackLevel := SetMax(clear);
        end if;
      end with;
    or
      \* no more tasks will be written
      goto WDone;
    end either;
  end while;
WDone:
  skip;
end process;

\* The database, serving reads and writes concurrently. M2: all succeed.
fair process dbRead = "dbRead"
begin
DbReadLoop:
  while TRUE do
    await rdState = "req";
    rdResult := KeepLowest({l \in dbTasks : l >= rdFrom}, rdMax);
    rdState  := "resp";
  end while;
end process;

fair process dbWrite = "dbWrite"
begin
DbWriteLoop:
  while TRUE do
    await wrState = "req";
    dbTasks  := dbTasks \cup wrBatch;
    everInDb := everInDb \cup wrBatch;
    wrState  := "resp";
  end while;
end process;

fair process dbGc = "dbGc"
begin
DbGcLoop:
  while TRUE do
    await gcState = "req";
    \* CompleteFairTasksLessThan(gcFrom.inc()): delete tasks <= gcFrom.
    \* The delete batch size limit is not modeled (it only splits the
    \* delete across calls; deletes are idempotent).
    with victims = {l \in dbTasks : l <= gcFrom} do
      dbTasks   := dbTasks \ victims;
      gcVictims := gcVictims \cup victims;
    end with;
    gcState := "resp";
  end while;
end process;

\* GC: maybeGCLocked/doGC. Triggered nondeterministically whenever the ack
\* level has moved (the numToGC/time trigger conditions are abstracted to
\* "may run at any such time"). inGC (single outstanding GC) is implied by
\* this being one sequential process. Deliberately NOT fair: liveness must
\* not depend on GC running.
process gc = "gc"
begin
GcLoop:
  while TRUE do
    \* doGC captures ackLevel under lock, then calls the db unlocked
    await ackLevel > NoLevel;
    gcFrom  := IF MutGcReadLevel THEN readLevel ELSE ackLevel;
    gcState := "req";
GcResp:
    await gcState = "resp";
    gcState := "idle";
  end while;
end process;

\* The acker: eventually acks every loaded task, in any order.
\* One step = completeTaskLocked: mark acked, advanceAckLevelLocked,
\* maybeReadTasksLocked.
fair process acker = "acker"
begin
AckLoop:
  while TRUE do
    await loaded /= {};
    with
      l     \in loaded,
      ld    =   loaded \ {l},
      ackd  =   ackedInMem \cup {l},
      clear =   IF PinActive(pinned \/ newlyWritten /= {}) THEN {}
                ELSE {c \in ackd : MutAckPastLoaded \/ \A m \in ld : c < m}
    do
      loaded     := ld;
      ackedInMem := ackd \ clear;
      if clear /= {} then
        ackLevel := SetMax(clear);
      end if;
      ackedGhost := ackedGhost \cup {l};
      \* maybeReadTasksLocked (inlined with post-ack values)
      if ~readPending /\ ~atEnd /\ Cardinality(ld) <= ReloadAt then
        readPending := TRUE;
      end if;
    end with;
  end while;
end process;

end algorithm; *)

\* BEGIN TRANSLATION
VARIABLES pc, dbTasks, rdState, rdFrom, rdMax, rdResult, wrState, wrBatch, 
          gcState, gcFrom, loaded, ackedInMem, readLevel, ackLevel, atEnd, 
          readPending, newlyWritten, pinned, usedLevels, everInDb, ackedGhost, 
          gcVictims, stuckFlag

(* define statement *)
Outstanding    == loaded \cup ackedInMem
LoadedCount    == Cardinality(loaded)

ShouldReadMore == ~atEnd /\ LoadedCount <= ReloadAt

AvailableLevels == {l \in Levels : l \notin usedLevels /\ l > ackLevel}
WriteBatches    == {B \in SUBSET AvailableLevels :
                      B /= {} /\ Cardinality(B) <= WBatchMax}


vars == << pc, dbTasks, rdState, rdFrom, rdMax, rdResult, wrState, wrBatch, 
           gcState, gcFrom, loaded, ackedInMem, readLevel, ackLevel, atEnd, 
           readPending, newlyWritten, pinned, usedLevels, everInDb, 
           ackedGhost, gcVictims, stuckFlag >>

ProcSet == {"reader"} \cup {"writer"} \cup {"dbRead"} \cup {"dbWrite"} \cup {"dbGc"} \cup {"gc"} \cup {"acker"}

Init == (* Global variables *)
        /\ dbTasks \in SUBSET Levels
        /\ rdState = "idle"
        /\ rdFrom = NoLevel
        /\ rdMax = 0
        /\ rdResult = {}
        /\ wrState = "idle"
        /\ wrBatch = {}
        /\ gcState = "idle"
        /\ gcFrom = NoLevel
        /\ loaded = {}
        /\ ackedInMem = {}
        /\ readLevel = NoLevel
        /\ ackLevel = NoLevel
        /\ atEnd = FALSE
        /\ readPending = TRUE
        /\ newlyWritten = {}
        /\ pinned = FALSE
        /\ usedLevels = dbTasks
        /\ everInDb = dbTasks
        /\ ackedGhost = {}
        /\ gcVictims = {}
        /\ stuckFlag = FALSE
        /\ pc = [self \in ProcSet |-> CASE self = "reader" -> "RWait"
                                        [] self = "writer" -> "WLoop"
                                        [] self = "dbRead" -> "DbReadLoop"
                                        [] self = "dbWrite" -> "DbWriteLoop"
                                        [] self = "dbGc" -> "DbGcLoop"
                                        [] self = "gc" -> "GcLoop"
                                        [] self = "acker" -> "AckLoop"]

RWait == /\ pc["reader"] = "RWait"
         /\ readPending
         /\ pc' = [pc EXCEPT !["reader"] = "RCheck"]
         /\ UNCHANGED << dbTasks, rdState, rdFrom, rdMax, rdResult, wrState, 
                         wrBatch, gcState, gcFrom, loaded, ackedInMem, 
                         readLevel, ackLevel, atEnd, readPending, newlyWritten, 
                         pinned, usedLevels, everInDb, ackedGhost, gcVictims, 
                         stuckFlag >>

RCheck == /\ pc["reader"] = "RCheck"
          /\ IF ShouldReadMore
                THEN /\ rdFrom' = readLevel + 1
                     /\ rdMax' = BatchTarget - LoadedCount
                     /\ rdState' = "req"
                     /\ pc' = [pc EXCEPT !["reader"] = "RResp"]
                     /\ UNCHANGED << loaded, ackedInMem, readLevel, ackLevel, 
                                     atEnd, readPending, newlyWritten >>
                ELSE /\ IF newlyWritten /= {}
                           THEN /\ LET r == MergeResult(loaded, ackedInMem, readLevel, ackLevel, atEnd,
                                                        newlyWritten, MWrite, pinned) IN
                                     /\ loaded' = r.loaded
                                     /\ ackedInMem' = r.acks
                                     /\ readLevel' = r.rl
                                     /\ ackLevel' = r.al
                                     /\ atEnd' = r.atEnd
                                     /\ newlyWritten' = {}
                                     /\ readPending' = (~r.atEnd /\ Cardinality(r.loaded) <= ReloadAt)
                           ELSE /\ readPending' = FALSE
                                /\ UNCHANGED << loaded, ackedInMem, readLevel, 
                                                ackLevel, atEnd, newlyWritten >>
                     /\ pc' = [pc EXCEPT !["reader"] = "RWait"]
                     /\ UNCHANGED << rdState, rdFrom, rdMax >>
          /\ UNCHANGED << dbTasks, rdResult, wrState, wrBatch, gcState, gcFrom, 
                          pinned, usedLevels, everInDb, ackedGhost, gcVictims, 
                          stuckFlag >>

RResp == /\ pc["reader"] = "RResp"
         /\ rdState = "resp"
         /\ rdState' = "idle"
         /\ LET mode == IF Cardinality(rdResult) < rdMax THEN MToEnd ELSE MMiddle IN
              LET r == MergeResult(loaded, ackedInMem, readLevel, ackLevel, atEnd,
                                   rdResult, mode, pinned \/ newlyWritten /= {}) IN
                /\ loaded' = r.loaded
                /\ ackedInMem' = r.acks
                /\ readLevel' = r.rl
                /\ ackLevel' = r.al
                /\ atEnd' = r.atEnd
         /\ pc' = [pc EXCEPT !["reader"] = "RCheck"]
         /\ UNCHANGED << dbTasks, rdFrom, rdMax, rdResult, wrState, wrBatch, 
                         gcState, gcFrom, readPending, newlyWritten, pinned, 
                         usedLevels, everInDb, ackedGhost, gcVictims, 
                         stuckFlag >>

reader == RWait \/ RCheck \/ RResp

WLoop == /\ pc["writer"] = "WLoop"
         /\ \/ /\ \E B \in WriteBatches:
                    /\ pinned' = TRUE
                    /\ wrBatch' = B
                    /\ usedLevels' = (usedLevels \cup B)
                    /\ wrState' = "req"
               /\ pc' = [pc EXCEPT !["writer"] = "WResp"]
            \/ /\ pc' = [pc EXCEPT !["writer"] = "WDone"]
               /\ UNCHANGED <<wrState, wrBatch, pinned, usedLevels>>
         /\ UNCHANGED << dbTasks, rdState, rdFrom, rdMax, rdResult, gcState, 
                         gcFrom, loaded, ackedInMem, readLevel, ackLevel, 
                         atEnd, readPending, newlyWritten, everInDb, 
                         ackedGhost, gcVictims, stuckFlag >>

WResp == /\ pc["writer"] = "WResp"
         /\ wrState = "resp"
         /\ wrState' = "idle"
         /\ IF readPending /\ ~MutNoWriteBuffering
               THEN /\ newlyWritten' = (newlyWritten \cup wrBatch)
                    /\ UNCHANGED << loaded, ackedInMem, readLevel, ackLevel, 
                                    atEnd, stuckFlag >>
               ELSE /\ LET r == MergeResult(loaded, ackedInMem, readLevel, ackLevel, atEnd,
                                            wrBatch, MWrite, TRUE) IN
                         /\ loaded' = r.loaded
                         /\ ackedInMem' = r.acks
                         /\ readLevel' = r.rl
                         /\ ackLevel' = r.al
                         /\ atEnd' = r.atEnd
                         /\ stuckFlag' = (stuckFlag \/ (r.stuck /\ ~readPending))
                    /\ UNCHANGED newlyWritten
         /\ pc' = [pc EXCEPT !["writer"] = "WUnpin"]
         /\ UNCHANGED << dbTasks, rdState, rdFrom, rdMax, rdResult, wrBatch, 
                         gcState, gcFrom, readPending, pinned, usedLevels, 
                         everInDb, ackedGhost, gcVictims >>

WUnpin == /\ pc["writer"] = "WUnpin"
          /\ pinned' = FALSE
          /\ LET clear == IF PinActive(newlyWritten /= {}) THEN {}
                          ELSE {c \in ackedInMem :
                                  MutAckPastLoaded \/ \A m \in loaded : c < m} IN
               /\ ackedInMem' = ackedInMem \ clear
               /\ IF clear /= {}
                     THEN /\ ackLevel' = SetMax(clear)
                     ELSE /\ TRUE
                          /\ UNCHANGED ackLevel
          /\ pc' = [pc EXCEPT !["writer"] = "WLoop"]
          /\ UNCHANGED << dbTasks, rdState, rdFrom, rdMax, rdResult, wrState, 
                          wrBatch, gcState, gcFrom, loaded, readLevel, atEnd, 
                          readPending, newlyWritten, usedLevels, everInDb, 
                          ackedGhost, gcVictims, stuckFlag >>

WDone == /\ pc["writer"] = "WDone"
         /\ TRUE
         /\ pc' = [pc EXCEPT !["writer"] = "Done"]
         /\ UNCHANGED << dbTasks, rdState, rdFrom, rdMax, rdResult, wrState, 
                         wrBatch, gcState, gcFrom, loaded, ackedInMem, 
                         readLevel, ackLevel, atEnd, readPending, newlyWritten, 
                         pinned, usedLevels, everInDb, ackedGhost, gcVictims, 
                         stuckFlag >>

writer == WLoop \/ WResp \/ WUnpin \/ WDone

DbReadLoop == /\ pc["dbRead"] = "DbReadLoop"
              /\ rdState = "req"
              /\ rdResult' = KeepLowest({l \in dbTasks : l >= rdFrom}, rdMax)
              /\ rdState' = "resp"
              /\ pc' = [pc EXCEPT !["dbRead"] = "DbReadLoop"]
              /\ UNCHANGED << dbTasks, rdFrom, rdMax, wrState, wrBatch, 
                              gcState, gcFrom, loaded, ackedInMem, readLevel, 
                              ackLevel, atEnd, readPending, newlyWritten, 
                              pinned, usedLevels, everInDb, ackedGhost, 
                              gcVictims, stuckFlag >>

dbRead == DbReadLoop

DbWriteLoop == /\ pc["dbWrite"] = "DbWriteLoop"
               /\ wrState = "req"
               /\ dbTasks' = (dbTasks \cup wrBatch)
               /\ everInDb' = (everInDb \cup wrBatch)
               /\ wrState' = "resp"
               /\ pc' = [pc EXCEPT !["dbWrite"] = "DbWriteLoop"]
               /\ UNCHANGED << rdState, rdFrom, rdMax, rdResult, wrBatch, 
                               gcState, gcFrom, loaded, ackedInMem, readLevel, 
                               ackLevel, atEnd, readPending, newlyWritten, 
                               pinned, usedLevels, ackedGhost, gcVictims, 
                               stuckFlag >>

dbWrite == DbWriteLoop

DbGcLoop == /\ pc["dbGc"] = "DbGcLoop"
            /\ gcState = "req"
            /\ LET victims == {l \in dbTasks : l <= gcFrom} IN
                 /\ dbTasks' = dbTasks \ victims
                 /\ gcVictims' = (gcVictims \cup victims)
            /\ gcState' = "resp"
            /\ pc' = [pc EXCEPT !["dbGc"] = "DbGcLoop"]
            /\ UNCHANGED << rdState, rdFrom, rdMax, rdResult, wrState, wrBatch, 
                            gcFrom, loaded, ackedInMem, readLevel, ackLevel, 
                            atEnd, readPending, newlyWritten, pinned, 
                            usedLevels, everInDb, ackedGhost, stuckFlag >>

dbGc == DbGcLoop

GcLoop == /\ pc["gc"] = "GcLoop"
          /\ ackLevel > NoLevel
          /\ gcFrom' = IF MutGcReadLevel THEN readLevel ELSE ackLevel
          /\ gcState' = "req"
          /\ pc' = [pc EXCEPT !["gc"] = "GcResp"]
          /\ UNCHANGED << dbTasks, rdState, rdFrom, rdMax, rdResult, wrState, 
                          wrBatch, loaded, ackedInMem, readLevel, ackLevel, 
                          atEnd, readPending, newlyWritten, pinned, usedLevels, 
                          everInDb, ackedGhost, gcVictims, stuckFlag >>

GcResp == /\ pc["gc"] = "GcResp"
          /\ gcState = "resp"
          /\ gcState' = "idle"
          /\ pc' = [pc EXCEPT !["gc"] = "GcLoop"]
          /\ UNCHANGED << dbTasks, rdState, rdFrom, rdMax, rdResult, wrState, 
                          wrBatch, gcFrom, loaded, ackedInMem, readLevel, 
                          ackLevel, atEnd, readPending, newlyWritten, pinned, 
                          usedLevels, everInDb, ackedGhost, gcVictims, 
                          stuckFlag >>

gc == GcLoop \/ GcResp

AckLoop == /\ pc["acker"] = "AckLoop"
           /\ loaded /= {}
           /\ \E l \in loaded:
                LET ld == loaded \ {l} IN
                  LET ackd == ackedInMem \cup {l} IN
                    LET clear == IF PinActive(pinned \/ newlyWritten /= {}) THEN {}
                                 ELSE {c \in ackd : MutAckPastLoaded \/ \A m \in ld : c < m} IN
                      /\ loaded' = ld
                      /\ ackedInMem' = ackd \ clear
                      /\ IF clear /= {}
                            THEN /\ ackLevel' = SetMax(clear)
                            ELSE /\ TRUE
                                 /\ UNCHANGED ackLevel
                      /\ ackedGhost' = (ackedGhost \cup {l})
                      /\ IF ~readPending /\ ~atEnd /\ Cardinality(ld) <= ReloadAt
                            THEN /\ readPending' = TRUE
                            ELSE /\ TRUE
                                 /\ UNCHANGED readPending
           /\ pc' = [pc EXCEPT !["acker"] = "AckLoop"]
           /\ UNCHANGED << dbTasks, rdState, rdFrom, rdMax, rdResult, wrState, 
                           wrBatch, gcState, gcFrom, readLevel, atEnd, 
                           newlyWritten, pinned, usedLevels, everInDb, 
                           gcVictims, stuckFlag >>

acker == AckLoop

Next == reader \/ writer \/ dbRead \/ dbWrite \/ dbGc \/ gc \/ acker

Spec == /\ Init /\ [][Next]_vars
        /\ WF_vars(reader)
        /\ WF_vars(writer)
        /\ WF_vars(dbRead)
        /\ WF_vars(dbWrite)
        /\ WF_vars(dbGc)
        /\ WF_vars(acker)

\* END TRANSLATION

---------------------------------------------------------------------------
(* Fairness *)

(* The acker step that acks specifically level l. *)
AckOf(l) == AckLoop /\ l \in loaded /\ l \notin loaded'

(* Weak fairness on the acker process is not enough: if a task is evicted,
   re-read, and re-delivered in a cycle, WF lets the acker ack only the
   re-delivered task forever and starve another loaded task. The intended
   assumption (see plan.md) is that every loaded task is eventually acked,
   which is per-level strong fairness. *)
AckerFairness == \A l \in Levels : SF_vars(AckOf(l))

FairSpec == Spec /\ AckerFairness

---------------------------------------------------------------------------
(* Invariants *)

TypeInv ==
  /\ loaded \subseteq Levels
  /\ ackedInMem \subseteq Levels
  /\ loaded \cap ackedInMem = {}
  /\ ackLevel \in NoLevel..MaxLevel
  /\ readLevel \in NoLevel..MaxLevel
  /\ newlyWritten \subseteq Levels
  /\ wrBatch \subseteq Levels
  /\ dbTasks \subseteq everInDb
  /\ everInDb \subseteq usedLevels

\* in-memory entries are exactly within (ackLevel, readLevel]
MemWindow == \A l \in loaded \cup ackedInMem : ackLevel < l /\ l <= readLevel

AckBelowRead == ackLevel <= readLevel

LoadedInDb == loaded \subseteq dbTasks /\ newlyWritten \subseteq dbTasks

LoadedBounded == Cardinality(loaded) <= BatchTarget

\* Safety: the ack level never passes a task that was not acked.
NoAckSkipped == \A l \in everInDb : (l <= ackLevel) => (l \in ackedGhost)

\* While a write is in flight, the pin keeps the ack level below all levels
\* being written (else the merge would drop them / GC could delete them).
\* Ditto for written tasks held in newlyWrittenTasks during a read.
PinProtectsWrites ==
  /\ pinned => \A l \in wrBatch : l > ackLevel
  /\ \A l \in newlyWritten : l > ackLevel

\* Safety: a task is never deleted from the database unless it was acked.
\* ("Even worse than getting stuck: a restart fixes stuck, not deleted.")
GCOnlyAcked == gcVictims \subseteq ackedGhost

\* The defensive "fair reader stuck" condition (see mergeTasks in
\* fair_task_reader.go) is unreachable in the fixed code.
NoStuck == ~stuckFlag

---------------------------------------------------------------------------
(* Temporal properties *)

AckLevelMonotonic == [][ackLevel' >= ackLevel]_vars

\* Liveness: every task ever written to the database is eventually acked.
AllTasksAcked == <>(\A l \in everInDb : l \in ackedGhost)

\* Liveness: the reader eventually learns it drained the whole queue
\* (isDrained) and stays that way. Implies the reader never gets stuck.
EventuallyDrained == <>[](atEnd /\ loaded = {})

===========================================================================

---------------------------- MODULE FairQueue ----------------------------
(***************************************************************************)
(* PlusCal model of matching's fair task queue reader/writer.              *)
(* Source: service/matching/fair_task_{reader,writer}.go                   *)
(* Milestones and scope decisions: see plan.md. Currently at: M1.          *)
(*                                                                         *)
(* Key abstractions:                                                       *)
(*                                                                         *)
(* - Fair levels <pass,id> are modeled as plain integers 1..MaxLevel.      *)
(*   The reader logic is order-invariant: it only ever compares levels,    *)
(*   so any real execution maps to an integer execution preserving order.  *)
(*                                                                         *)
(* - Go guards reader state with tr.lock; every locked critical section    *)
(*   is one atomic PlusCal step (one label). Lock release points (network  *)
(*   calls) are label boundaries, which is where processes interleave.     *)
(*                                                                         *)
(* - outstandingTasks (treemap level -> task|nil) is split into two sets:  *)
(*   "loaded" (unacked *internalTask entries) and "ackedInMem" (nil        *)
(*   entries). loadedTasks == Cardinality(loaded).                         *)
(*                                                                         *)
(* - A task is ackable as soon as it is merged into "loaded"; the matcher  *)
(*   handoff is not modeled (see plan.md M7).                              *)
(*                                                                         *)
(* - The DB is a separate process reached via a request/response channel   *)
(*   (rdState/rdFrom/rdMax/rdResult). In M1 every call succeeds.           *)
(*                                                                         *)
(* Mutation flags "Mut...": each TRUE re-introduces a bug to validate that *)
(* the properties catch it; all FALSE models the current code. See run.sh. *)
(***************************************************************************)
EXTENDS Naturals, FiniteSets, Sequences, TLC

CONSTANTS
  MaxLevel,        \* task levels are 1..MaxLevel
  BatchTarget,     \* config.GetTasksBatchSize: max tasks to keep loaded
  ReloadAt,        \* config.GetTasksReloadAt: read more when loaded <= this
  \* mutation flags
  MutAtEndOnMiddleRead,  \* seeded bug: treat every read as reaching the end
  MutAckPastLoaded       \* seeded bug: ack level advance ignores loaded tasks

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

(* --algorithm FairQueue

variables
  \* ---- database ----
  dbTasks \in SUBSET Levels,   \* nondeterministic initial backlog
  \* ---- read RPC channel: reader -> db ----
  rdState  = "idle",           \* idle -> req -> resp -> idle
  rdFrom   = NoLevel,
  rdMax    = 0,
  rdResult = {},
  \* ---- reader state (fields of fairTaskReader, guarded by tr.lock) ----
  loaded      = {},            \* levels of unacked tasks in memory
  ackedInMem  = {},            \* levels of acked (nil) placeholder entries
  readLevel   = NoLevel,
  ackLevel    = NoLevel,
  atEnd       = FALSE,
  readPending = TRUE,          \* Start() calls maybeReadTasksLocked
  \* ---- ghost state (not part of the implementation) ----
  ackedGhost = {};             \* every level ever acked

define
  Outstanding    == loaded \cup ackedInMem
  LoadedCount    == Cardinality(loaded)
  \* shouldReadMoreLocked
  ShouldReadMore == ~atEnd /\ LoadedCount <= ReloadAt
end define;

\* mergeTasksLocked. Computes all field updates in one shot (one critical
\* section). "inc" is the set of incoming task levels (read or written).
macro mergeTasks(inc, mode) begin
  with
    \* filter incoming: skip below-or-at ackLevel; skip above readLevel on
    \* write when not atEnd; skip already-present levels
    filtered = {l \in inc :
                  /\ l > ackLevel
                  /\ ~(mode = MWrite /\ ~atEnd /\ l > readLevel)
                  /\ l \notin (loaded \cup ackedInMem)},
    \* loaded tasks plus incoming, keep the BatchTarget lowest
    merged = loaded \cup filtered,
    kept   = KeepLowest(merged, BatchTarget),
    \* "if we have any tasks at all in memory, set readLevel to the max of
    \* that set"; if merged is empty leave readLevel unchanged (f534e74e)
    newRL = IF kept /= {} THEN SetMax(kept) ELSE readLevel,
    \* evict acked (nil) entries above the new read level
    keptAcks = {l \in ackedInMem : l <= newRL},
    \* evictedAnyTasks: set for every merged entry beyond BatchTarget
    \* (loaded evictions and ignored new tasks) and for every evicted ack
    evictedAny = \/ (merged \ kept) /= {}
                 \/ keptAcks /= ackedInMem,
    \* advanceAckLevelLocked: pop acked entries below the lowest loaded task
    clear = IF MutAckPastLoaded THEN keptAcks
            ELSE {c \in keptAcks : \A m \in kept : c < m}
  do
    loaded     := kept;
    ackedInMem := keptAcks \ clear;
    readLevel  := newRL;
    if clear /= {} then
      ackLevel := SetMax(clear);
    end if;
    \* update atEnd: middle read or any eviction -> not at end;
    \* read-to-end -> at end; write -> unchanged
    atEnd := IF MutAtEndOnMiddleRead /\ mode \in {MMiddle, MToEnd} THEN TRUE
             ELSE IF (mode = MMiddle) \/ evictedAny THEN FALSE
             ELSE IF mode = MToEnd THEN TRUE
             ELSE atEnd;
  end with;
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
      mergeTasks(rdResult,
                 IF Cardinality(rdResult) < rdMax THEN MToEnd ELSE MMiddle);
      goto RCheck;
    else
      \* exit path of readTasksImpl, still holding the lock: clear
      \* readPending. (The final maybeReadTasksLocked re-check is a no-op
      \* here in M1: ShouldReadMore is false in this very step.)
      readPending := FALSE;
    end if;
  end while;
end process;

\* The database: handles one read request at a time. M1: always succeeds.
fair process db = "db"
begin
DbLoop:
  while TRUE do
    await rdState = "req";
    rdResult := KeepLowest({l \in dbTasks : l >= rdFrom}, rdMax);
    rdState  := "resp";
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
      clear =   IF MutAckPastLoaded THEN ackd
                ELSE {c \in ackd : \A m \in ld : c < m}
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
VARIABLES pc, dbTasks, rdState, rdFrom, rdMax, rdResult, loaded, ackedInMem, 
          readLevel, ackLevel, atEnd, readPending, ackedGhost

(* define statement *)
Outstanding    == loaded \cup ackedInMem
LoadedCount    == Cardinality(loaded)

ShouldReadMore == ~atEnd /\ LoadedCount <= ReloadAt


vars == << pc, dbTasks, rdState, rdFrom, rdMax, rdResult, loaded, ackedInMem, 
           readLevel, ackLevel, atEnd, readPending, ackedGhost >>

ProcSet == {"reader"} \cup {"db"} \cup {"acker"}

Init == (* Global variables *)
        /\ dbTasks \in SUBSET Levels
        /\ rdState = "idle"
        /\ rdFrom = NoLevel
        /\ rdMax = 0
        /\ rdResult = {}
        /\ loaded = {}
        /\ ackedInMem = {}
        /\ readLevel = NoLevel
        /\ ackLevel = NoLevel
        /\ atEnd = FALSE
        /\ readPending = TRUE
        /\ ackedGhost = {}
        /\ pc = [self \in ProcSet |-> CASE self = "reader" -> "RWait"
                                        [] self = "db" -> "DbLoop"
                                        [] self = "acker" -> "AckLoop"]

RWait == /\ pc["reader"] = "RWait"
         /\ readPending
         /\ pc' = [pc EXCEPT !["reader"] = "RCheck"]
         /\ UNCHANGED << dbTasks, rdState, rdFrom, rdMax, rdResult, loaded, 
                         ackedInMem, readLevel, ackLevel, atEnd, readPending, 
                         ackedGhost >>

RCheck == /\ pc["reader"] = "RCheck"
          /\ IF ShouldReadMore
                THEN /\ rdFrom' = readLevel + 1
                     /\ rdMax' = BatchTarget - LoadedCount
                     /\ rdState' = "req"
                     /\ pc' = [pc EXCEPT !["reader"] = "RResp"]
                     /\ UNCHANGED readPending
                ELSE /\ readPending' = FALSE
                     /\ pc' = [pc EXCEPT !["reader"] = "RWait"]
                     /\ UNCHANGED << rdState, rdFrom, rdMax >>
          /\ UNCHANGED << dbTasks, rdResult, loaded, ackedInMem, readLevel, 
                          ackLevel, atEnd, ackedGhost >>

RResp == /\ pc["reader"] = "RResp"
         /\ rdState = "resp"
         /\ rdState' = "idle"
         /\ LET filtered == {l \in rdResult :
                               /\ l > ackLevel
                               /\ ~((IF Cardinality(rdResult) < rdMax THEN MToEnd ELSE MMiddle) = MWrite /\ ~atEnd /\ l > readLevel)
                               /\ l \notin (loaded \cup ackedInMem)} IN
              LET merged == loaded \cup filtered IN
                LET kept == KeepLowest(merged, BatchTarget) IN
                  LET newRL == IF kept /= {} THEN SetMax(kept) ELSE readLevel IN
                    LET keptAcks == {l \in ackedInMem : l <= newRL} IN
                      LET evictedAny == \/ (merged \ kept) /= {}
                                        \/ keptAcks /= ackedInMem IN
                        LET clear == IF MutAckPastLoaded THEN keptAcks
                                     ELSE {c \in keptAcks : \A m \in kept : c < m} IN
                          /\ loaded' = kept
                          /\ ackedInMem' = keptAcks \ clear
                          /\ readLevel' = newRL
                          /\ IF clear /= {}
                                THEN /\ ackLevel' = SetMax(clear)
                                ELSE /\ TRUE
                                     /\ UNCHANGED ackLevel
                          /\ atEnd' = (IF MutAtEndOnMiddleRead /\ (IF Cardinality(rdResult) < rdMax THEN MToEnd ELSE MMiddle) \in {MMiddle, MToEnd} THEN TRUE
                                       ELSE IF ((IF Cardinality(rdResult) < rdMax THEN MToEnd ELSE MMiddle) = MMiddle) \/ evictedAny THEN FALSE
                                       ELSE IF (IF Cardinality(rdResult) < rdMax THEN MToEnd ELSE MMiddle) = MToEnd THEN TRUE
                                       ELSE atEnd)
         /\ pc' = [pc EXCEPT !["reader"] = "RCheck"]
         /\ UNCHANGED << dbTasks, rdFrom, rdMax, rdResult, readPending, 
                         ackedGhost >>

reader == RWait \/ RCheck \/ RResp

DbLoop == /\ pc["db"] = "DbLoop"
          /\ rdState = "req"
          /\ rdResult' = KeepLowest({l \in dbTasks : l >= rdFrom}, rdMax)
          /\ rdState' = "resp"
          /\ pc' = [pc EXCEPT !["db"] = "DbLoop"]
          /\ UNCHANGED << dbTasks, rdFrom, rdMax, loaded, ackedInMem, 
                          readLevel, ackLevel, atEnd, readPending, ackedGhost >>

db == DbLoop

AckLoop == /\ pc["acker"] = "AckLoop"
           /\ loaded /= {}
           /\ \E l \in loaded:
                LET ld == loaded \ {l} IN
                  LET ackd == ackedInMem \cup {l} IN
                    LET clear == IF MutAckPastLoaded THEN ackd
                                 ELSE {c \in ackd : \A m \in ld : c < m} IN
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
           /\ UNCHANGED << dbTasks, rdState, rdFrom, rdMax, rdResult, 
                           readLevel, atEnd >>

acker == AckLoop

Next == reader \/ db \/ acker

Spec == /\ Init /\ [][Next]_vars
        /\ WF_vars(reader)
        /\ WF_vars(db)
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

\* in-memory entries are exactly within (ackLevel, readLevel]
MemWindow == \A l \in loaded \cup ackedInMem : ackLevel < l /\ l <= readLevel

AckBelowRead == ackLevel <= readLevel

LoadedInDb == loaded \subseteq dbTasks

LoadedBounded == Cardinality(loaded) <= BatchTarget

\* Safety: the ack level never passes a task that was not acked.
NoAckSkipped == \A l \in dbTasks : (l <= ackLevel) => (l \in ackedGhost)

---------------------------------------------------------------------------
(* Temporal properties *)

AckLevelMonotonic == [][ackLevel' >= ackLevel]_vars

\* Liveness: every task in the database is eventually acked.
AllTasksAcked == <>(\A l \in dbTasks : l \in ackedGhost)

\* Liveness: the reader eventually learns it drained the whole queue
\* (isDrained) and stays that way. Implies the reader never gets stuck.
EventuallyDrained == <>[](atEnd /\ loaded = {})

===========================================================================

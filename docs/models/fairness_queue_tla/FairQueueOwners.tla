------------------------- MODULE FairQueueOwners -------------------------
(***************************************************************************)
(* FairQueue.tla extended with partition ownership changes.                *)
(* Source: service/matching/fair_{task_reader,task_writer,backlog_manager} *)
(* .go and db.go.                                                          *)
(*                                                                         *)
(* A copy of FairQueue.tla (single owner) with the per-owner state         *)
(* (reader/writer/acker/gc and their RPC channels) indexed by owner, plus  *)
(* the persisted task queue metadata and the takeover protocol:            *)
(*                                                                         *)
(* - Metadata: range id (metaRange) and the persisted fair ack level       *)
(*   (metaAck). Other metadata fields are not modeled.                     *)
(*                                                                         *)
(* - Owners 1..NumOwners take over in order. Owner 1 owns range 1 at the   *)
(*   start. Owner o > 1 may take over (unfairly: ownership changes are     *)
(*   optional) once owner o-1 has, via takeOverTaskQueueLocked: a plain    *)
(*   read of the metadata, then an LWT bumping the range id, conditional   *)
(*   only on the range id, that writes back the metadata it read. The new  *)
(*   owner starts with readLevel = ackLevel = the ack level it read.       *)
(*   (RenewLease timeouts are not modeled: a timed-out takeover LWT that   *)
(*   applied is retried and is equivalent to a successful takeover.)       *)
(*                                                                         *)
(* - Fencing: task writes (CreateTasks) and metadata updates (SyncState:   *)
(*   periodic ack level persistence and ownership check) are LWTs          *)
(*   conditional on the owner's range id; the resulting ConditionFailed    *)
(*   makes the owner unload (signalIfFatal), modeled as the owner dying:   *)
(*   all its processes stop. Reads (GetTasks) and GC deletes               *)
(*   (CompleteTasksLessThan) are NOT fenced in current code.               *)
(*                                                                         *)
(* - A stale owner (still alive, not yet aware it lost ownership) keeps    *)
(*   reading, acking and GCing. Its acks are real dispatches (the worker   *)
(*   can still start the task), so ackedGhost is global.                   *)
(*                                                                         *)
(* - Fair levels are integers, as in FairQueue.tla. With multiple owners   *)
(*   this over-approximates: a later owner's task can be placed anywhere   *)
(*   above its own pinned ack level, whereas in reality its (higher) ids   *)
(*   sort it after an earlier owner's task with the same pass. The bug     *)
(*   this model targets only needs a pass below the stale owner's          *)
(*   in-memory ack pass, which is real. Counterexamples should be checked  *)
(*   against this.                                                         *)
(*                                                                         *)
(* - GC and sync are single atomic steps (capture level + DB effect).      *)
(*   Splitting them would only add behaviors that act on an older, less    *)
(*   aggressive level. Sync timeouts are not modeled (an applied-but-      *)
(*   unacknowledged sync only makes GcMode="persisted" less aggressive).   *)
(*                                                                         *)
(* - Simplifications vs FairQueue.tla (covered there): no mutation flags,  *)
(*   all levels ackable. Read timeouts, write timeouts and expiry are      *)
(*   switchable to keep the two-owner state space tractable.               *)
(*                                                                         *)
(* GcMode selects the GC behavior (all but "unfenced" are candidate fixes):*)
(*   "unfenced":  current code: delete <= in-memory ackLevel, unfenced.    *)
(*   "verified":  verify ownership (read range id) first, then delete.     *)
(*   "persisted": delete only <= the ack level this owner persisted with   *)
(*                a successful (range-fenced) metadata LWT.                *)
(*   "fenced":    the delete itself is conditional on the range id.        *)
(* TakeoverCAS: the takeover LWT also requires the persisted ack level to  *)
(* be unchanged since the takeover read (else re-read and retry).          *)
(***************************************************************************)
EXTENDS Naturals, FiniteSets, Sequences, TLC

CONSTANTS
  MaxLevel,        \* task levels are 1..MaxLevel
  BatchTarget,     \* config.GetTasksBatchSize: max tasks to keep loaded
  ReloadAt,        \* config.GetTasksReloadAt: read more when loaded <= this
  WBatchMax,       \* max tasks per write batch
  EvictedCacheMax, \* evictedAcksCacheSize: max evicted-ack cache entries
  NumOwners,       \* owners 1..NumOwners take over in order (at most 9)
  ReadTimeouts,    \* reads may time out (backoff timer + retry)
  WriteTimeouts,   \* writes may time out (applied or not)
  Expiry,          \* tasks may be expired by the time they're merged
  GcMode,          \* "unfenced" (current code) | "verified" | "persisted" | "fenced"
  TakeoverCAS,     \* takeover LWT also conditional on the ack level read
  WriterMayStop    \* the writer may stop writing for good (only matters
                   \* for liveness; FALSE shrinks safety-only runs)

ASSUME ReloadAt < BatchTarget
ASSUME MaxLevel >= 1
ASSUME NumOwners \in 1..9
ASSUME GcMode \in {"unfenced", "verified", "persisted", "fenced"}

Levels  == 1..MaxLevel
NoLevel == 0
Owners  == 1..NumOwners

\* process ids: (process kind offset) + owner
RdId(o) == o
TmId(o) == 10 + o
WrId(o) == 20 + o
AkId(o) == 30 + o
GcId(o) == 40 + o
SyId(o) == 50 + o
DrId(o) == 60 + o
DwId(o) == 70 + o
TkId(o) == 80 + o
Own(p)  == p % 10

\* merge modes (mergeMode in fair_task_reader.go)
MMiddle == "readMiddle"
MToEnd  == "readToEnd"
MWrite  == "write"

SetMax(S) == CHOOSE x \in S : \A y \in S : y <= x
\* The n lowest elements of S (all of S if it has <= n elements).
KeepLowest(S, n) == {l \in S : Cardinality({m \in S : m <= l}) <= n}

\* candidate sets of incoming tasks that are expired at merge time
ExpSubsets(inc) == IF Expiry THEN SUBSET inc ELSE {{}}

(***************************************************************************)
(* mergeTasksLocked as a pure function; identical to FairQueue.tla with    *)
(* the mutation flags removed.                                             *)
(***************************************************************************)
MergeResult(loaded0, acks0, rl0, al0, atEnd0, inc, mode, pinnedNow, expSel,
            cache0) ==
  LET
    filtered == {l \in inc :
                   /\ l > al0
                   /\ ~(mode = MWrite /\ ~atEnd0 /\ l > rl0)
                   /\ l \notin (loaded0 \cup acks0)}
    merged == loaded0 \cup filtered
    kept   == KeepLowest(merged, BatchTarget)
    newRL == IF kept /= {} THEN SetMax(kept) ELSE rl0
    keptAcks == {l \in acks0 : l <= newRL}
    evictedAny == \/ (merged \ kept) /= {}
                  \/ keptAcks /= acks0
    cacheTrimmed == KeepLowest(cache0 \cup (acks0 \ keptAcks), EvictedCacheMax)
    cacheHits == (kept \cap filtered) \cap cacheTrimmed
    keptNewExpired == (kept \cap filtered) \cap expSel
    newLoaded == kept \ (keptNewExpired \cup cacheHits)
    newAcks   == keptAcks \cup keptNewExpired \cup cacheHits
    clear == IF pinnedNow THEN {}
             ELSE {c \in newAcks : \A m \in newLoaded : c < m}
    newAtEnd == IF (mode = MMiddle) \/ evictedAny THEN FALSE
                ELSE IF mode = MToEnd THEN TRUE
                ELSE atEnd0
  IN [
    loaded   |-> newLoaded,
    acks     |-> newAcks \ clear,
    rl       |-> newRL,
    al       |-> IF clear = {} THEN al0 ELSE SetMax(clear),
    atEnd    |-> newAtEnd,
    stuck    |-> mode = MWrite /\ ~newAtEnd /\ newLoaded = {},
    cache    |-> cacheTrimmed \ cacheHits,
    consumed |-> keptNewExpired
  ]

(* --algorithm FairQueueOwners

variables
  \* ---- database: tasks ----
  dbTasks \in SUBSET Levels,   \* nondeterministic initial backlog
  \* ---- database: task queue metadata ----
  metaRange = 1,               \* range id; owner 1 holds range 1 at the start
  metaAck   = NoLevel,         \* persisted fair ack level
  \* ---- ownership ----
  ostate   = [o \in Owners |-> IF o = 1 THEN "active" ELSE "idle"],
                               \* idle -> taking -> active -> dead
  ownRange = [o \in Owners |-> IF o = 1 THEN 1 ELSE 0],
  tkRange  = [o \in Owners |-> 0],       \* takeover: metadata as read
  tkAck    = [o \in Owners |-> NoLevel],
  confirmedAck = [o \in Owners |-> NoLevel], \* ack level persisted by a
                               \* successful LWT (only tracked for "persisted")
  gcVerified   = [o \in Owners |-> FALSE],   \* GcMode "verified": ownership
                               \* check done, delete pending
  \* ---- read RPC channels: reader -> db ----
  rdState  = [o \in Owners |-> "idle"],
  rdFrom   = [o \in Owners |-> NoLevel],
  rdMax    = [o \in Owners |-> 0],
  rdResult = [o \in Owners |-> {}],
  rdOk     = [o \in Owners |-> TRUE],
  \* ---- write RPC channels: writer -> db ----
  wrState  = [o \in Owners |-> "idle"],
  wrBatch  = [o \in Owners |-> {}],
  wrRes    = [o \in Owners |-> "ok"],    \* ok | timeout | condfail
  \* ---- reader state per owner ----
  loaded       = [o \in Owners |-> {}],
  ackedInMem   = [o \in Owners |-> {}],
  readLevel    = [o \in Owners |-> NoLevel],
  ackLevel     = [o \in Owners |-> NoLevel],
  atEnd        = [o \in Owners |-> FALSE],
  readPending  = [o \in Owners |-> o = 1],
  newlyWritten = [o \in Owners |-> {}],
  pinned       = [o \in Owners |-> FALSE],
  evictedAcks  = [o \in Owners |-> {}],
  backoffTimer = [o \in Owners |-> FALSE],
  \* ---- writer state (ids are unique across owners) ----
  usedLevels = dbTasks,
  \* ---- ghost state ----
  everInDb   = dbTasks,
  committed  = dbTasks,        \* initial backlog + writes whose RPC succeeded
  ackedGhost = {},             \* every level ever acked, by any owner
  gcVictims  = {},             \* every level ever deleted by GC
  stuckFlag  = FALSE;          \* defensive "fair reader stuck" check fired

define
  Alive(o)   == ostate[o] = "active"
  \* the owner whose range id is the persisted one (at most one is alive)
  Current(o) == Alive(o) /\ ownRange[o] = metaRange
  LoadedCount(o)    == Cardinality(loaded[o])
  ShouldReadMore(o) == ~atEnd[o] /\ LoadedCount(o) <= ReloadAt
  AvailableLevels(o) == {l \in Levels : l \notin usedLevels /\ l > ackLevel[o]}
  WriteBatches(o)    == {B \in SUBSET AvailableLevels(o) :
                           B /= {} /\ Cardinality(B) <= WBatchMax}
  GcLevel(o) == IF GcMode = "persisted" THEN confirmedAck[o] ELSE ackLevel[o]
end define;

macro applyMerge(o, r) begin
  loaded[o]      := r.loaded;
  ackedInMem[o]  := r.acks;
  readLevel[o]   := r.rl;
  ackLevel[o]    := r.al;
  atEnd[o]       := r.atEnd;
  evictedAcks[o] := r.cache;
end macro;

\* The exit path of readTasksImpl; see FairQueue.tla.
macro readerExit(o) begin
  if newlyWritten[o] /= {} then
    with expSel \in ExpSubsets(newlyWritten[o]),
         r = MergeResult(loaded[o], ackedInMem[o], readLevel[o], ackLevel[o],
                         atEnd[o], newlyWritten[o], MWrite, pinned[o], expSel,
                         evictedAcks[o])
    do
      applyMerge(o, r);
      committed       := committed \ r.consumed;
      newlyWritten[o] := {};
      readPending[o]  := ~r.atEnd /\ Cardinality(r.loaded) <= ReloadAt
                         /\ ~backoffTimer[o];
    end with;
  else
    readPending[o] := ShouldReadMore(o) /\ ~backoffTimer[o];
  end if;
end macro;

\* takeOverTaskQueueLocked (RenewLease with no lease held) + initState +
\* newFairTaskReader/Start.
process takeover \in {TkId(o) : o \in 2..NumOwners}
begin
TkRead:
  with o = Own(self) do
    await ostate[o-1] \in {"active", "dead"};
    \* GetTaskQueue: a plain (unfenced) read
    tkRange[o] := metaRange;
    tkAck[o]   := metaAck;
    ostate[o]  := "taking";
  end with;
TkLwt:
  with o = Own(self) do
    \* UpdateTaskQueue(RangeID: r+1, PrevRangeID: r), writing back the
    \* metadata as read -- including the ack level, which the old owner may
    \* have advanced (and persisted) since the read
    if metaRange = tkRange[o] /\ (~TakeoverCAS \/ metaAck = tkAck[o]) then
      metaRange       := tkRange[o] + 1;
      metaAck         := tkAck[o];
      ownRange[o]     := tkRange[o] + 1;
      confirmedAck[o] := tkAck[o];
      readLevel[o]    := tkAck[o];
      ackLevel[o]     := tkAck[o];
      readPending[o]  := TRUE;
      ostate[o]       := "active";
      tkRange[o]      := 0;
      tkAck[o]        := NoLevel;
    else
      goto TkRead;
    end if;
  end with;
end process;

\* readTasksImpl, per owner. Reader ids are the owner ids.
fair process reader \in {RdId(o) : o \in Owners}
begin
RWait:
  while TRUE do
    await readPending[self] /\ Alive(self);
RCheck:
    await Alive(self);
    if ShouldReadMore(self) then
      rdFrom[self]  := readLevel[self] + 1;
      rdMax[self]   := BatchTarget - LoadedCount(self);
      rdState[self] := "req";
RResp:
      await rdState[self] = "resp" /\ Alive(self);
      rdState[self] := "idle";
      if rdOk[self] then
        with expSel \in ExpSubsets(rdResult[self]),
             mode = IF Cardinality(rdResult[self]) < rdMax[self]
                    THEN MToEnd ELSE MMiddle,
             r    = MergeResult(loaded[self], ackedInMem[self], readLevel[self],
                                ackLevel[self], atEnd[self], rdResult[self],
                                mode, pinned[self] \/ newlyWritten[self] /= {},
                                expSel, evictedAcks[self])
        do
          applyMerge(self, r);
          committed := committed \ r.consumed;
        end with;
        \* (request/response fields are cleared once consumed, to keep
        \* otherwise-identical states from being distinct)
        rdResult[self] := {};
        rdFrom[self]   := NoLevel;
        rdMax[self]    := 0;
        goto RCheck;
      else
        rdFrom[self] := NoLevel;
        rdMax[self]  := 0;
        if ~backoffTimer[self] then
          backoffTimer[self] := TRUE;
        end if;
      end if;
RExit:
      await Alive(self);
      readerExit(self);
    else
      readerExit(self);
    end if;
  end while;
end process;

fair process timer \in {TmId(o) : o \in Owners}
begin
TimerLoop:
  while TRUE do
    with o = Own(self) do
      await backoffTimer[o] /\ Alive(o);
      backoffTimer[o] := FALSE;
      if ~readPending[o] /\ ShouldReadMore(o) then
        readPending[o] := TRUE;
      end if;
    end with;
  end while;
end process;

\* taskWriterLoop/writeBatch, per owner.
fair process writer \in {WrId(o) : o \in Owners}
begin
WLoop:
  while TRUE do
    with o = Own(self) do
      await Alive(o);
      either
        \* getAndPinAckLevels + pickPasses: levels above this owner's
        \* pinned ack level; ids are never reused
        with B \in WriteBatches(o) do
          pinned[o]  := TRUE;
          wrBatch[o] := B;
          usedLevels := usedLevels \cup B;
          wrState[o] := "req";
        end with;
      or
        await WriterMayStop;
        goto WDone;
      end either;
    end with;
WResp:
    with o = Own(self) do
      await wrState[o] = "resp" /\ Alive(o);
      wrState[o] := "idle";
      if wrRes[o] = "ok" then
        if readPending[o] then
          newlyWritten[o] := newlyWritten[o] \cup wrBatch[o];
        else
          with expSel \in ExpSubsets(wrBatch[o]),
               r = MergeResult(loaded[o], ackedInMem[o], readLevel[o],
                               ackLevel[o], atEnd[o], wrBatch[o], MWrite, TRUE,
                               expSel, evictedAcks[o])
          do
            applyMerge(o, r);
            committed := committed \ r.consumed;
            stuckFlag := stuckFlag \/ (r.stuck /\ ~readPending[o] /\ ~backoffTimer[o]);
            if r.stuck /\ ~readPending[o] /\ ~backoffTimer[o] then
              readPending[o] := TRUE;
            end if;
          end with;
        end if;
      elsif wrRes[o] = "condfail" then
        \* signalIfFatal: ownership lost -> unload
        ostate[o] := "dead";
      end if;
    end with;
WUnpin:
    with o = Own(self) do
      await Alive(o);
      if wrRes[o] /= "ok" then
        atEnd[o] := FALSE;
        if ~readPending[o] /\ ~backoffTimer[o] /\ LoadedCount(o) <= ReloadAt then
          readPending[o] := TRUE;
        end if;
      end if;
      pinned[o]  := FALSE;
      wrBatch[o] := {};
      wrRes[o]   := "ok";
      with clear = IF newlyWritten[o] /= {} THEN {}
                   ELSE {c \in ackedInMem[o] : \A m \in loaded[o] : c < m}
      do
        ackedInMem[o] := ackedInMem[o] \ clear;
        if clear /= {} then
          ackLevel[o] := SetMax(clear);
        end if;
      end with;
    end with;
  end while;
WDone:
  skip;
end process;

fair process dbRead \in {DrId(o) : o \in Owners}
begin
DbReadLoop:
  while TRUE do
    with o = Own(self) do
      await rdState[o] = "req" /\ Alive(o);
      either
        \* reads are not fenced by range id
        rdResult[o] := KeepLowest({l \in dbTasks : l >= rdFrom[o]}, rdMax[o]);
        rdOk[o]     := TRUE;
      or
        await ReadTimeouts;
        rdOk[o] := FALSE;
      end either;
      rdState[o] := "resp";
    end with;
  end while;
end process;

fair process dbWrite \in {DwId(o) : o \in Owners}
begin
DbWriteLoop:
  while TRUE do
    with o = Own(self) do
      await wrState[o] = "req" /\ Alive(o);
      if ownRange[o] /= metaRange then
        \* CreateTasks is an LWT on the range id: rejected, not applied
        wrRes[o] := "condfail";
      else
        either
          dbTasks   := dbTasks \cup wrBatch[o];
          everInDb  := everInDb \cup wrBatch[o];
          committed := committed \cup wrBatch[o];
          wrRes[o]  := "ok";
        or
          \* incoming timeout: applied, but the writer sees an error
          await WriteTimeouts;
          dbTasks  := dbTasks \cup wrBatch[o];
          everInDb := everInDb \cup wrBatch[o];
          wrRes[o] := "timeout";
        or
          \* outgoing timeout: not applied
          await WriteTimeouts;
          wrRes[o] := "timeout";
        end either;
      end if;
      wrState[o] := "resp";
    end with;
  end while;
end process;

\* periodicSync/SyncState (and Stop's final update): persist the ack level
\* with an LWT on the range id, or just verify the range id. Either way a
\* ConditionFailed unloads the owner. Weakly fair: a stale owner eventually
\* finds out.
fair process sync \in {SyId(o) : o \in Owners}
begin
SyncLoop:
  while TRUE do
    with o = Own(self) do
      await Alive(o);
      if ownRange[o] /= metaRange then
        ostate[o] := "dead";
      else
        metaAck := ackLevel[o];
        if GcMode = "persisted" then
          confirmedAck[o] := ackLevel[o];
        end if;
      end if;
    end with;
  end while;
end process;

\* maybeGCLocked/doGC, per owner: delete tasks <= GcLevel. Not fair.
process gc \in {GcId(o) : o \in Owners}
begin
GcLoop:
  while TRUE do
    with o = Own(self) do
      await Alive(o);
      either
        \* GcMode "verified": check ownership first (separate RPC)
        await GcMode = "verified" /\ ~gcVerified[o];
        if ownRange[o] /= metaRange then
          ostate[o] := "dead";
        else
          gcVerified[o] := TRUE;
        end if;
      or
        await GcLevel(o) > NoLevel /\ (GcMode = "verified" => gcVerified[o]);
        if GcMode = "verified" then
          gcVerified[o] := FALSE;
        end if;
        \* CompleteFairTasksLessThan(GcLevel.inc()); may also not apply
        \* (timeout), which is the same as not running
        if GcMode /= "fenced" \/ ownRange[o] = metaRange then
          with victims = {l \in dbTasks : l <= GcLevel(o)} do
            dbTasks   := dbTasks \ victims;
            gcVictims := gcVictims \cup victims;
          end with;
        end if;
      end either;
    end with;
  end while;
end process;

\* The acker, per owner: completeTaskLocked.
fair process acker \in {AkId(o) : o \in Owners}
begin
AckLoop:
  while TRUE do
    with o = Own(self) do
      await Alive(o) /\ loaded[o] /= {};
      with
        l     \in loaded[o],
        ld    =   loaded[o] \ {l},
        ackd  =   ackedInMem[o] \cup {l},
        clear =   IF pinned[o] \/ newlyWritten[o] /= {} THEN {}
                  ELSE {c \in ackd : \A m \in ld : c < m}
      do
        loaded[o]     := ld;
        ackedInMem[o] := ackd \ clear;
        if clear /= {} then
          ackLevel[o] := SetMax(clear);
        end if;
        ackedGhost := ackedGhost \cup {l};
        if ~readPending[o] /\ ~atEnd[o] /\ Cardinality(ld) <= ReloadAt
           /\ ~backoffTimer[o] then
          readPending[o] := TRUE;
        end if;
      end with;
    end with;
  end while;
end process;

end algorithm; *)

\* BEGIN TRANSLATION
VARIABLES pc, dbTasks, metaRange, metaAck, ostate, ownRange, tkRange, tkAck, 
          confirmedAck, gcVerified, rdState, rdFrom, rdMax, rdResult, rdOk, 
          wrState, wrBatch, wrRes, loaded, ackedInMem, readLevel, ackLevel, 
          atEnd, readPending, newlyWritten, pinned, evictedAcks, backoffTimer, 
          usedLevels, everInDb, committed, ackedGhost, gcVictims, stuckFlag

(* define statement *)
Alive(o)   == ostate[o] = "active"

Current(o) == Alive(o) /\ ownRange[o] = metaRange
LoadedCount(o)    == Cardinality(loaded[o])
ShouldReadMore(o) == ~atEnd[o] /\ LoadedCount(o) <= ReloadAt
AvailableLevels(o) == {l \in Levels : l \notin usedLevels /\ l > ackLevel[o]}
WriteBatches(o)    == {B \in SUBSET AvailableLevels(o) :
                         B /= {} /\ Cardinality(B) <= WBatchMax}
GcLevel(o) == IF GcMode = "persisted" THEN confirmedAck[o] ELSE ackLevel[o]


vars == << pc, dbTasks, metaRange, metaAck, ostate, ownRange, tkRange, tkAck, 
           confirmedAck, gcVerified, rdState, rdFrom, rdMax, rdResult, rdOk, 
           wrState, wrBatch, wrRes, loaded, ackedInMem, readLevel, ackLevel, 
           atEnd, readPending, newlyWritten, pinned, evictedAcks, 
           backoffTimer, usedLevels, everInDb, committed, ackedGhost, 
           gcVictims, stuckFlag >>

ProcSet == ({TkId(o) : o \in 2..NumOwners}) \cup ({RdId(o) : o \in Owners}) \cup ({TmId(o) : o \in Owners}) \cup ({WrId(o) : o \in Owners}) \cup ({DrId(o) : o \in Owners}) \cup ({DwId(o) : o \in Owners}) \cup ({SyId(o) : o \in Owners}) \cup ({GcId(o) : o \in Owners}) \cup ({AkId(o) : o \in Owners})

Init == (* Global variables *)
        /\ dbTasks \in SUBSET Levels
        /\ metaRange = 1
        /\ metaAck = NoLevel
        /\ ostate = [o \in Owners |-> IF o = 1 THEN "active" ELSE "idle"]
        /\ ownRange = [o \in Owners |-> IF o = 1 THEN 1 ELSE 0]
        /\ tkRange = [o \in Owners |-> 0]
        /\ tkAck = [o \in Owners |-> NoLevel]
        /\ confirmedAck = [o \in Owners |-> NoLevel]
        /\ gcVerified = [o \in Owners |-> FALSE]
        /\ rdState = [o \in Owners |-> "idle"]
        /\ rdFrom = [o \in Owners |-> NoLevel]
        /\ rdMax = [o \in Owners |-> 0]
        /\ rdResult = [o \in Owners |-> {}]
        /\ rdOk = [o \in Owners |-> TRUE]
        /\ wrState = [o \in Owners |-> "idle"]
        /\ wrBatch = [o \in Owners |-> {}]
        /\ wrRes = [o \in Owners |-> "ok"]
        /\ loaded = [o \in Owners |-> {}]
        /\ ackedInMem = [o \in Owners |-> {}]
        /\ readLevel = [o \in Owners |-> NoLevel]
        /\ ackLevel = [o \in Owners |-> NoLevel]
        /\ atEnd = [o \in Owners |-> FALSE]
        /\ readPending = [o \in Owners |-> o = 1]
        /\ newlyWritten = [o \in Owners |-> {}]
        /\ pinned = [o \in Owners |-> FALSE]
        /\ evictedAcks = [o \in Owners |-> {}]
        /\ backoffTimer = [o \in Owners |-> FALSE]
        /\ usedLevels = dbTasks
        /\ everInDb = dbTasks
        /\ committed = dbTasks
        /\ ackedGhost = {}
        /\ gcVictims = {}
        /\ stuckFlag = FALSE
        /\ pc = [self \in ProcSet |-> CASE self \in {TkId(o) : o \in 2..NumOwners} -> "TkRead"
                                        [] self \in {RdId(o) : o \in Owners} -> "RWait"
                                        [] self \in {TmId(o) : o \in Owners} -> "TimerLoop"
                                        [] self \in {WrId(o) : o \in Owners} -> "WLoop"
                                        [] self \in {DrId(o) : o \in Owners} -> "DbReadLoop"
                                        [] self \in {DwId(o) : o \in Owners} -> "DbWriteLoop"
                                        [] self \in {SyId(o) : o \in Owners} -> "SyncLoop"
                                        [] self \in {GcId(o) : o \in Owners} -> "GcLoop"
                                        [] self \in {AkId(o) : o \in Owners} -> "AckLoop"]

TkRead(self) == /\ pc[self] = "TkRead"
                /\ LET o == Own(self) IN
                     /\ ostate[o-1] \in {"active", "dead"}
                     /\ tkRange' = [tkRange EXCEPT ![o] = metaRange]
                     /\ tkAck' = [tkAck EXCEPT ![o] = metaAck]
                     /\ ostate' = [ostate EXCEPT ![o] = "taking"]
                /\ pc' = [pc EXCEPT ![self] = "TkLwt"]
                /\ UNCHANGED << dbTasks, metaRange, metaAck, ownRange, 
                                confirmedAck, gcVerified, rdState, rdFrom, 
                                rdMax, rdResult, rdOk, wrState, wrBatch, wrRes, 
                                loaded, ackedInMem, readLevel, ackLevel, atEnd, 
                                readPending, newlyWritten, pinned, evictedAcks, 
                                backoffTimer, usedLevels, everInDb, committed, 
                                ackedGhost, gcVictims, stuckFlag >>

TkLwt(self) == /\ pc[self] = "TkLwt"
               /\ LET o == Own(self) IN
                    IF metaRange = tkRange[o] /\ (~TakeoverCAS \/ metaAck = tkAck[o])
                       THEN /\ metaRange' = tkRange[o] + 1
                            /\ metaAck' = tkAck[o]
                            /\ ownRange' = [ownRange EXCEPT ![o] = tkRange[o] + 1]
                            /\ confirmedAck' = [confirmedAck EXCEPT ![o] = tkAck[o]]
                            /\ readLevel' = [readLevel EXCEPT ![o] = tkAck[o]]
                            /\ ackLevel' = [ackLevel EXCEPT ![o] = tkAck[o]]
                            /\ readPending' = [readPending EXCEPT ![o] = TRUE]
                            /\ ostate' = [ostate EXCEPT ![o] = "active"]
                            /\ tkRange' = [tkRange EXCEPT ![o] = 0]
                            /\ tkAck' = [tkAck EXCEPT ![o] = NoLevel]
                            /\ pc' = [pc EXCEPT ![self] = "Done"]
                       ELSE /\ pc' = [pc EXCEPT ![self] = "TkRead"]
                            /\ UNCHANGED << metaRange, metaAck, ostate, 
                                            ownRange, tkRange, tkAck, 
                                            confirmedAck, readLevel, ackLevel, 
                                            readPending >>
               /\ UNCHANGED << dbTasks, gcVerified, rdState, rdFrom, rdMax, 
                               rdResult, rdOk, wrState, wrBatch, wrRes, loaded, 
                               ackedInMem, atEnd, newlyWritten, pinned, 
                               evictedAcks, backoffTimer, usedLevels, everInDb, 
                               committed, ackedGhost, gcVictims, stuckFlag >>

takeover(self) == TkRead(self) \/ TkLwt(self)

RWait(self) == /\ pc[self] = "RWait"
               /\ readPending[self] /\ Alive(self)
               /\ pc' = [pc EXCEPT ![self] = "RCheck"]
               /\ UNCHANGED << dbTasks, metaRange, metaAck, ostate, ownRange, 
                               tkRange, tkAck, confirmedAck, gcVerified, 
                               rdState, rdFrom, rdMax, rdResult, rdOk, wrState, 
                               wrBatch, wrRes, loaded, ackedInMem, readLevel, 
                               ackLevel, atEnd, readPending, newlyWritten, 
                               pinned, evictedAcks, backoffTimer, usedLevels, 
                               everInDb, committed, ackedGhost, gcVictims, 
                               stuckFlag >>

RCheck(self) == /\ pc[self] = "RCheck"
                /\ Alive(self)
                /\ IF ShouldReadMore(self)
                      THEN /\ rdFrom' = [rdFrom EXCEPT ![self] = readLevel[self] + 1]
                           /\ rdMax' = [rdMax EXCEPT ![self] = BatchTarget - LoadedCount(self)]
                           /\ rdState' = [rdState EXCEPT ![self] = "req"]
                           /\ pc' = [pc EXCEPT ![self] = "RResp"]
                           /\ UNCHANGED << loaded, ackedInMem, readLevel, 
                                           ackLevel, atEnd, readPending, 
                                           newlyWritten, evictedAcks, 
                                           committed >>
                      ELSE /\ IF newlyWritten[self] /= {}
                                 THEN /\ \E expSel \in ExpSubsets(newlyWritten[self]):
                                           LET r == MergeResult(loaded[self], ackedInMem[self], readLevel[self], ackLevel[self],
                                                                atEnd[self], newlyWritten[self], MWrite, pinned[self], expSel,
                                                                evictedAcks[self]) IN
                                             /\ loaded' = [loaded EXCEPT ![self] = r.loaded]
                                             /\ ackedInMem' = [ackedInMem EXCEPT ![self] = r.acks]
                                             /\ readLevel' = [readLevel EXCEPT ![self] = r.rl]
                                             /\ ackLevel' = [ackLevel EXCEPT ![self] = r.al]
                                             /\ atEnd' = [atEnd EXCEPT ![self] = r.atEnd]
                                             /\ evictedAcks' = [evictedAcks EXCEPT ![self] = r.cache]
                                             /\ committed' = committed \ r.consumed
                                             /\ newlyWritten' = [newlyWritten EXCEPT ![self] = {}]
                                             /\ readPending' = [readPending EXCEPT ![self] = ~r.atEnd /\ Cardinality(r.loaded) <= ReloadAt
                                                                                             /\ ~backoffTimer[self]]
                                 ELSE /\ readPending' = [readPending EXCEPT ![self] = ShouldReadMore(self) /\ ~backoffTimer[self]]
                                      /\ UNCHANGED << loaded, ackedInMem, 
                                                      readLevel, ackLevel, 
                                                      atEnd, newlyWritten, 
                                                      evictedAcks, committed >>
                           /\ pc' = [pc EXCEPT ![self] = "RWait"]
                           /\ UNCHANGED << rdState, rdFrom, rdMax >>
                /\ UNCHANGED << dbTasks, metaRange, metaAck, ostate, ownRange, 
                                tkRange, tkAck, confirmedAck, gcVerified, 
                                rdResult, rdOk, wrState, wrBatch, wrRes, 
                                pinned, backoffTimer, usedLevels, everInDb, 
                                ackedGhost, gcVictims, stuckFlag >>

RResp(self) == /\ pc[self] = "RResp"
               /\ rdState[self] = "resp" /\ Alive(self)
               /\ rdState' = [rdState EXCEPT ![self] = "idle"]
               /\ IF rdOk[self]
                     THEN /\ \E expSel \in ExpSubsets(rdResult[self]):
                               LET mode == IF Cardinality(rdResult[self]) < rdMax[self]
                                           THEN MToEnd ELSE MMiddle IN
                                 LET r == MergeResult(loaded[self], ackedInMem[self], readLevel[self],
                                                      ackLevel[self], atEnd[self], rdResult[self],
                                                      mode, pinned[self] \/ newlyWritten[self] /= {},
                                                      expSel, evictedAcks[self]) IN
                                   /\ loaded' = [loaded EXCEPT ![self] = r.loaded]
                                   /\ ackedInMem' = [ackedInMem EXCEPT ![self] = r.acks]
                                   /\ readLevel' = [readLevel EXCEPT ![self] = r.rl]
                                   /\ ackLevel' = [ackLevel EXCEPT ![self] = r.al]
                                   /\ atEnd' = [atEnd EXCEPT ![self] = r.atEnd]
                                   /\ evictedAcks' = [evictedAcks EXCEPT ![self] = r.cache]
                                   /\ committed' = committed \ r.consumed
                          /\ rdResult' = [rdResult EXCEPT ![self] = {}]
                          /\ rdFrom' = [rdFrom EXCEPT ![self] = NoLevel]
                          /\ rdMax' = [rdMax EXCEPT ![self] = 0]
                          /\ pc' = [pc EXCEPT ![self] = "RCheck"]
                          /\ UNCHANGED backoffTimer
                     ELSE /\ rdFrom' = [rdFrom EXCEPT ![self] = NoLevel]
                          /\ rdMax' = [rdMax EXCEPT ![self] = 0]
                          /\ IF ~backoffTimer[self]
                                THEN /\ backoffTimer' = [backoffTimer EXCEPT ![self] = TRUE]
                                ELSE /\ TRUE
                                     /\ UNCHANGED backoffTimer
                          /\ pc' = [pc EXCEPT ![self] = "RExit"]
                          /\ UNCHANGED << rdResult, loaded, ackedInMem, 
                                          readLevel, ackLevel, atEnd, 
                                          evictedAcks, committed >>
               /\ UNCHANGED << dbTasks, metaRange, metaAck, ostate, ownRange, 
                               tkRange, tkAck, confirmedAck, gcVerified, rdOk, 
                               wrState, wrBatch, wrRes, readPending, 
                               newlyWritten, pinned, usedLevels, everInDb, 
                               ackedGhost, gcVictims, stuckFlag >>

RExit(self) == /\ pc[self] = "RExit"
               /\ Alive(self)
               /\ IF newlyWritten[self] /= {}
                     THEN /\ \E expSel \in ExpSubsets(newlyWritten[self]):
                               LET r == MergeResult(loaded[self], ackedInMem[self], readLevel[self], ackLevel[self],
                                                    atEnd[self], newlyWritten[self], MWrite, pinned[self], expSel,
                                                    evictedAcks[self]) IN
                                 /\ loaded' = [loaded EXCEPT ![self] = r.loaded]
                                 /\ ackedInMem' = [ackedInMem EXCEPT ![self] = r.acks]
                                 /\ readLevel' = [readLevel EXCEPT ![self] = r.rl]
                                 /\ ackLevel' = [ackLevel EXCEPT ![self] = r.al]
                                 /\ atEnd' = [atEnd EXCEPT ![self] = r.atEnd]
                                 /\ evictedAcks' = [evictedAcks EXCEPT ![self] = r.cache]
                                 /\ committed' = committed \ r.consumed
                                 /\ newlyWritten' = [newlyWritten EXCEPT ![self] = {}]
                                 /\ readPending' = [readPending EXCEPT ![self] = ~r.atEnd /\ Cardinality(r.loaded) <= ReloadAt
                                                                                 /\ ~backoffTimer[self]]
                     ELSE /\ readPending' = [readPending EXCEPT ![self] = ShouldReadMore(self) /\ ~backoffTimer[self]]
                          /\ UNCHANGED << loaded, ackedInMem, readLevel, 
                                          ackLevel, atEnd, newlyWritten, 
                                          evictedAcks, committed >>
               /\ pc' = [pc EXCEPT ![self] = "RWait"]
               /\ UNCHANGED << dbTasks, metaRange, metaAck, ostate, ownRange, 
                               tkRange, tkAck, confirmedAck, gcVerified, 
                               rdState, rdFrom, rdMax, rdResult, rdOk, wrState, 
                               wrBatch, wrRes, pinned, backoffTimer, 
                               usedLevels, everInDb, ackedGhost, gcVictims, 
                               stuckFlag >>

reader(self) == RWait(self) \/ RCheck(self) \/ RResp(self) \/ RExit(self)

TimerLoop(self) == /\ pc[self] = "TimerLoop"
                   /\ LET o == Own(self) IN
                        /\ backoffTimer[o] /\ Alive(o)
                        /\ backoffTimer' = [backoffTimer EXCEPT ![o] = FALSE]
                        /\ IF ~readPending[o] /\ ShouldReadMore(o)
                              THEN /\ readPending' = [readPending EXCEPT ![o] = TRUE]
                              ELSE /\ TRUE
                                   /\ UNCHANGED readPending
                   /\ pc' = [pc EXCEPT ![self] = "TimerLoop"]
                   /\ UNCHANGED << dbTasks, metaRange, metaAck, ostate, 
                                   ownRange, tkRange, tkAck, confirmedAck, 
                                   gcVerified, rdState, rdFrom, rdMax, 
                                   rdResult, rdOk, wrState, wrBatch, wrRes, 
                                   loaded, ackedInMem, readLevel, ackLevel, 
                                   atEnd, newlyWritten, pinned, evictedAcks, 
                                   usedLevels, everInDb, committed, ackedGhost, 
                                   gcVictims, stuckFlag >>

timer(self) == TimerLoop(self)

WLoop(self) == /\ pc[self] = "WLoop"
               /\ LET o == Own(self) IN
                    /\ Alive(o)
                    /\ \/ /\ \E B \in WriteBatches(o):
                               /\ pinned' = [pinned EXCEPT ![o] = TRUE]
                               /\ wrBatch' = [wrBatch EXCEPT ![o] = B]
                               /\ usedLevels' = (usedLevels \cup B)
                               /\ wrState' = [wrState EXCEPT ![o] = "req"]
                          /\ pc' = [pc EXCEPT ![self] = "WResp"]
                       \/ /\ WriterMayStop
                          /\ pc' = [pc EXCEPT ![self] = "WDone"]
                          /\ UNCHANGED <<wrState, wrBatch, pinned, usedLevels>>
               /\ UNCHANGED << dbTasks, metaRange, metaAck, ostate, ownRange, 
                               tkRange, tkAck, confirmedAck, gcVerified, 
                               rdState, rdFrom, rdMax, rdResult, rdOk, wrRes, 
                               loaded, ackedInMem, readLevel, ackLevel, atEnd, 
                               readPending, newlyWritten, evictedAcks, 
                               backoffTimer, everInDb, committed, ackedGhost, 
                               gcVictims, stuckFlag >>

WResp(self) == /\ pc[self] = "WResp"
               /\ LET o == Own(self) IN
                    /\ wrState[o] = "resp" /\ Alive(o)
                    /\ wrState' = [wrState EXCEPT ![o] = "idle"]
                    /\ IF wrRes[o] = "ok"
                          THEN /\ IF readPending[o]
                                     THEN /\ newlyWritten' = [newlyWritten EXCEPT ![o] = newlyWritten[o] \cup wrBatch[o]]
                                          /\ UNCHANGED << loaded, ackedInMem, 
                                                          readLevel, ackLevel, 
                                                          atEnd, readPending, 
                                                          evictedAcks, 
                                                          committed, stuckFlag >>
                                     ELSE /\ \E expSel \in ExpSubsets(wrBatch[o]):
                                               LET r == MergeResult(loaded[o], ackedInMem[o], readLevel[o],
                                                                    ackLevel[o], atEnd[o], wrBatch[o], MWrite, TRUE,
                                                                    expSel, evictedAcks[o]) IN
                                                 /\ loaded' = [loaded EXCEPT ![o] = r.loaded]
                                                 /\ ackedInMem' = [ackedInMem EXCEPT ![o] = r.acks]
                                                 /\ readLevel' = [readLevel EXCEPT ![o] = r.rl]
                                                 /\ ackLevel' = [ackLevel EXCEPT ![o] = r.al]
                                                 /\ atEnd' = [atEnd EXCEPT ![o] = r.atEnd]
                                                 /\ evictedAcks' = [evictedAcks EXCEPT ![o] = r.cache]
                                                 /\ committed' = committed \ r.consumed
                                                 /\ stuckFlag' = (stuckFlag \/ (r.stuck /\ ~readPending[o] /\ ~backoffTimer[o]))
                                                 /\ IF r.stuck /\ ~readPending[o] /\ ~backoffTimer[o]
                                                       THEN /\ readPending' = [readPending EXCEPT ![o] = TRUE]
                                                       ELSE /\ TRUE
                                                            /\ UNCHANGED readPending
                                          /\ UNCHANGED newlyWritten
                               /\ UNCHANGED ostate
                          ELSE /\ IF wrRes[o] = "condfail"
                                     THEN /\ ostate' = [ostate EXCEPT ![o] = "dead"]
                                     ELSE /\ TRUE
                                          /\ UNCHANGED ostate
                               /\ UNCHANGED << loaded, ackedInMem, readLevel, 
                                               ackLevel, atEnd, readPending, 
                                               newlyWritten, evictedAcks, 
                                               committed, stuckFlag >>
               /\ pc' = [pc EXCEPT ![self] = "WUnpin"]
               /\ UNCHANGED << dbTasks, metaRange, metaAck, ownRange, tkRange, 
                               tkAck, confirmedAck, gcVerified, rdState, 
                               rdFrom, rdMax, rdResult, rdOk, wrBatch, wrRes, 
                               pinned, backoffTimer, usedLevels, everInDb, 
                               ackedGhost, gcVictims >>

WUnpin(self) == /\ pc[self] = "WUnpin"
                /\ LET o == Own(self) IN
                     /\ Alive(o)
                     /\ IF wrRes[o] /= "ok"
                           THEN /\ atEnd' = [atEnd EXCEPT ![o] = FALSE]
                                /\ IF ~readPending[o] /\ ~backoffTimer[o] /\ LoadedCount(o) <= ReloadAt
                                      THEN /\ readPending' = [readPending EXCEPT ![o] = TRUE]
                                      ELSE /\ TRUE
                                           /\ UNCHANGED readPending
                           ELSE /\ TRUE
                                /\ UNCHANGED << atEnd, readPending >>
                     /\ pinned' = [pinned EXCEPT ![o] = FALSE]
                     /\ wrBatch' = [wrBatch EXCEPT ![o] = {}]
                     /\ wrRes' = [wrRes EXCEPT ![o] = "ok"]
                     /\ LET clear == IF newlyWritten[o] /= {} THEN {}
                                     ELSE {c \in ackedInMem[o] : \A m \in loaded[o] : c < m} IN
                          /\ ackedInMem' = [ackedInMem EXCEPT ![o] = ackedInMem[o] \ clear]
                          /\ IF clear /= {}
                                THEN /\ ackLevel' = [ackLevel EXCEPT ![o] = SetMax(clear)]
                                ELSE /\ TRUE
                                     /\ UNCHANGED ackLevel
                /\ pc' = [pc EXCEPT ![self] = "WLoop"]
                /\ UNCHANGED << dbTasks, metaRange, metaAck, ostate, ownRange, 
                                tkRange, tkAck, confirmedAck, gcVerified, 
                                rdState, rdFrom, rdMax, rdResult, rdOk, 
                                wrState, loaded, readLevel, newlyWritten, 
                                evictedAcks, backoffTimer, usedLevels, 
                                everInDb, committed, ackedGhost, gcVictims, 
                                stuckFlag >>

WDone(self) == /\ pc[self] = "WDone"
               /\ TRUE
               /\ pc' = [pc EXCEPT ![self] = "Done"]
               /\ UNCHANGED << dbTasks, metaRange, metaAck, ostate, ownRange, 
                               tkRange, tkAck, confirmedAck, gcVerified, 
                               rdState, rdFrom, rdMax, rdResult, rdOk, wrState, 
                               wrBatch, wrRes, loaded, ackedInMem, readLevel, 
                               ackLevel, atEnd, readPending, newlyWritten, 
                               pinned, evictedAcks, backoffTimer, usedLevels, 
                               everInDb, committed, ackedGhost, gcVictims, 
                               stuckFlag >>

writer(self) == WLoop(self) \/ WResp(self) \/ WUnpin(self) \/ WDone(self)

DbReadLoop(self) == /\ pc[self] = "DbReadLoop"
                    /\ LET o == Own(self) IN
                         /\ rdState[o] = "req" /\ Alive(o)
                         /\ \/ /\ rdResult' = [rdResult EXCEPT ![o] = KeepLowest({l \in dbTasks : l >= rdFrom[o]}, rdMax[o])]
                               /\ rdOk' = [rdOk EXCEPT ![o] = TRUE]
                            \/ /\ ReadTimeouts
                               /\ rdOk' = [rdOk EXCEPT ![o] = FALSE]
                               /\ UNCHANGED rdResult
                         /\ rdState' = [rdState EXCEPT ![o] = "resp"]
                    /\ pc' = [pc EXCEPT ![self] = "DbReadLoop"]
                    /\ UNCHANGED << dbTasks, metaRange, metaAck, ostate, 
                                    ownRange, tkRange, tkAck, confirmedAck, 
                                    gcVerified, rdFrom, rdMax, wrState, 
                                    wrBatch, wrRes, loaded, ackedInMem, 
                                    readLevel, ackLevel, atEnd, readPending, 
                                    newlyWritten, pinned, evictedAcks, 
                                    backoffTimer, usedLevels, everInDb, 
                                    committed, ackedGhost, gcVictims, 
                                    stuckFlag >>

dbRead(self) == DbReadLoop(self)

DbWriteLoop(self) == /\ pc[self] = "DbWriteLoop"
                     /\ LET o == Own(self) IN
                          /\ wrState[o] = "req" /\ Alive(o)
                          /\ IF ownRange[o] /= metaRange
                                THEN /\ wrRes' = [wrRes EXCEPT ![o] = "condfail"]
                                     /\ UNCHANGED << dbTasks, everInDb, 
                                                     committed >>
                                ELSE /\ \/ /\ dbTasks' = (dbTasks \cup wrBatch[o])
                                           /\ everInDb' = (everInDb \cup wrBatch[o])
                                           /\ committed' = (committed \cup wrBatch[o])
                                           /\ wrRes' = [wrRes EXCEPT ![o] = "ok"]
                                        \/ /\ WriteTimeouts
                                           /\ dbTasks' = (dbTasks \cup wrBatch[o])
                                           /\ everInDb' = (everInDb \cup wrBatch[o])
                                           /\ wrRes' = [wrRes EXCEPT ![o] = "timeout"]
                                           /\ UNCHANGED committed
                                        \/ /\ WriteTimeouts
                                           /\ wrRes' = [wrRes EXCEPT ![o] = "timeout"]
                                           /\ UNCHANGED <<dbTasks, everInDb, committed>>
                          /\ wrState' = [wrState EXCEPT ![o] = "resp"]
                     /\ pc' = [pc EXCEPT ![self] = "DbWriteLoop"]
                     /\ UNCHANGED << metaRange, metaAck, ostate, ownRange, 
                                     tkRange, tkAck, confirmedAck, gcVerified, 
                                     rdState, rdFrom, rdMax, rdResult, rdOk, 
                                     wrBatch, loaded, ackedInMem, readLevel, 
                                     ackLevel, atEnd, readPending, 
                                     newlyWritten, pinned, evictedAcks, 
                                     backoffTimer, usedLevels, ackedGhost, 
                                     gcVictims, stuckFlag >>

dbWrite(self) == DbWriteLoop(self)

SyncLoop(self) == /\ pc[self] = "SyncLoop"
                  /\ LET o == Own(self) IN
                       /\ Alive(o)
                       /\ IF ownRange[o] /= metaRange
                             THEN /\ ostate' = [ostate EXCEPT ![o] = "dead"]
                                  /\ UNCHANGED << metaAck, confirmedAck >>
                             ELSE /\ metaAck' = ackLevel[o]
                                  /\ IF GcMode = "persisted"
                                        THEN /\ confirmedAck' = [confirmedAck EXCEPT ![o] = ackLevel[o]]
                                        ELSE /\ TRUE
                                             /\ UNCHANGED confirmedAck
                                  /\ UNCHANGED ostate
                  /\ pc' = [pc EXCEPT ![self] = "SyncLoop"]
                  /\ UNCHANGED << dbTasks, metaRange, ownRange, tkRange, tkAck, 
                                  gcVerified, rdState, rdFrom, rdMax, rdResult, 
                                  rdOk, wrState, wrBatch, wrRes, loaded, 
                                  ackedInMem, readLevel, ackLevel, atEnd, 
                                  readPending, newlyWritten, pinned, 
                                  evictedAcks, backoffTimer, usedLevels, 
                                  everInDb, committed, ackedGhost, gcVictims, 
                                  stuckFlag >>

sync(self) == SyncLoop(self)

GcLoop(self) == /\ pc[self] = "GcLoop"
                /\ LET o == Own(self) IN
                     /\ Alive(o)
                     /\ \/ /\ GcMode = "verified" /\ ~gcVerified[o]
                           /\ IF ownRange[o] /= metaRange
                                 THEN /\ ostate' = [ostate EXCEPT ![o] = "dead"]
                                      /\ UNCHANGED gcVerified
                                 ELSE /\ gcVerified' = [gcVerified EXCEPT ![o] = TRUE]
                                      /\ UNCHANGED ostate
                           /\ UNCHANGED <<dbTasks, gcVictims>>
                        \/ /\ GcLevel(o) > NoLevel /\ (GcMode = "verified" => gcVerified[o])
                           /\ IF GcMode = "verified"
                                 THEN /\ gcVerified' = [gcVerified EXCEPT ![o] = FALSE]
                                 ELSE /\ TRUE
                                      /\ UNCHANGED gcVerified
                           /\ IF GcMode /= "fenced" \/ ownRange[o] = metaRange
                                 THEN /\ LET victims == {l \in dbTasks : l <= GcLevel(o)} IN
                                           /\ dbTasks' = dbTasks \ victims
                                           /\ gcVictims' = (gcVictims \cup victims)
                                 ELSE /\ TRUE
                                      /\ UNCHANGED << dbTasks, gcVictims >>
                           /\ UNCHANGED ostate
                /\ pc' = [pc EXCEPT ![self] = "GcLoop"]
                /\ UNCHANGED << metaRange, metaAck, ownRange, tkRange, tkAck, 
                                confirmedAck, rdState, rdFrom, rdMax, rdResult, 
                                rdOk, wrState, wrBatch, wrRes, loaded, 
                                ackedInMem, readLevel, ackLevel, atEnd, 
                                readPending, newlyWritten, pinned, evictedAcks, 
                                backoffTimer, usedLevels, everInDb, committed, 
                                ackedGhost, stuckFlag >>

gc(self) == GcLoop(self)

AckLoop(self) == /\ pc[self] = "AckLoop"
                 /\ LET o == Own(self) IN
                      /\ Alive(o) /\ loaded[o] /= {}
                      /\ \E l \in loaded[o]:
                           LET ld == loaded[o] \ {l} IN
                             LET ackd == ackedInMem[o] \cup {l} IN
                               LET clear == IF pinned[o] \/ newlyWritten[o] /= {} THEN {}
                                            ELSE {c \in ackd : \A m \in ld : c < m} IN
                                 /\ loaded' = [loaded EXCEPT ![o] = ld]
                                 /\ ackedInMem' = [ackedInMem EXCEPT ![o] = ackd \ clear]
                                 /\ IF clear /= {}
                                       THEN /\ ackLevel' = [ackLevel EXCEPT ![o] = SetMax(clear)]
                                       ELSE /\ TRUE
                                            /\ UNCHANGED ackLevel
                                 /\ ackedGhost' = (ackedGhost \cup {l})
                                 /\ IF ~readPending[o] /\ ~atEnd[o] /\ Cardinality(ld) <= ReloadAt
                                       /\ ~backoffTimer[o]
                                       THEN /\ readPending' = [readPending EXCEPT ![o] = TRUE]
                                       ELSE /\ TRUE
                                            /\ UNCHANGED readPending
                 /\ pc' = [pc EXCEPT ![self] = "AckLoop"]
                 /\ UNCHANGED << dbTasks, metaRange, metaAck, ostate, ownRange, 
                                 tkRange, tkAck, confirmedAck, gcVerified, 
                                 rdState, rdFrom, rdMax, rdResult, rdOk, 
                                 wrState, wrBatch, wrRes, readLevel, atEnd, 
                                 newlyWritten, pinned, evictedAcks, 
                                 backoffTimer, usedLevels, everInDb, committed, 
                                 gcVictims, stuckFlag >>

acker(self) == AckLoop(self)

Next == (\E self \in {TkId(o) : o \in 2..NumOwners}: takeover(self))
           \/ (\E self \in {RdId(o) : o \in Owners}: reader(self))
           \/ (\E self \in {TmId(o) : o \in Owners}: timer(self))
           \/ (\E self \in {WrId(o) : o \in Owners}: writer(self))
           \/ (\E self \in {DrId(o) : o \in Owners}: dbRead(self))
           \/ (\E self \in {DwId(o) : o \in Owners}: dbWrite(self))
           \/ (\E self \in {SyId(o) : o \in Owners}: sync(self))
           \/ (\E self \in {GcId(o) : o \in Owners}: gc(self))
           \/ (\E self \in {AkId(o) : o \in Owners}: acker(self))

Spec == /\ Init /\ [][Next]_vars
        /\ \A self \in {RdId(o) : o \in Owners} : WF_vars(reader(self))
        /\ \A self \in {TmId(o) : o \in Owners} : WF_vars(timer(self))
        /\ \A self \in {WrId(o) : o \in Owners} : WF_vars(writer(self))
        /\ \A self \in {DrId(o) : o \in Owners} : WF_vars(dbRead(self))
        /\ \A self \in {DwId(o) : o \in Owners} : WF_vars(dbWrite(self))
        /\ \A self \in {SyId(o) : o \in Owners} : WF_vars(sync(self))
        /\ \A self \in {AkId(o) : o \in Owners} : WF_vars(acker(self))

\* END TRANSLATION

---------------------------------------------------------------------------
(* Fairness *)

(* The acker step of owner o that acks level l. *)
AckOf(o, l) == AckLoop(AkId(o)) /\ l \in loaded[o] /\ l \notin loaded'[o]

(* Per-level strong fairness on every owner's acker; see FairQueue.tla. *)
AckerFairness == \A o \in Owners, l \in Levels : SF_vars(AckOf(o, l))

(* Reads eventually succeed; see FairQueue.tla. *)
DbReadSuccess(o) ==
  DbReadLoop(DrId(o)) /\ rdState[o] = "req" /\ rdState'[o] = "resp" /\ rdOk'[o]

FairSpec == Spec /\ AckerFairness /\ \A o \in Owners : SF_vars(DbReadSuccess(o))

---------------------------------------------------------------------------
(* Invariants *)

TypeInv ==
  /\ \A o \in Owners :
       /\ ostate[o] \in {"idle", "taking", "active", "dead"}
       /\ loaded[o] \subseteq Levels
       /\ ackedInMem[o] \subseteq Levels
       /\ loaded[o] \cap ackedInMem[o] = {}
       /\ ackLevel[o] \in NoLevel..MaxLevel
       /\ readLevel[o] \in NoLevel..MaxLevel
       /\ newlyWritten[o] \subseteq Levels
       /\ wrBatch[o] \subseteq Levels
       /\ evictedAcks[o] \subseteq Levels
       /\ confirmedAck[o] <= ackLevel[o]
  /\ metaAck \in NoLevel..MaxLevel
  /\ dbTasks \subseteq everInDb
  /\ everInDb \subseteq usedLevels
  /\ committed \subseteq everInDb
  \* at most one owner holds the persisted range id
  /\ \A o, p \in Owners : Current(o) /\ Current(p) => o = p

\* Per-owner bookkeeping (holds for stale owners too: it's local state).
MemWindow ==
  \A o \in Owners : \A l \in loaded[o] \cup ackedInMem[o] :
    ackLevel[o] < l /\ l <= readLevel[o]
AckBelowRead == \A o \in Owners : ackLevel[o] <= readLevel[o]
LoadedBounded == \A o \in Owners : Cardinality(loaded[o]) <= BatchTarget
CacheBounded == \A o \in Owners : Cardinality(evictedAcks[o]) <= EvictedCacheMax
CacheOnlyAcked == \A o \in Owners : (evictedAcks[o] \cap committed) \subseteq ackedGhost
PinProtectsWrites ==
  \A o \in Owners :
    /\ pinned[o] => \A l \in wrBatch[o] : l > ackLevel[o]
    /\ \A l \in newlyWritten[o] : l > ackLevel[o]

\* The current owner's in-memory tasks are in the db. (A stale owner's may
\* legitimately have been acked and GC'd by the new owner.)
LoadedInDb ==
  \A o \in Owners : Current(o) =>
    loaded[o] \subseteq dbTasks /\ newlyWritten[o] \subseteq dbTasks

\* The current owner's ack level never passes an unacked committed task. (A
\* stale owner's may: a newer owner can write below it.)
NoAckSkipped ==
  \A o \in Owners : Current(o) =>
    \A l \in committed : (l <= ackLevel[o]) => (l \in ackedGhost)

\* The persisted ack level (what the next owner starts from) never passes an
\* unacked committed task.
PersistedAckSafe ==
  \A l \in committed : (l <= metaAck) => (l \in ackedGhost)

\* THE safety property: a committed task is never deleted before it was
\* acked, by any owner, stale or not.
GCOnlyAcked == (gcVictims \cap committed) \subseteq ackedGhost

NoStuck == ~stuckFlag

---------------------------------------------------------------------------
(* State-space reduction for safety-only runs (cfg VIEW): a dead owner's    *)
(* processes are all disabled forever and nothing reads its local state,   *)
(* so states that differ only in a dead owner's locals are equivalent.     *)

DeadMask(f, dflt) == [o \in Owners |-> IF ostate[o] = "dead" THEN dflt ELSE f[o]]

View == <<
  [p \in DOMAIN pc |-> IF ostate[Own(p)] = "dead" THEN "dead" ELSE pc[p]],
  dbTasks, metaRange, metaAck, ostate, ownRange, tkRange, tkAck,
  DeadMask(confirmedAck, NoLevel), DeadMask(gcVerified, FALSE),
  DeadMask(rdState, "idle"), DeadMask(rdFrom, NoLevel), DeadMask(rdMax, 0),
  DeadMask(rdResult, {}), DeadMask(rdOk, TRUE),
  DeadMask(wrState, "idle"), DeadMask(wrBatch, {}), DeadMask(wrRes, "ok"),
  DeadMask(loaded, {}), DeadMask(ackedInMem, {}), DeadMask(readLevel, NoLevel),
  DeadMask(ackLevel, NoLevel), DeadMask(atEnd, FALSE),
  DeadMask(readPending, FALSE), DeadMask(newlyWritten, {}),
  DeadMask(pinned, FALSE), DeadMask(evictedAcks, {}),
  DeadMask(backoffTimer, FALSE),
  usedLevels, everInDb, committed, ackedGhost, gcVictims, stuckFlag >>

---------------------------------------------------------------------------
(* Temporal properties *)

AckLevelMonotonic == [][\A o \in Owners : ackLevel'[o] >= ackLevel[o]]_vars

AllTasksAcked == <>(\A l \in committed : l \in ackedGhost)

\* The current owner eventually drains the queue and stays drained.
EventuallyDrained ==
  <>[](\E o \in Owners : Current(o) /\ atEnd[o] /\ loaded[o] = {})

===========================================================================

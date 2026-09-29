# TLA+ model of the fair task queue

A PlusCal/TLA+ model of `service/matching/fair_task_{reader,writer}.go`,
checking correctness and liveness properties of the reader/writer/acker/GC
machinery over an unreliable database. See `plan.md` for goals and
milestones, `findings.md` for issues surfaced by the model.

## Files

- `FairQueue.tla` — the model (PlusCal; the TLA+ translation is embedded).
- `FairQueue.cfg` — default config: safety + liveness at MaxLevel=3.
- `FairQueue_safety4.cfg` — safety only at MaxLevel=4 (liveness at 4 is too
  slow for routine runs; run it manually before big changes if desired).
- `FairQueue_churn.cfg` — findings.md #1 regression: a task that never
  completes makes the reader busy-loop instead of quiescing (expected to
  VIOLATE ReaderQuiesce on current code).
- `run.sh` — translate + check the real model, then check every mutation
  and verify TLC catches it, then reproduce the confirmed findings.
- `Trivial.tla/.cfg` — toolchain smoke test.
- `FairQueueOwners.tla` — copy of the model extended with partition
  ownership changes (see "Ownership model" below).
- `FairQueueOwners.cfg` — ownership model, safety only (with a VIEW that
  collapses dead owners' state); as checked in, current code, which
  VIOLATES GCOnlyAcked (findings.md #4).
- `FairQueueOwners_live.cfg` — ownership model, safety + liveness at
  MaxLevel=2 (no VIEW).
- `run_owners.sh` — current code must fail; each candidate GC fix must pass
  or show its expected violation. Slow (~2h total: the passing safety runs
  take 15-30 min each, liveness ~40 min each).

## Running

```sh
./run.sh                  # full suite (real model + all mutations)
# single run:
java -cp ../tla2tools.jar pcal.trans -nocfg FairQueue.tla
java -XX:+UseParallelGC -cp ../tla2tools.jar tlc2.TLC -workers auto FairQueue.tla
```

Note: always translate with `-nocfg` or pcal.trans clobbers the hand-written
`.cfg`.

## Model structure

Processes: `reader` (readTasksImpl loop), `timer` (read-retry backoff),
`writer` (taskWriterLoop/writeBatch), `acker` (completeTask calls),
`gc` (maybeGC/doGC), and `dbRead`/`dbWrite`/`dbGc` (the database serving
each RPC channel). Requests/responses are separate steps, so RPCs interleave
with everything.

Go's `tr.lock` critical sections map to single atomic PlusCal steps;
`mergeTasksLocked` is the pure operator `MergeResult` composed atomically at
each call site.

Key abstractions (see the header comment in FairQueue.tla for the full
list):

- Fair levels `<pass, id>` are plain integers: the logic only compares
  levels, so this preserves all orderings. The stride counter is abstracted
  to "writer picks any unused levels above the pinned ack level".
- DB calls may time out with the operation applied (incoming timeout) or
  not applied (outgoing). Liveness assumes only that *reads* succeed
  infinitely often if attempted infinitely often (SF on read success).
- Only "committed" tasks (initial backlog + writes whose RPC succeeded)
  carry delivery guarantees; rows landed by timed-out writes are
  unguaranteed duplicates (the caller re-submits).
- The acker has per-level strong fairness: every loaded task is eventually
  acked, even across evict/re-read cycles.
- Not modeled: subqueues, ownership/fencing (separate model if needed),
  matcher handoff races, explicit DB errors, throttling.

## Properties

Safety (invariants):
- `MemWindow`: in-memory entries are exactly within (ackLevel, readLevel].
- `NoAckSkipped`: the ack level never passes an unacked committed task.
- `GCOnlyAcked`: GC never deletes an unacked committed task.
- `PinProtectsWrites`: the write pin keeps ackLevel below in-flight writes.
- `CacheOnlyAcked`/`CacheBounded`: the evicted-ack cache never fabricates
  an ack and stays within its size bound.
- `NoStuck`: the defensive "fair reader stuck" softassert does not fire.
  NOT an invariant of current code (findings.md #3) — used by run.sh as a
  findings regression and for historical mutations with StuckRepair=FALSE.
- plus type/bookkeeping invariants (`TypeInv`, `AckBelowRead`, `LoadedInDb`,
  `LoadedBounded`).

Liveness (under the fairness assumptions above):
- `AllTasksAcked`: every committed task is eventually acked.
- `EventuallyDrained`: the reader eventually reaches (and keeps) the
  drained state: atEnd with nothing loaded.
- `AckLevelMonotonic`: the ack level never moves backwards.

## Mutation tests

Each `Mut*` constant re-introduces one bug (historical bugs are tagged with
their fixing commit; "seeded" ones are synthetic). `run.sh` checks that TLC
finds the expected violation for each — a milestone isn't trusted until its
target bugs are demonstrably caught. All flags FALSE = current code.

## Ownership model

`FairQueueOwners.tla` adds what `FairQueue.tla` leaves out: owners
1..NumOwners taking over the partition in turn, each with its own
reader/writer/acker/GC/sync processes over the shared database. It models
the persisted metadata (range id and fair ack level), the takeover in
`takeOverTaskQueueLocked` (plain read, then an LWT on the range id that
writes back the metadata as read), range-id fencing of `CreateTasks` and
`SyncState` (ConditionFailed -> the owner unloads), and the *absence* of
fencing on `GetTasks` and `CompleteTasksLessThan`. A stale owner keeps
reading, acking and GCing until a fenced call tells it otherwise.

Mutation flags are dropped (FairQueue.tla covers them); read timeouts,
write timeouts and expiry are switchable constants so the two-owner state
space stays tractable. `GcMode` selects current code (`"unfenced"`) or a
candidate fix; `TakeoverCAS` makes the takeover LWT also require the
persisted ack level to be unchanged since its read. See the header comment
of FairQueueOwners.tla for the abstractions and findings.md #4 for results.

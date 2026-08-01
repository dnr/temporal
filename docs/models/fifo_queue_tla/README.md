# TLA+ model of the fifo task queue reader

A PlusCal/TLA+ model of `service/matching/pri_task_{reader,writer}.go` (the
fifo, task-id ordered backlog; the reader is called `priTaskReader` for
historical reasons) plus the ack-level bookkeeping in
`service/matching/db.go`.

It was built around the bugs fixed in `e0a4751c` ("Fix read and ack levels
moving backwards in priTaskReader"): those bugs do not break at-least-once
delivery, they let the read level and the ack level move *backwards*, which
causes redundant reads and duplicate dispatch. Each hunk of that fix is a
mutation flag, so the suite proves the model catches the bug and that the
fix removes it. See `findings.md` for issues the model surfaced.

This is a sibling of `../fairness_queue_tla`, which models the *fair* queue
reader. The two readers share a shape but almost none of their mechanics:
the fair reader keeps `atEnd`/eviction/merge state, while this one relies on
task ids and the db's `maxReadLevel`.

## Files

- `PriQueue.tla` — the model (PlusCal; the TLA+ translation is embedded).
- `PriQueue.cfg` — default config: safety + liveness at MaxLevel=3.
- `PriQueue_levels.cfg` — same model, but only the "levels never move
  backwards / no task is dispatched twice" properties, so the e0a4751c
  mutations report *those* rather than whichever bookkeeping invariant
  happens to trip first.
- `PriQueue_safety4.cfg` — safety only at MaxLevel=4 (liveness at 4 is too
  slow for the routine suite).
- `PriQueue_caughtup.cfg` — writes never fail; adds `EventuallyCaughtUp`
  (the reader always reaches the max read level and persists it). Used both
  as a real-model check and, with `WriteErrors = TRUE` or
  `Expirable = {2}`, to reproduce findings.md #1 and #2.
- `run.sh` — translate, check the real model, then check every mutation and
  verify TLC catches it, and reproduce the confirmed finding.

## Running

```sh
./run.sh                  # full suite (~10 min)
# single run:
java -cp ~/tla2tools.jar pcal.trans -nocfg PriQueue.tla
java -XX:+UseParallelGC -cp ~/tla2tools.jar tlc2.TLC -workers auto PriQueue.tla
```

`JAR=/path/to/tla2tools.jar ./run.sh` if the jar isn't at `~/tla2tools.jar`.
Always translate with `-nocfg` or `pcal.trans` clobbers the hand-written
`.cfg`.

## Model structure

Processes: `reader` (getTasksPump + getTaskBatch + processTaskBatch),
`timer` (read-retry backoff), `writer` (taskWriterLoop → CreateTasks →
signalNewTasks), `acker` (completeTask), `gc` (maybeGC/doGC), the
`dbRead`/`dbWrite`/`dbGc` processes serving each RPC channel, and `heal`
(the network eventually stops timing out reads). Requests and responses are
separate steps, so RPCs interleave with everything.

Go's `tr.lock` critical sections map to single atomic PlusCal steps. The
race at the heart of e0a4751c comes from the boundaries *between* them:
`getTaskBatch` snapshots `tr.readLevel` under `tr.lock`, then reads
`db.GetMaxReadLevel` without it, then does its `GetTasks` calls without it,
and only then does the pump apply the result — while `signalNewTasks` can
run at any point in between.

Key abstractions (see the header comment in `PriQueue.tla` for the full
list):

- Task ids are `1..MaxLevel`; `1..InitLevels` is the initial backlog (any
  subset, so id gaps are covered) and the writer allocates above the db's
  max read level, possibly with gaps — which is what ids burned by failed
  writes, id-block boundaries and other subqueues' ids look like here.
- `db.CreateTasks` holds `db.Lock` across the store call and advances
  `maxReadLevel` before returning *whether or not the write succeeded*, so
  it is one atomic db step. A write may fail with the rows landed or not;
  on failure `priTaskWriter` does not call `signalReaders` at all.
- Only "committed" tasks (initial backlog + writes whose RPC succeeded)
  carry delivery guarantees; rows landed by failed writes are unguaranteed
  duplicates (the caller re-submits with a fresh id).
- The acker has per-level strong fairness: every loaded task is eventually
  acked. GC is deliberately unfair: liveness must not depend on it.
- Not modeled: multiple subqueues, ownership/fencing and rangeid renewal,
  the approximate backlog counter, matcher handoff and respooling,
  throttling, forwarding.

## Properties

Safety (invariants):
- `MemWindow`: in-memory entries are exactly within (ackLevel, readLevel].
- `AckBelowRead`: the ack level never passes the read level.
- `ReadBelowMaxRead`: the reader never moves its levels past an id that
  could still be written — the property that makes "scanned that range and
  found nothing" a safe conclusion.
- `NoAckSkipped`: neither the in-memory nor the persisted ack level ever
  passes an unacked committed task.
- `GCOnlyAcked`: GC never deletes an unacked committed task.
- `LoadedInDb`, `LoadedBounded`, `TypeInv`: bookkeeping.
- `NoReDispatch`: no task is ever handed to the matcher twice. Duplicate
  dispatch is permitted by the at-least-once contract, but it is wasted
  work — and it is the damage the e0a4751c bugs actually cause.
- `NoDbAckBackwards` / `NoWriteDup`: the two softasserts on this path
  ("ack level in subqueue should not move backwards", "newly-written task
  already present in outstanding tasks") never fire.

Liveness:
- `AllTasksAcked` / `EventuallyDrained`: every committed task is eventually
  acked, and the reader ends up holding nothing.
- `ReaderQuiesce`: the reader eventually stops reading and signalling
  itself — catches both a stuck reader and a busy re-read loop.
- `EventuallyCaughtUp` (only in `PriQueue_caughtup.cfg`): the reader
  reaches the max read level and persists that ack level, so the next load
  of the queue doesn't re-scan. NOT a property of the current code when
  writes can fail (findings.md #1) or when a read batch comes back entirely
  expired (findings.md #2); `run.sh` reproduces both.

## Mutation tests

Each `Mut*` constant re-introduces one bug; `run.sh` checks that TLC finds
the expected violation for each. All flags FALSE = current code (as of
e0a4751c).

The three flags that model the code *before* e0a4751c, one per hunk of that
commit:

| flag | code | model result |
| --- | --- | --- |
| `MutStaleGapLevels` | `setReadLevelAfterGap` applies levels a concurrent `signalNewTasks` has moved past | `ReadLevelMonotonic`, `AckLevelMonotonic`, `MemWindow` |
| `MutNoAckedTaskFilter` | `processTaskBatch` doesn't skip tasks at or below the ack level | `NoReDispatch`, `MemWindow` |
| `MutDbAckBackwards` | `db.updateAckLevelAndBacklogStats` doesn't keep the max | nothing on its own; with `MutStaleGapLevels`, `DbAckLevelMonotonic` |

Two things worth reading off that table. The third hunk is a backstop, so
the suite also checks that with the clamp in place the *persisted* ack
level stays monotonic even when the reader hands it a regressed one. And
the levels moving backwards comes specifically from the
`setReadLevelAfterGap` hunk: `MutNoAckedTaskFilter` on its own never
regresses the in-memory ack level (checked up to MaxLevel=4) — its damage
is dispatching an already-acked task a second time, which matches what
`TestProcessTaskBatch_IgnoresAlreadyAckedTasks` asserts.

`MutSignalAlwaysOnGap` covers the boundary of the fix (signalling when the
levels are not actually stale spins the pump — the case
`TestSetReadLevelAfterGap_NoReloadSignalWhenCaughtUp` guards), and
`MutNoSignalOnStale` covers the other side: dropping the self-signal from the
abort path is *not* caught, and findings.md #3 says why the signal is still
worth keeping. The remaining flags are seeded bugs that exercise the rest of the machinery:
`MutNoDedup`, `MutNoDirectAddLevelCheck`, `MutGcReadLevel`,
`MutNoGapSignal`, `MutNoReloadSignal`, `MutDrainedAckIgnoresOutstanding`,
plus `MutNoRoomCheck`, which is documented as *not* caught at this model
size.

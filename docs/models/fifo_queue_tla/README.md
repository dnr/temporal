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
  as a real-model check and, with `WriteErrors = TRUE`, to reproduce
  finding #1. With `Expirable = {2}`, it checks the resolved expiry finding #2.
- `PriQueue_expired.cfg` — all three ids are initially available and may
  expire; checks safety, delivery, quiescence and persisted catch-up without
  relying on a subsequent write.
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
The named-property diagnostics in `run.sh` require TLA+ tools v1.8.0 or
newer. `WORKERS=4` limits parallelism; `JAVA_TOOL_OPTIONS=-Xmx4g` limits
Java heap. `LOG_DIR=/absolute/path` saves each configuration and its full
TLC output there (default: ignored `tlc-results/`). The runner works with
both BSD and GNU `sed`.

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
- `EventuallyCaughtUp` (in `PriQueue_caughtup.cfg` and `PriQueue_expired.cfg`): the reader
  reaches the max read level and persists that ack level, so the next load
  of the queue doesn't re-scan. NOT a property of the current code when
  writes can fail (findings.md #1). It now holds when reads drop expired
  tasks (resolved finding #2); `run.sh` checks both outcomes.

## Mutation tests

Each `Mut*` constant re-introduces one bug; `run.sh` checks that TLC finds
the expected violation for each. With all flags FALSE, the model represents
e0a4751c plus the acknowledgement fix in temporalio/temporal#12595.

The three historical flags restore individual hunks of e0a4751c. The
historical checks use no expiry, where the new gap condition and the old
equality condition are equivalent:

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

## Expired-tail acknowledgement update

Source: `dnr/temporal` branch `fairmodels`, commit
`3173ca3473f79bae4bae6cf1b5fa08ba03909de7`. This local copy updates `RGap`
to match temporalio/temporal#12595: after rejecting stale scan levels,
advance and persist acknowledgement when `Outstanding = {}` rather than
requiring `ackLevel = readLevel`. `Outstanding` includes both `loaded` and
`ackedInMem`, matching all entries of the Go map.

`MutGapAckRequiresEqualLevels` restores the old condition. With expiry and
successful writes it must violate `EventuallyCaughtUp`; the same fixture
must pass with the flag disabled. Additional checks cover all-expired
backlogs, valid tasks followed by expired tails without further writes,
expiry concurrent with writes and read errors, and unfinished lower tasks
with out-of-order acknowledgements. Existing properties and fairness
assumptions are unchanged.

These are exhaustive checks of finite configurations, not an unbounded
proof of the Go implementation. Safety is checked through MaxLevel=4;
liveness is checked at MaxLevel=3. The model omits backlog counters and
uses unfair GC: it checks safe deletion and acknowledgement progress, not
eventual row deletion. Failed-write finding #1 remains an expected
liveness violation and is outside this change.

## Local verification — 2026-10-19

TLA+ tools 1.8.0 with Temurin Java 21 completed all 29 runner checks with
the expected outcomes. Safety checks passed through MaxLevel=4, including
expiry with concurrent writes/read errors and unfinished lower tasks.
The MaxLevel=3 liveness checks passed, including all-expired backlogs,
valid-then-expired tails, and persisted acknowledgement catch-up.

The original model reproduces the expired-tail `EventuallyCaughtUp`
violation. The updated model passes that fixture; restoring the old
condition with `MutGapAckRequiresEqualLevels` reproduces the violation.
The standard configuration explores the same 812,282 distinct states
before and after the change. The separate failed-write finding #1 remains
an expected violation.

Local evidence: `tlc-results/verification.json` records every configuration,
state count and log; `tlc-results/baseline/` preserves the unchanged-model
runs; `tlc-results/model-change.patch` is the patch against the pinned
source above. The evidence directory is ignored by Git.

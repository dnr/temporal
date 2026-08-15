# Findings from the PriQueue model

Issues surfaced while building the model. Each needs verification (e.g. a
targeted Go unit test) before being treated as a real bug.

## 1. A failed CreateTasks leaves the reader below the max read level (candidate)

Status: **model-confirmed against current code** (post-e0a4751c).
`PriQueue_caughtup.cfg` with `WriteErrors = TRUE` violates
`EventuallyCaughtUp`; `run.sh` reproduces it as a findings regression. A Go
unit test would still be a useful end-to-end confirmation.

`db.CreateTasks` advances the subqueue's `maxReadLevel` after the store call
returns, **whether or not the write succeeded** ("Do this even if the write
fails, we won't reuse the task ids"). But `priTaskWriter.appendTasks` returns
early on error, so `signalReaders` — and therefore `signalNewTasks` — is
never called. Nothing else triggers a read: the pump only wakes on
`SignalTaskLoading`, and its other callers are the ack path
(`loadedTasks == GetTasksReloadAt`), the read-retry timer, and
`signalNewTasks` itself.

Model trace (MaxLevel=3): a write of id 2..3 fails, `maxReadLevel` goes to 3,
and the reader stops for good at `readLevel = ackLevel = dbAckLevel = 1`.

Impact, in increasing order of importance:

- The persisted ack level never reaches the max read level, so
  `updateAckLevelAndBacklogStats` never takes its "reset
  ApproximateBacklogCount" branch, and the next load of this task queue
  re-scans from the stale ack level. That re-scan is exactly what
  `setReadLevelAfterGap`'s ack-level advance exists to avoid ("prevent
  excessive reads on the next load").
- If the failed write actually landed its rows (an incoming timeout), those
  rows sit unread until something else wakes the reader or the queue
  reloads. They carry no delivery guarantee — history re-submits with a
  fresh task id — so this is not a delivery bug, but it is a row that the
  backlog counters and `isDrained` disagree about.
- `isDrained` stays false (`readLevel < maxReadLevel`), which matters for
  the drain/unload path.

In practice the caller retries, and the next *successful* write signals the
reader, which then reads everything above `readLevel` including the orphan
rows. So this only bites when the last write before the queue goes idle
fails. Note the fair reader has an explicit fix for the same shape:
`unpinAckLevel(writeErr)` resets `atEnd` and triggers a read on write error
(8ca7b640 #5, and see `../fairness_queue_tla/findings.md`). The fifo reader
has no equivalent.

Fix shape: call `signalReaders`/`SignalTaskLoading` on the CreateTasks error
path too (the max read level moved, so there is something to catch up to).

## 2. A batch of expired tasks leaves the ack level stuck behind (candidate)

Status: **model-confirmed against current code**, independent of #1 (it
reproduces with `WriteErrors = FALSE`): `PriQueue_caughtup.cfg` with
`Expirable = {2}` violates `EventuallyCaughtUp`. `run.sh` reproduces it.

`processTaskBatch` drops tasks that are already expired when they are read
(`IsTaskExpired` → `slices.DeleteFunc`). Dropping raises `tr.readLevel` past
them but does no ack bookkeeping at all: they never enter `outstandingTasks`,
so no `completeTask`/`ackTaskLocked` ever runs for them. The only other way
the ack level advances is `setReadLevelAfterGap`, and that requires
`tr.ackLevel == tr.readLevel` — which the drop just made false.

So after a read batch that was entirely expired, with nothing else
outstanding, the reader ends in a state it cannot leave: `ackLevel` below
`readLevel == maxReadLevel`, `outstandingTasks` empty. Model trace: one
committed task (id 2) is read, found expired and dropped;
`readLevel = maxReadLevel = 2` while `ackLevel = dbAckLevel = 0`, and further
reads (which find nothing) cannot advance it.

Impact:

- **The expired rows are never deleted.** GC only deletes up to `ackLevel`
  (`CompleteTasksLessThan(ackLevel+1)`), and `finalGC` on unload uses the same
  level (and returns early when it is 0). The rows stay in persistence.
- Every subsequent load of the queue re-reads that range from the stale
  persisted ack level, finds the same expired rows, drops them again, and gets
  stuck at the same place. Wasted reads on every load, forever.
- `ApproximateBacklogCount` is never decremented for expired-dropped tasks
  (only `completeTask` passes a `countDelta`), and the "reset the count"
  branch in `updateAckLevelAndBacklogStats` only fires when
  `newAckLevel == maxReadLevel`, which is exactly what cannot happen here. So
  the backlog count over-reports for as long as the queue stays idle.
- `isDrained()` returns true throughout (it looks at `outstandingTasks` and
  `readLevel`, not the ack level), so nothing notices.

It self-heals as soon as any later task is written and completed: the ack
level then pops past the expired ids and GC catches up. So the exposure is a
queue whose backlog *tail* is all-expired and then goes idle — e.g. activity
tasks with a short schedule-to-start timeout after a worker outage.

The fair reader does not have this problem: 0b372d5e made expired tasks
become pre-acked (nil) entries, which advance the ack level and get GC'd.

Fix shapes: treat expired-on-read tasks like acked ones (insert and
immediately ack, so the existing machinery advances the level and the count),
or relax `setReadLevelAfterGap` to advance the ack level whenever
`outstandingTasks` is empty rather than only when `ackLevel == readLevel`.

## 3. Spec clarifications (not bugs)

Things the model forced us to make precise:

- **The db-side ack level clamp is a backstop, not a fix.** With the
  reader-side hunks of e0a4751c in place, the reader never hands
  `updateAckLevelAndBacklogStats` a lower ack level, so the softassert
  ("ack level in subqueue should not move backwards") should never fire and
  can be treated as a genuine alarm. `run.sh` checks both directions:
  reverting only the db clamp changes nothing observable, and reverting the
  reader fix while keeping the clamp keeps the *persisted* level monotonic
  while the in-memory one regresses.

- **`setReadLevelAfterGap`'s `SignalTaskLoading` on the stale path is
  defensive, not load-bearing.** `MutNoSignalOnStale` keeps the staleness
  check but drops the self-signal, and the model finds no violation (safety +
  liveness at MaxLevel=3 and 4, including `EventuallyCaughtUp`, plus variants
  with a single full-range scan and with more room for direct adds).

  The reason: the abort path is only reachable when something raised
  `tr.readLevel` while the read was in flight, and the only thing that can is
  `signalNewTasks`' direct-add path. That path sets
  `tr.readLevel = resp.maxReadLevelAfter`, which *is* the subqueue's new
  `maxReadLevel`, and it has already put every task of that write into
  `outstandingTasks` and dispatched it. So at the abort there is provably
  nothing to read: the reader is at the end of the queue, and the tasks it
  skipped reading are in memory. The remaining wakeups (`loadedTasks ==
  GetTasksReloadAt` when they are acked, and the drained-ack path) do the rest.
  And if `canAddDirect` was false, `signalNewTasks` called
  `SignalTaskLoading` itself, so a pending signal already survives the abort.

  Keep the signal anyway: it costs one extra read only when the race actually
  happens, and without it correctness depends on the coupling above — that a
  direct add always leaves the reader exactly at the end. Anything that
  weakened that (adding only part of a write, or advancing `readLevel` short
  of `maxReadLevelAfter`) would turn the missing signal into a stalled reader,
  and it would not be caught by a test of `setReadLevelAfterGap` alone.
  `MutNoSignalOnStale` is kept in `run.sh` as an expected-pass entry so this
  stays documented rather than re-derived.

- **Rows from a timed-out write carry no delivery guarantee.** A write that
  returns an error may still have applied. The caller re-submits with a new
  task id, so the landed row is an unguaranteed duplicate: the ack level may
  legitimately pass it undispatched and GC may then delete it. The model's
  delivery and GC-safety properties are therefore scoped to "committed"
  tasks (initial backlog + writes whose RPC returned success).

  The model does assume a timed-out write's rows land *before* `CreateTasks`
  returns (the db step is atomic). A store that applies the write later,
  after `maxReadLevel` has already moved and the reader has scanned past it,
  is out of scope — under the contract above it is still not a lost task,
  but nothing in this model rules that timing out.

- **`if tr.loadedTasks == config.GetTasksReloadAt()` is safe here.**
  The `TODO(pri): is this safe?` next to that exact-equality check is about
  a lost wakeup: the pump drops a signal whenever more than `ReloadAt` tasks
  are loaded, so if the count could cross the threshold without hitting it
  exactly, the reader would sleep forever. `loadedTasks` only ever
  *decreases* one at a time (`completeTask`), so every crossing hits the
  threshold, and the model (which checks the crossing under all
  interleavings of reads, writes and acks) finds no lost wakeup — while
  `MutNoReloadSignal` shows the signal is load-bearing: removing it violates
  `AllTasksAcked`. That argument does depend on the reload threshold not
  changing under the reader; a dynamic-config change to `GetTasksReloadAt`
  between the ack and the pump's re-check is not modeled.

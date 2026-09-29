# Findings from the FairQueue model

Candidate issues in the real code surfaced while building the model. Each
needs verification (e.g. a targeted Go unit test) before being treated as a
real bug.

## 1. Busy re-read loop when lowest loaded task is slow to ack (candidate)

Status: **model-confirmed against current code** (with the evicted-ack
cache modeled): the `FairQueue_churn.cfg` configuration — one loaded task
that never completes, one acked task above it — violates `ReaderQuiesce`
(the reader never stops issuing reads). Reproduced by `run.sh` as a
findings-regression entry. A Go unit test would still be a useful
end-to-end confirmation.

Scenario (single subqueue, current code as of 7f976f7e90):

1. Tasks 1 and 2 are in the DB. Reader loads both; readLevel=2.
2. Task 2 completes (acked in memory as a nil entry); task 1 is still
   outstanding (e.g. slow/absent poller for its key). ackLevel stays 0, so
   GC can't delete task 2.
3. Reader reads from level 3, gets nothing => mergeReadToEnd with 0 tasks.
4. In `mergeTasksLocked`, `merged` = loaded tasks only = {task 1}, so
   `highestLevel` = 1 and **readLevel is lowered 2 -> 1**, even though we
   just confirmed (2, inf) is empty.
5. The ack for task 2 is now above readLevel, so it is **evicted** (into
   evictedAcks), which sets `evictedAnyTasks`, which forces **atEnd=false**
   (overriding the mergeReadToEnd we just did).
6. shouldReadMore: loaded=1 <= reload-at, not atEnd => read from 2 again,
   re-reads task 2 from the DB. The evicted-ack cache turns it back into a
   nil entry (without the cache, it would be re-dispatched). Read got a full
   batch => mergeReadMiddle => atEnd stays false. readLevel back to 2.
7. Go to 3. Tight loop: two successful DB reads per iteration, no backoff
   (backoff only applies to read *errors*), until task 1 is finally acked.

Impact: sustained redundant DB read load per partition whenever the lowest
loaded task is unacked for a while and acked-but-not-GCed tasks sit above it
at the tail of the queue. No correctness violation (the cache prevents
re-dispatch; without it this is at-least-once re-dispatch, which is
allowed).

Possible fix shape: don't lower readLevel below its current value on a read
merge (`tr.readLevel = max(tr.readLevel, highestLevel)` on read modes), or
exclude nil entries above `highestLevel` from eviction when the merge saw no
new tasks. Needs thought about interaction with the write path, where
lowering readLevel is intentional.

## 3. "Fair reader stuck" softassert is reachable via expired writes (confirmed in model)

Status: **model-confirmed against current code** (TLC finds it in seconds;
`run.sh` reproduces it as the "findings regression" entry). Needs a Go unit
test to confirm end-to-end, but the step sequence maps 1:1 to real code
paths.

The defensive check in `mergeTasks` (added in f534e74e, kept "in case other
bugs produce the same state") is not dead code: current code can reach the
"stuck" state it guards against, and relies on its repair read. Sequence:

1. Reader is at the end: e.g. task 3 loaded, readLevel=3, atEnd=true.
2. Writer pins the ack level and writes task 1, where task 1 is *already
   expired* when the write lands (e.g. a very short schedule-to-start
   timeout plus a slow write).
3. While the write is in flight, task 3 completes: it becomes a nil entry;
   the ack level can't advance (pinned by the write). loadedTasks=0.
4. The write succeeds; `wroteNewTasks` merges task 1: it is expired, so it
   becomes a nil entry (0b372d5e behavior); `highestLevel`=1, so
   **readLevel is lowered 3 -> 1**; the nil for task 3 (above the new
   readLevel) is **evicted**, which sets `evictedAnyTasks` and forces
   **atEnd=false**.
5. Post-merge state: loadedTasks=0, atEnd=false, no read pending, no
   backoff timer -> the softassert fires (log + FairReaderStuckDetected
   metric), and its `maybeReadTasksLocked` repair is the only thing that
   un-sticks the reader.

Notes:

- The f534e74e fix covered the merged-set-empty case; here the merged set
  is *non-empty but all-expired*, a path that fix does not cover.
- Impact: alarm/metric noise on a "should never happen" signal, plus a
  dependency on a "defensive" backstop for liveness. If the softassert is
  ever demoted to a hard assert or the backstop removed, this becomes a
  stuck reader.
- Fix shapes: treat a merge whose kept set contains no live tasks like the
  empty-merge case for readLevel purposes (don't lower readLevel), or
  trigger the read deliberately (not via softassert) when expired incoming
  tasks zero the loaded set.
- Related: the readLevel lowering in step 4 is the same mechanism as
  finding #1.

## 4. Stale owner's GC deletes the new owner's tasks (confirmed; found by unit test)

Status: **confirmed** (found first by a unit test; reproduced by
`FairQueueOwners.tla`, TLC finds it in ~2s at depth 11). Not reachable in
`FairQueue.tla`, which has a single owner.

Sequence (TLC's trace, levels as integers):

1. Owner A owns range 1; persisted ack level 0; task 3 in the db.
2. Owner B takes over (range 2), starting from persisted ack level 0.
3. B writes task 1 (pass at its base, below A's tasks' passes). Committed.
4. A, not yet aware it lost ownership, reads and acks task 3: A's
   in-memory ack level is 3.
5. A GCs `<= 3`. `CompleteTasksLessThan` is not fenced by range id, so it
   deletes B's undispatched task 1. `GCOnlyAcked` violated.

The pri (FIFO) queues are immune because task ids increase across range
ids; fair passes don't.

### Candidate fixes checked (`GcMode`, `TakeoverCAS`; see run_owners.sh)

- `"verified"` (check range id, then GC): **broken**, TOCTOU: B takes over
  between A's check and A's delete.
- `"persisted"` (GC only up to the ack level this owner persisted with a
  successful range-fenced LWT): **broken** by the takeover's
  read-then-LWT window. `takeOverTaskQueueLocked` reads the metadata, then
  does an LWT conditional *only on the range id* that writes the metadata
  back as read. A's `SyncState` between B's read and B's LWT doesn't change
  the range id, so both LWTs succeed: A believes it persisted ack level 2
  and GCs up to it, while B's LWT overwrote it with 0 and B writes at 1.
- `"persisted"` + `TakeoverCAS` (the takeover LWT also fails if the
  persisted ack level changed since its read; equivalently, the takeover
  bumps only the range id and uses the ack level read after that): **passes**
  safety (MaxLevel=3, 6.3M states; and MaxLevel=2 with read/write timeouts
  and expiry) and liveness (MaxLevel=2).
- `"fenced"` (the delete itself is conditional on the range id): **passes**
  safety (MaxLevel=3, 8.1M states; MaxLevel=2 with read/write timeouts and
  expiry) and liveness (MaxLevel=2).

No other violations were found with either passing fix.

Notes:

- Whether a range-conditional range delete is expressible is store-
  dependent (easy in a SQL transaction; for Cassandra it'd need a
  conditional batch on the partition). The persisted+CAS variant needs only
  row LWTs, at the cost of GC lagging the in-memory ack level by up to one
  ack-persist interval.
- Model caveat: integer levels let a later owner's task land anywhere above
  its pinned ack level; in reality, for equal passes the later owner's
  (higher) ids sort after. The bug only needs a pass strictly below the
  stale owner's in-memory ack pass, which is real.

## 2. Spec clarifications surfaced by M4 (not bugs)

Two things the model checker forced us to make precise; both match
fairness.md's intent but are worth stating:

- **Tasks from timed-out writes carry no delivery guarantee.** A write that
  returns an error may still have applied (incoming timeout). The caller
  (history) re-submits, so the landed rows are unguaranteed duplicates: the
  ack level may legitimately pass them un-dispatched (the pin is released on
  write error), and GC may then delete them unacked. If such a row is ever
  re-read (e.g. after a readLevel drop) it is dispatched normally. The
  model's delivery/GC-safety properties are therefore scoped to "committed"
  tasks: the initial backlog plus writes whose RPC returned success.

- **The atEnd=false reset in unpinAckLevel on write error is defense in
  depth, not required for committed-task correctness.** The model passes
  with it removed (MutNoAtEndResetOnWriteError): the only tasks it can
  strand are rows landed by timed-out writes, which have no delivery
  guarantee. Keep the reset (it improves best-effort delivery of such
  orphans and protects against caller retry loss), but don't rely on it.

- **The modern "fair reader stuck" detector subsumes some older fixes on
  its trigger path.** Re-introducing the 26d9a561 or 8ca7b640-#5 bugs now
  trips the detector (NoStuck) before any liveness violation manifests --
  but only because a later write-merge happens to run the check. If no
  further writes occur, the detector never fires and the historical
  liveness bugs would still bite; run.sh checks both variants (with the
  detector as invariant, and without it expecting the liveness violation).

- **The defensive "fair reader stuck" condition requires backoffTimer==nil.**
  With read timeouts in the model, a write can merge into an empty buffer
  with atEnd=false while a read-retry backoff timer is armed; that state is
  not stuck (the timer fires and triggers the read). The Go check already
  excludes it -- an earlier version of the model that omitted the timer
  condition produced a counterexample within seconds, confirming the timer
  condition in the Go code is load-bearing.

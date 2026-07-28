# Findings from the FairQueue model

Candidate issues in the real code surfaced while building the model. Each
needs verification (e.g. a targeted Go unit test) before being treated as a
real bug.

## 1. Busy re-read loop when lowest loaded task is slow to ack (candidate)

Status: **unverified** — found while analyzing an M1 counterexample; revisit
at M6 (evicted-ack cache) with a dedicated property and/or a Go test.

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

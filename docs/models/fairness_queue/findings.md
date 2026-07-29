# Findings

## 1. The "fair reader stuck" softassert is reachable (via expired tasks)

**Status: model-confirmed against the current code structure (M5).**

`fairTaskReader.mergeTasks` (fair_task_reader.go:397) treats the state
`mode == mergeWrite && !atEnd && loadedTasks == 0 && !readPending &&
backoffTimer == nil` as a should-never-happen bug: it fires
`softassert.Fail("fair reader stuck")`, bumps `FairReaderStuckDetected`, and
does a repair read described as defensive.

The model reaches this state with no bug present, in a few steps
(FairQueue_0_0.txt trace, 0.3s to find):

1. Writer writes task at level 3; reader merges it directly (atEnd), acker
   completes it. The ack level stays pinned at 0 because the writer has
   already pinned for its next write. Buffer now holds only the ack for 3.
2. Writer writes level 2 (below readLevel 3; legal, it's above the pinned ack
   level 0), and by merge time the task is expired.
3. mergeTasksLocked: merged = {2} (3 is an ack, not part of the merged set),
   so readLevel regresses to 2 — correct per the merge rules — and the ack at
   3 is evicted as "above the new readLevel", clearing atEnd. The expired
   task 2 becomes a pre-acked entry, not a loaded task.
4. Result: loadedTasks == 0, !atEnd, no read pending, no backoff timer →
   softassert fires; without the repair read the reader would be stuck (the
   evicted ack at 3 means the db still holds task 3, and nothing else would
   trigger a read).

Consequences:

- The softassert and metric will fire spuriously in production whenever a
  write below readLevel expires before merging while the buffer holds only
  acks. (Requires an already-expired task to be written / merged, e.g. a very
  short schedule-to-start timeout combined with write latency or a respool.)
- The repair read is load-bearing for correctness in this scenario, not
  defensive: the model asserts the stuck state is unreachable *except* via
  expired tasks, and that holds across the search portfolio.

Suggested code changes (either or both):

- Downgrade the softassert to a debug log / expected-case metric when the
  merge involved expired tasks, or
- Trigger the read directly when a write-merge ends with loadedTasks == 0 &&
  !atEnd instead of treating it as an anomaly.

A Go unit test reproducing the sequence above would confirm on the real code.

# Mutation testing log

Each mutation reintroduces a historical bug (or breaks newly-added logic) to
verify the model can detect it. "Caught by" lists what actually failed.
Detection strategy portfolio: `--sch-random`, `--sch-pct 10`,
`--sch-feedbackpct 20`, 20-30k schedules each.

## M1

| id  | mutation | target | result |
|-----|----------|--------|--------|
| M1a | no maybeRead() after completing a task | harness sanity | caught: liveness, 0.15s, 33% of schedules |
| M1b | don't clear atEnd when a written task doesn't fit | harness sanity | caught: liveness, <1s |

## M2

| id  | mutation | target | result |
|-----|----------|--------|--------|
| M2a | merge writes immediately instead of buffering during pending read | 12e7c43a | caught: liveness, 0.2s, 25% of schedules — but ONLY after modeling in-flight read responses (see below) |
| M2b | keep acks above new readLevel instead of evicting | 8ca7b640 #4 | caught: invariant `outstanding <= readLevel`, 100% of schedules; with that invariant disabled: `ackLevel <= readLevel`; with both disabled: liveness (lost task). Three independent detection layers. |
| M2c | collapse readLevel to ackLevel when merged set is empty | f534e74e | caught: "fair reader stuck" assert — but only by `--sch-feedbackpct 20` (12.6s, 0.06% of schedules). Random and plain feedback missed it at 20-30k schedules. |

## M3

| id  | mutation | target | result |
|-----|----------|--------|--------|
| M3a | disable ack level pinning | fairness.md "ack level movement while a write is in flight" | caught: 100% of schedules. Liveness (write filtered below runaway ackLevel) on most seeds; NoLostTask "task deleted before completion" (GC deletes an in-flight write) on 3/8 seeds. |

### Modeling lessons

- **In-flight read responses are load-bearing.** P's `send` enqueues atomically
  into a FIFO queue, so a DB read response would always beat any eWroteTasks
  triggered by a later DB op, hiding the 12e7c43a race entirely. The Database
  machine therefore holds computed read results in a self-send hop
  (eReadServed) before delivering, modeling Go's lock-released-during-IO
  window. Mutation M2a is invisible without this.
- **Search strategy matters.** M2c is only found by feedbackpct. Every clean
  claim should run the full portfolio.
- 8ca7b640 #1 (ignore read tasks <= ackLevel): analysis says this filter is
  unreachable in the current code structure (single sequential read loop;
  response tasks are always > readLevel-at-send >= any reachable ackLevel, as
  ackLevel <= readLevel and readLevel is frozen while a read is in flight). It
  appears to be defensive. Not usable as a mutation; revisit in M7 and when
  M4-M6 change reachability.
- 8ca7b640 #3 (ignore re-read tasks that are already-acked in outstanding):
  reverting it re-dispatches an acked task; the spec permits duplicate
  dispatch (at-least-once), so no violation is expected. Its real-world
  consequence was accounting corruption not modeled here.

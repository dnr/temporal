
We have a component with some very tricky logic, that we've found some tricky
bugs in before. I would like to increase my confidence in the component by
writing a TLA+ model for it and checking various properties (primarily
liveness).

The code is in `./service/matching/fair_task_{reader,writer}.go` from the base
of this repo, and related files. It implements a fair queue processor on top of
a database. More details are available in `fairness.md` in that directory as
well.

The basic ideas that I think the model should include are:

- The system works on top of a database. The database should be modeled as
  a concurrent process connected over the network.
  - Database calls may succeed.
  - Database calls may time out, either on the outgoing side (the operation was
    not performed) or the incoming side (the operation was performed). In both
    cases the queue logic is not aware of the success of the operation.
  - In this model, we don't need to include database calls that explicitly fail.
- Tasks are written by the task writer (another concurrent process). Note the
  "pinning ack level" logic in the code that does synchronize the writer and
   reader to some degree.
- The primary key includes a `<pass, id>` pair to have the database sort tasks
  for us in a fair order based on a stride scheduling algorithm.
- Tasks are read by the task reader (another concurrent process).
- The task reader attempts to maintain a bounded head of the queue in memory at
  all times.
- Tasks that are either read or written successfully are merged into the head
  according to the logic in mergeTasksLocked.
- A task in memory may be "acked", marking it complete. The "acker" should be
  considered another concurrent process, that will eventually ack all the tasks
  loaded in memory.
- The ack level may move up to the lowest unacked task.
- Tasks below the ack level may be GCed (deleted). This should be modeled as
  another current process.
- The "evicted ack cache" (in a later milestone).
- Pay attention to the logic that retries failed reads after a timeout. There
  was a bug there that I think should be caught by a detailed-enough model
  (commit 26d9a561).
- Pay attention to the comment about not resetting the read level after a
  merge-after-write results in zero loaded tasks. There was a bug there that I
  think should be caught by a model (commit f534e74e).
- Pay attention to expired tasks. There was a bug there too (commit 0b372d5e).
- Older commits worth looking at to see what kind of bugs we're trying to catch:
  - ad717eae: tricky race that involves matcher, maybe outside the scope of what
    we can do with a single model
  - 8ca7b640: five(!) early bugs fixed (before production thankfully)
  - 12e7c43a: another early bug, fixed writer pinning+merge logic

Details in the code to explicitly **not** include:

- Database calls that explicitly fail: this would primarily be due to
  "conflict", i.e. task queue partition ownership loss. We'll create a separate
  model for the ownership/write fencing protocol if necessary.
- Subqueues: assume only one writer and one reader.

For other questions about which details of the code should be reflected in the
model or ignored, please ask.

Properties to check:

- If a task is written it will eventually be acked. That implies these
  properties, which could be worth checking separately:
  - The reader never gets "stuck" (in a state where there are tasks in the
    database but the reader will never read them).
  - We never GC a task before acking it (this would be even worse.. a restart
    could fix the previous situation but not this).

In general I don't want the model to verify anything about "fairness", even
though that's the purpose of the system. Our notion of fairness is statistical
and not something that TLA+ is suited to handle. We're looking for correctness
and liveness properties here.

Please use PlusCal, it'll be much easier to read and maintain. `java` is
available on the PATH, and `tla2tools.jar` is available in
`./docs/models/tla2tools.jar`.

It may be too complicated to build the full desired model in one shot. First,
please list a series of milestones of increasing model complexity. After that
we'll implement them one by one.

Milestones:

_fill this in_



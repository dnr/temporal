
# Fair Task Reader operation

## Definitions

**Task ID**: sequential int64 assigned to tasks. Two tasks never have the same id.

**Pass**: int64 used to "spread out" tasks.

**Fair level**: tuple of <pass, id>. Fair levels are ordered lexicographically and ordered that way
by persistence.

Tasks are _written_ out of order and we rely on persistence to order them by level (most
importantly by pass).

Considering the set of tasks that were recently dispatched plus in the backlog, ordered by
level:

```
  <-------------A-----R-----------------------M------------->
```

A is the ack level (persisted): We have dispatched all tasks < A. Therefore, we never need to
read below A, and therefore we must never write below A. A can only move forwards within the
lifetime of a partition. However, we don't write A to persistence on every dispatch, we only
update it once in a while. So one partition owner may move A forwards in memory, then crash,
then another owner may load the partition. In that case it will see an old version of A. This
is allowed to cause repeated dispatch of some tasks.

M is the maximum task level that has ever been written, inclusive (persisted). Whenever we
write tasks we also write M, so M should always be accurate. If we write below M we don't need
to update M; if we write above M then we update M to the new level.

R is the level that we have read up to, exclusive (not persisted). I.e. we've read range [A, R)
and we have them in memory waiting to dispatch. We may write new tasks either above or below R.

## Operation

### Reading

When we load the task queue metadata, we get A and M. All non-dispatched backlog tasks should
be in that range (see Fencing below). Initialize R to A.

When we read: Do a read for tasks with R <= level, limit Bt (batch target size). Note that
if R > M, then the range must be empty and we can skip the read.

Suppose we get n tasks and the last one is m (n <= Bt). For now, suppose that we're
not concurrently writing tasks.

Case n == Bt: We've read one full batch. Set R to m+1 so that we can start the next read there.

Case 0 < n < Bt: We read a partial batch and got to the end. Set R to m+1. We won't do another
read next time since R > M.

Case n == 0: We didn't find any tasks at all. Set R to M+1.


### Writing

Now add in concurrent writing:

When we write tasks, we allocate ids for them, and choose passes based on their fairness keys,
which together make the levels of the new tasks. All new task levels must be >= A! (I.e. their
pass must be >= A.pass. We know their id is > A.id because ids are assigned sequentially.)

Let [Wmin, Wmax] be the range of task levels we just wrote.

If Wmin > R, then they'll show up when we read above R, so we don't have to do anything to R.

If Wmin < R, things get interesting: We're supposed to have everything below R in memory so we
don't need to read it again. But if we write below R, that breaks that assumption.

The simplest solution (plan 1) is to set R to Wmin so we'll read from there next time, so we
won't miss the new tasks. And we should drop any tasks we have in-memory that are above the new
R so the new ones get treated fairly relative to and older tasks that were in that range. If we
have more room in memory at that point, we can do a read immediately.


### Bypass optimization

Potentially dropping a bunch of tasks and rereading them on every write is inefficient. Also if
we don't have too much in memory, we should not have to re-read the tasks we just wrote (even
if we don't drop anything). Instead of dropping and re-reading, we can simulate what would
happen with the add/drop/reread and add tasks to the buffer directly (plan 2):

Take the tasks in the buffer plus the tasks that were just written and sort them by level. Take
the first Bt of them (or all of them if < Bt). Set the buffer to that set. Set R to the maximum
level in that set. Discard the rest from memory.

Note that works whether Wmin is above or below R.


### GC

At any time we can issue a delete for tasks < A. We'll do this periodically based on time or
when the number of acked tasks passes a threshold.


### Tombstone scanning

In what situations will we scan tombstones, and how many?

We always delete < A, always read > A, and A always moves forward, so in normal operation we
should never scan tombstones. We could if A moves backwards: this happens if we crash after
dispatching some tasks, before persisting the new A.

Note that we persist A when we write tasks. So we're only likely to move backwards by a
significant amount if tasks are being just dispatched without being written.

Suppose we persist A every Tp seconds, and issue a delete every Tg seconds or Ng tasks.
Considering only time: we may rescan up to Tp seconds worth of tasks, and find about Tp/Tg
tombstones in it. So we should ensure that Tg is not much smaller than Tp.

(In practice we have Tp as 60 seconds and Tg used to be 1 second, but was changed to 15 for the
priority task reader, so <= ~4 tombstones, or up to ~60 using 1 second.)

We also have to consider Ng. If the dispatch rate is R, then we'll dispatch up to Tp * R tasks.
We'd issue Tp * R / Ng deletes based on count. So we should ensure Ng is not much smaller than
R, or the maximum practical R.

(In practice, let's say the maximum R for a partition is 1000 t/s, and Tp is 60s, so we could
re-dispatch 60,000 tasks. With Ng at 100, we'd scan up to ~600 tombstones. We could increase Ng
to 1000 to reduce that to ~60.)

The Cassandra maximum tombstone limit before a read fails is 100,000, though performance may be
affected at lower levels.


### Fencing

We've been assuming a single partition owner operating without interference. The owner may
change over time, so we have to consider what happens if two owners attempt to operate
simultaneously.

We also have to consider the possibility of delayed writes: if a write returns "timed out",
then we (or another owner) may observe it to take effect at some point in the future (unless
it's an LWT that we can prevent from succeeding).

For matching, if writing tasks returns "timed out", an error will be returned to the caller and
the caller will retry, or maybe give up. In any case, the caller won't assume the task has been
written. So this can lead to duplicate task dispatch, but that's not a problem.

Basically, the problematic case is the following potential sequence:

1. History calls AddTask on the old owner
2. The old owner does a write
3. Ownership changes, but the old owner doesn't realize yet
4. The new owner starts up, reads the metadata, and reads the range [A, M]
5. The new owner updates its R to the end of what it just read
6. The write now takes effect within that range
7. The old owner returns success to history, and history assumes the task has been queued
8. The new owner never re-reads that range because R has moved past it
9. The task is now effectively lost

The current solution for avoiding this is that the write in 2 is an LWT that's conditional on
the old range id. In 4, the new owner does an LWT that updates the range id and gets the
current metadata at that time. That guarantees that either the write took effect before the new
owner's update, or it will fail. If it took effect before, then the new owner will definitely
see the task when it does a read.

TODO: is that enough to prevent all problems in the fairness schema?

TODO: are there any other ways to do it?
we could just put a uuid "owner" instead of range id
and then just allocate task ids densely

can we do it without lwt? that probably means time-based lease, otherwise there's no way to
force the write to fail




## Counting

How are pass numbers assigned?

TODO



















































# CRAP

Let the maximum buffer size be Bmax, the number of tasks currently in the buffer Bcur, and the
number being written Wcount.
Let Bfree = Bmax - Bcur (must be >= 0).

Let Wcutoff be the level of the first task that doesn't fit in the buffer, or Wmax+1 if all
tasks fit.

Consider these cases:

```
P   <-------------A---------------------R--------------------------->
                                           Wmin----|----Wmax

Q   <-------------A---------------------R--------------------------->
                                  Wmin----|----Wmax

R   <-------------A---------------------R--------------------------->
                             Wmin----|----Wmax

S   <-------------A---------------------R--------------------------->
                     Wmin----|----Wmax
```

The `|` between Wmin and Wmax is Wcutoff, and is drawn between Wmin and Wmax, but could be
equal to Wmin (in P, R, and S) or just above Wmax (in P, Q, and S).

P: We can put the tasks that fit into the buffer, then set R to Wcutoff. This is okay because
before we've read up to R, and now have effective "read" up to Wcutoff.

Q: 

R: 

S: We can put the tasks that fit into the buffer, then set R to Wcutoff. We need to move R
backwards 




---

Basically, we put all the tasks that can fit into the buffer, then set R to Wcutoff. FIXME:
max/min?



If Wmin > R:

    Do nothing as before. FIXME: wait, shouldn't we squeeze them in mem if we can?

If Wmin < R:

    If Wcount <= Bfree: Add all new tasks to the buffer and set R = max(R, Wmax).

    If Wcount > Bfree: Add the first Bfree tasks to the buffer. Let Wcutoff be the level of the
    first task not added to the buffer. Set R = Wcutoff.








#!/usr/bin/env bash
# Check the PriQueue model: translate, run TLC on the real (fixed) model,
# then run each mutation and check that TLC catches it. A mutation
# re-introduces a bug; if TLC does NOT catch it, the model is too weak.
set -uo pipefail
cd "$(dirname "$0")"

JAR=${JAR:-$HOME/tla2tools.jar}
TLC() { java -XX:+UseParallelGC -cp "$JAR" tlc2.TLC -workers auto "$@"; }

echo "=== translating ==="
java -cp "$JAR" pcal.trans -nocfg PriQueue.tla || exit 1

fail=0

# check <desc> <pass|fail> <base-cfg> <expected-output-regex> [sed-expr]...
# Each sed-expr edits a copy of the base cfg (normally to flip a Mut* flag).
check() {
  local desc=$1 expect=$2 base=$3 pattern=$4; shift 4
  local cfg=mut_tmp.cfg e
  cp "$base" "$cfg"
  for e in "$@"; do sed -i "$e" "$cfg"; done
  if [[ $# -gt 0 ]] && cmp -s "$cfg" "$base"; then
    echo "FAIL: $desc: cfg edit changed nothing (flag renamed?)"; fail=1; return
  fi
  local out status
  out=$(TLC -config "$cfg" PriQueue.tla 2>&1)
  status=$?
  if [[ $expect == pass && $status -ne 0 ]]; then
    echo "FAIL: $desc: expected pass, TLC found an error:"
    echo "$out" | grep -E "^Error:" | head -3
    fail=1
  elif [[ $expect == fail && $status -eq 0 ]]; then
    echo "FAIL: $desc: expected TLC to find a violation, but it passed"
    fail=1
  elif ! echo "$out" | grep -qE "$pattern"; then
    echo "FAIL: $desc: output did not match /$pattern/:"
    echo "$out" | grep -E "^Error:" | head -3
    fail=1
  else
    echo "ok: $desc"
  fi
  rm -f "$cfg"
}

on()   { echo "s/^  $1 = FALSE$/  $1 = TRUE/"; }
off()  { echo "s/^  $1 = TRUE$/  $1 = FALSE/"; }
drop() { echo "/^  $1$/d"; }

echo "=== real model (expect pass) ==="
check "real model, safety + liveness" \
  pass PriQueue.cfg "No error has been found"
check "real model, no write errors: reader catches up to the max read level" \
  pass PriQueue_caughtup.cfg "No error has been found"
check "real model, with expiring tasks" \
  pass PriQueue.cfg "No error has been found" 's/Expirable = {}/Expirable = {2}/'
check "real model, safety only, MaxLevel=4" \
  pass PriQueue_safety4.cfg "No error has been found"

echo "=== e0a4751c: the bugs this model was built for (expect violations) ==="
# The code before e0a4751c: all three hunks reverted. The damage is that the
# read and ack levels move backwards (checked on its own in the levels-only
# runs below); here a bookkeeping invariant trips first.
check "pre-fix code (all three hunks reverted)" \
  fail PriQueue.cfg "Invariant (MemWindow|AckBelowRead|NoReDispatch) is violated" \
  "$(on MutStaleGapLevels)" "$(on MutNoAckedTaskFilter)" "$(on MutDbAckBackwards)"

# Hunk 3 (setReadLevelAfterGap): getTaskBatch concluded "read to the end,
# found nothing" from levels that signalNewTasks has since moved past;
# applying them drags readLevel -- and, when ackLevel == readLevel, ackLevel --
# backwards over tasks that were direct-added in the meantime.
check "MutStaleGapLevels: in-memory entries end up above the read level" \
  fail PriQueue.cfg "Invariant MemWindow is violated" "$(on MutStaleGapLevels)"
check "MutStaleGapLevels: read level moves backwards" \
  fail PriQueue_levels.cfg "Action property ReadLevelMonotonic is violated" \
  "$(on MutStaleGapLevels)"
check "MutStaleGapLevels: ack level moves backwards" \
  fail PriQueue_levels.cfg "Action property AckLevelMonotonic is violated" \
  "$(on MutStaleGapLevels)" "$(drop ReadLevelMonotonic)" "$(drop NoDbAckBackwards)"

# Hunk 2 (processTaskBatch): an in-flight read returns rows that
# signalNewTasks already direct-added and that were acked while the read was in
# flight. outstandingTasks has dropped them (the ack level passed them), so the
# dedup check cannot see them and they are dispatched a second time.
check "MutNoAckedTaskFilter: already-acked tasks re-enter the outstanding map" \
  fail PriQueue.cfg "Invariant (MemWindow|NoReDispatch) is violated" \
  "$(on MutNoAckedTaskFilter)"
check "MutNoAckedTaskFilter: task dispatched twice" \
  fail PriQueue_levels.cfg "Invariant NoReDispatch is violated" \
  "$(on MutNoAckedTaskFilter)"

# Hunk 1 (db.updateAckLevelAndBacklogStats): not a bug on its own -- it is the
# backstop that keeps the *persisted* ack level monotonic when the reader hands
# it a lower one.
check "MutDbAckBackwards alone: no observable effect (backstop only)" \
  pass PriQueue.cfg "No error has been found" "$(on MutDbAckBackwards)"
check "MutStaleGapLevels with the clamp: persisted ack level still monotonic" \
  pass PriQueue_levels.cfg "No error has been found" \
  "$(on MutStaleGapLevels)" "$(drop ReadLevelMonotonic)" "$(drop AckLevelMonotonic)" \
  "$(drop NoReDispatch)" "$(drop NoDbAckBackwards)"
check "MutStaleGapLevels + MutDbAckBackwards: persisted ack level regresses" \
  fail PriQueue_levels.cfg "Action property DbAckLevelMonotonic is violated" \
  "$(on MutStaleGapLevels)" "$(on MutDbAckBackwards)" \
  "$(drop ReadLevelMonotonic)" "$(drop AckLevelMonotonic)" "$(drop NoReDispatch)" \
  "$(drop NoDbAckBackwards)"

echo "=== boundary of the fix (expect violation) ==="
# The fix may only signal itself when the levels it got are actually stale.
# Signalling whenever a gap scan ends at the current read level spins the pump:
# empty batch -> signal -> empty batch -> ... This is the case guarded by
# TestSetReadLevelAfterGap_NoReloadSignalWhenCaughtUp.
check "MutSignalAlwaysOnGap: the pump never quiesces" \
  fail PriQueue.cfg "Temporal property ReaderQuiesce was violated" \
  "$(on MutSignalAlwaysOnGap)"

# Dropping only the self-signal from that abort path is NOT caught: after a
# direct add the reader is provably at the end of the queue with every written
# task already in memory, so there is nothing to re-read. See findings.md #3 --
# the signal is cheap insurance for a coupling that is easy to break.
check "MutNoSignalOnStale: not load-bearing (findings.md #3)" \
  pass PriQueue_caughtup.cfg "No error has been found" "$(on MutNoSignalOnStale)"

echo "=== seeded mutations (expect violations) ==="
check "MutNoDedup: re-read of a loaded task is dispatched again" \
  fail PriQueue.cfg "Invariant NoReDispatch is violated" "$(on MutNoDedup)"
check "MutNoDirectAddLevelCheck: read level jumps over an unread range" \
  fail PriQueue.cfg "Invariant (NoAckSkipped|MemWindow|GCOnlyAcked) is violated" \
  "$(on MutNoDirectAddLevelCheck)"
check "MutGcReadLevel: GC deletes loaded, unacked tasks" \
  fail PriQueue.cfg "Invariant (LoadedInDb|GCOnlyAcked) is violated" \
  "$(on MutGcReadLevel)"
check "MutNoGapSignal: reader sleeps in the middle of the backlog" \
  fail PriQueue.cfg "Temporal propert(y|ies).*(was|were) violated" "$(on MutNoGapSignal)"
check "MutNoReloadSignal: nothing wakes the reader after it fills up" \
  fail PriQueue.cfg "Temporal propert(y|ies).*(was|were) violated" "$(on MutNoReloadSignal)"
check "MutDrainedAckIgnoresOutstanding: ack level jumps over loaded tasks" \
  fail PriQueue.cfg "Invariant (MemWindow|NoAckSkipped) is violated" \
  "$(on MutDrainedAckIgnoresOutstanding)"

echo "=== not caught at this model size (expect pass) ==="
# signalNewTasks' "room in memory" check bounds memory, not correctness, and at
# MaxLevel = 3 a direct add can never exceed the bound anyway (it requires
# readLevel == maxReadLevelBefore, i.e. everything loaded is below the ids being
# added). Kept as documentation of what LoadedBounded does not prove.
check "MutNoRoomCheck: memory bound only, unreachable at this size" \
  pass PriQueue.cfg "No error has been found" "$(on MutNoRoomCheck)"

echo "=== findings regression (expected violation on the REAL model) ==="
# findings.md #1: a failed CreateTasks moves the db's max read level but never
# signals the reader, so the reader can stop below it forever.
check "finding #1: no read is triggered after a failed write" \
  fail PriQueue_caughtup.cfg "Temporal property EventuallyCaughtUp was violated" \
  "$(on WriteErrors)"
# findings.md #2: a read batch that is entirely expired raises the read level
# without any ack bookkeeping, and the ack level can then never catch up -- so
# the expired rows are never GCed.
check "finding #2: an all-expired batch strands the ack level" \
  fail PriQueue_caughtup.cfg "Temporal property EventuallyCaughtUp was violated" \
  's/Expirable = {}/Expirable = {2}/'

if [[ $fail -ne 0 ]]; then echo "=== FAILURES ==="; exit 1; fi
echo "=== all checks passed ==="

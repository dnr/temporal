#!/usr/bin/env bash
# Check the FairQueue model: translate, run TLC on the real model (expect
# pass), then run each mutation (expect the listed violation). A mutation
# re-introduces a bug; if TLC does NOT catch it, the model is too weak.
set -uo pipefail
cd "$(dirname "$0")"

JAR=../tla2tools.jar
TLC() { java -XX:+UseParallelGC -cp "$JAR" tlc2.TLC -workers auto "$@"; }

echo "=== translating ==="
java -cp "$JAR" pcal.trans -nocfg FairQueue.tla || exit 1

fail=0

# run_expect <pass|fail> <mutation-flag-or-'-'> <expected-output-regex> [nostuck]
# "nostuck" also removes the NoStuck invariant: it models code without the
# defensive "fair reader stuck" detector (which post-dates some of the bugs
# and otherwise catches them before the liveness violation manifests).
run_expect() {
  local expect=$1 mut=$2 pattern=$3 nostuck=${4:-}
  local cfg=FairQueue.cfg desc="real model"
  if [[ $mut != - ]]; then
    desc="mutation $mut${nostuck:+ (no NoStuck)}"
    cfg=mut_tmp.cfg
    sed "s/$mut = FALSE/$mut = TRUE/" FairQueue.cfg > "$cfg"
    if cmp -s "$cfg" FairQueue.cfg; then
      echo "FAIL: $desc: flag not found in FairQueue.cfg"; fail=1; return
    fi
    if [[ -n $nostuck ]]; then
      sed -i '/^  NoStuck$/d' "$cfg"
    fi
  fi
  local out
  out=$(TLC -config "$cfg" FairQueue.tla 2>&1)
  local status=$?
  if [[ $expect == pass && $status -ne 0 ]]; then
    echo "FAIL: $desc: expected pass, TLC found an error:"
    echo "$out" | grep -E "Error:" | head -5
    fail=1
  elif [[ $expect == fail && $status -eq 0 ]]; then
    echo "FAIL: $desc: expected TLC to find a violation, but it passed"
    fail=1
  elif ! echo "$out" | grep -qE "$pattern"; then
    echo "FAIL: $desc: output did not match /$pattern/:"
    echo "$out" | grep -E "Error:" | head -5
    fail=1
  else
    echo "ok: $desc"
  fi
  rm -f mut_tmp.cfg
}

echo "=== checking real model (expect pass) ==="
# full check (safety + liveness) at MaxLevel=3
run_expect pass - "No error has been found"

# safety-only at MaxLevel=4 for broader invariant coverage (liveness at 4
# takes too long for the default suite; run it manually when needed)
echo "=== checking real model, safety only, MaxLevel=4 (expect pass) ==="
out=$(TLC -config FairQueue_safety4.cfg FairQueue.tla 2>&1)
if [[ $? -ne 0 ]] || ! echo "$out" | grep -q "No error has been found"; then
  echo "FAIL: safety-only MaxLevel=4:"
  echo "$out" | grep -E "Error:" | head -5
  fail=1
else
  echo "ok: safety-only MaxLevel=4"
fi

echo "=== checking mutations (expect violations) ==="
# seeded bug: middle reads marked atEnd -> reader stops early, tasks never read
run_expect fail MutAtEndOnMiddleRead "Invariant NoAckSkipped is violated|Temporal propert(y|ies).*violated"
# seeded bug: ack level advances past loaded (unacked) tasks
run_expect fail MutAckPastLoaded "Invariant (MemWindow|NoAckSkipped) is violated"
# 12e7c43a: writes merged directly during a pending read get dropped when the
# read's stale to-end result establishes atEnd above them
run_expect fail MutNoWriteBuffering "Invariant NoAckSkipped is violated|Temporal propert(y|ies).*violated"
# f534e74e: collapsing readLevel to ackLevel on an empty merge evicts all acks
# and strands the reader (defensive stuck check fires)
run_expect fail MutResetReadLevelOnEmptyMerge "Invariant NoStuck is violated|Temporal propert(y|ies).*violated"
# 8ca7b640 #4: stale acks above the new readLevel let ackLevel jump over
# evicted (never-dispatched) tasks
run_expect fail MutKeepEvictedAcks "Invariant (MemWindow|NoAckSkipped) is violated"
# fairness.md "Problems" #1: without pinning, the ack level can pass levels of
# an in-flight write; the merge then drops the tasks as below-ack
run_expect fail MutNoPin "Invariant (PinProtectsWrites|MemWindow|NoAckSkipped|GCOnlyAcked) is violated|Temporal propert(y|ies).*violated"
# seeded bug: GC deletes up to readLevel -> deletes loaded, unacked tasks
run_expect fail MutGcReadLevel "Invariant (GCOnlyAcked|LoadedInDb) is violated"
# 26d9a561: backoff timer fires while readPending; without the exit re-check
# the reader never reads again. With the modern stuck detector the state is
# flagged as NoStuck; without it (historical code) it is a liveness bug.
run_expect fail MutNoExitRecheck "Invariant NoStuck is violated"
run_expect fail MutNoExitRecheck "Temporal propert(y|ies).*violated" nostuck
# 8ca7b640 #5: write error with empty buffer must trigger a read from unpin.
# Same detector note as above.
run_expect fail MutNoReadOnWriteError "Invariant NoStuck is violated"
run_expect fail MutNoReadOnWriteError "Temporal propert(y|ies).*violated" nostuck
# NOT a bug under the delivery contract: skipping the atEnd reset on write
# error strands only rows landed by timed-out writes, which carry no
# guarantee (the caller re-submits). The reset is best-effort delivery of
# such orphans, so the model passes with it removed. See findings.md.
run_expect pass MutNoAtEndResetOnWriteError "No error has been found"

if [[ $fail -ne 0 ]]; then echo "=== FAILURES ==="; exit 1; fi
echo "=== all checks passed ==="

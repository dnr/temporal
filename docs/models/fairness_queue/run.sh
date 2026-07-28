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

# run_expect <pass|fail> <mutation-flag-or-'-'> <expected-output-regex>
run_expect() {
  local expect=$1 mut=$2 pattern=$3
  local cfg=FairQueue.cfg desc="real model"
  if [[ $mut != - ]]; then
    desc="mutation $mut"
    cfg=mut_tmp.cfg
    sed "s/$mut = FALSE/$mut = TRUE/" FairQueue.cfg > "$cfg"
    if cmp -s "$cfg" FairQueue.cfg; then
      echo "FAIL: $desc: flag not found in FairQueue.cfg"; fail=1; return
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
run_expect pass - "No error has been found"

echo "=== checking mutations (expect violations) ==="
# seeded bug: middle reads marked atEnd -> reader stops early, tasks never read
run_expect fail MutAtEndOnMiddleRead "Temporal propert(y|ies).*violated"
# seeded bug: ack level advances past loaded (unacked) tasks
run_expect fail MutAckPastLoaded "Invariant (MemWindow|NoAckSkipped) is violated"

if [[ $fail -ne 0 ]]; then echo "=== FAILURES ==="; exit 1; fi
echo "=== all checks passed ==="

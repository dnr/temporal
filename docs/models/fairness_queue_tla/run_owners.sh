#!/usr/bin/env bash
# Check the FairQueueOwners model (ownership changes): current code must
# show the stale-owner GC bug; each candidate fix must either pass or show
# the expected violation.
set -uo pipefail
cd "$(dirname "$0")"

JAR=${TLA2TOOLS:-../tla2tools.jar}
TLC() { java -XX:+UseParallelGC -cp "$JAR" tlc2.TLC -workers auto "$@"; }

echo "=== translating ==="
java -cp "$JAR" pcal.trans -nocfg FairQueueOwners.tla || exit 1

fail=0

# run_expect <pass|fail> <desc> <expected-output-regex> <cfg> [NAME=VALUE ...]
# Overrides constants of <cfg> with the given values.
run_expect() {
  local expect=$1 desc=$2 pattern=$3 base=$4
  shift 4
  local cfg=owners_tmp.cfg
  cp "$base" "$cfg"
  for kv in "$@"; do
    local k=${kv%%=*} v=${kv#*=}
    if ! grep -qE "^  $k = " "$cfg"; then
      echo "FAIL: $desc: constant $k not found in $base"; fail=1; return
    fi
    sed -i "s|^  $k = .*|  $k = $v|" "$cfg"
  done
  local out
  out=$(TLC -config "$cfg" FairQueueOwners.tla 2>&1)
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
  rm -f "$cfg" FairQueueOwners_TTrace_*.tla
}

echo "=== current code (expect violation) ==="
# findings.md #4: a stale owner's unfenced GC deletes the new owner's tasks
run_expect fail "current code: stale-owner GC" "Invariant GCOnlyAcked is violated" \
  FairQueueOwners.cfg

echo "=== candidate fixes ==="
# check ownership (read range id) before GC: TOCTOU, still broken
run_expect fail "GcMode=verified" "Invariant GCOnlyAcked is violated" \
  FairQueueOwners.cfg 'GcMode="verified"'
# GC only up to the ack level persisted under our range id: broken by the
# takeover read -> LWT window (the takeover LWT writes back an older ack
# level after the old owner persisted, and GC'd up to, a newer one)
run_expect fail "GcMode=persisted" "Invariant GCOnlyAcked is violated" \
  FairQueueOwners.cfg 'GcMode="persisted"'
# ...plus a takeover that can't clobber a concurrently persisted ack level
run_expect pass "GcMode=persisted, TakeoverCAS" "No error has been found" \
  FairQueueOwners.cfg 'GcMode="persisted"' TakeoverCAS=TRUE
# GC delete fenced by range id
run_expect pass "GcMode=fenced" "No error has been found" \
  FairQueueOwners.cfg 'GcMode="fenced"'

echo "=== candidate fixes, liveness (FairQueueOwners_live.cfg) ==="
run_expect pass "GcMode=fenced, liveness" "No error has been found" \
  FairQueueOwners_live.cfg 'GcMode="fenced"'
run_expect pass "GcMode=persisted, TakeoverCAS, liveness" "No error has been found" \
  FairQueueOwners_live.cfg 'GcMode="persisted"' TakeoverCAS=TRUE

if [[ $fail -ne 0 ]]; then echo "=== FAILURES ==="; exit 1; fi
echo "=== all checks passed ==="

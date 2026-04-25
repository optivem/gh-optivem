#!/usr/bin/env bash
# End-to-end test for the new runner subcommands against the real shop
# repo's TypeScript / monolith layout. Runs the full latest + legacy suites
# (not --sample, not a single --suite filter).
#
# Steps:
#   1. Rebuild the gh-optivem binary from this repo.
#   2. From shop/system-test/typescript/:
#        gh-optivem run system        --system monolith/system.json
#        gh-optivem run system tests  --system monolith/system.json --tests tests-latest.json
#        gh-optivem run system tests  --system monolith/system.json --tests tests-legacy.json
#        gh-optivem stop system       --system monolith/system.json
#   3. Print a per-phase pass/fail summary.
#
# Requires: docker, node 22+, the optivem academy workspace cloned alongside
# this repo (so shop/ is at ../shop relative to gh-optivem/).
#
# Always tries to stop the system at the end (best-effort cleanup), even on
# prior failure. Exits non-zero if any test phase failed.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKSPACE_ROOT="$(cd "$REPO_ROOT/.." && pwd)"
SHOP_DIR="$WORKSPACE_ROOT/shop"
TS_DIR="$SHOP_DIR/system-test/typescript"
BIN="$REPO_ROOT/gh-optivem.exe"

SYSTEM="monolith/system.json"

echo "=== Step 1/5: Build gh-optivem ==="
( cd "$REPO_ROOT" && go build -o gh-optivem.exe . ) || {
  echo "FAILED: go build" >&2
  exit 1
}

if [ ! -d "$TS_DIR" ]; then
  echo "FAILED: shop typescript dir not found at $TS_DIR" >&2
  exit 1
fi

echo
echo "=== Step 2/5: run system (Up) ==="
( cd "$TS_DIR" && "$BIN" run system --system "$SYSTEM" )
UP_RC=$?
if [ $UP_RC -ne 0 ]; then
  echo "FAILED: run system exited $UP_RC" >&2
  echo "Attempting cleanup..."
  ( cd "$TS_DIR" && "$BIN" stop system --system "$SYSTEM" ) || true
  exit 1
fi

echo
echo "=== Step 3/5: run system tests — Latest (full suite) ==="
LATEST_RC=0
( cd "$TS_DIR" && "$BIN" run system tests --system "$SYSTEM" --tests tests-latest.json ) || LATEST_RC=$?

echo
echo "=== Step 4/5: run system tests — Legacy (full suite) ==="
LEGACY_RC=0
( cd "$TS_DIR" && "$BIN" run system tests --system "$SYSTEM" --tests tests-legacy.json ) || LEGACY_RC=$?

echo
echo "=== Step 5/5: stop system (cleanup) ==="
( cd "$TS_DIR" && "$BIN" stop system --system "$SYSTEM" ) || echo "warn: stop system failed (continuing)"

echo
echo "=== Summary ==="
printf "%-10s %s\n" "Latest:" "$([ $LATEST_RC -eq 0 ] && echo PASSED || echo "FAILED (rc=$LATEST_RC)")"
printf "%-10s %s\n" "Legacy:" "$([ $LEGACY_RC -eq 0 ] && echo PASSED || echo "FAILED (rc=$LEGACY_RC)")"

if [ $LATEST_RC -eq 0 ] && [ $LEGACY_RC -eq 0 ]; then
  exit 0
fi
exit 1

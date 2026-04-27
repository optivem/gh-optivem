#!/usr/bin/env bash
# End-to-end test for the runner subcommands against the real shop repo's
# TypeScript / monolith layout. Exercises the implicit-build/start path that
# `gh optivem test system` provides (inspired by `dotnet test` / `gradle
# test`) by starting from a stopped state.
#
# Steps:
#   1. Rebuild the gh-optivem binary from this repo.
#   2. From shop/ root:
#        gh-optivem stop system  --system docker/typescript/monolith/system.json   (best effort, cold start)
#        gh-optivem test system  --system docker/typescript/monolith/system.json --tests system-test/typescript/tests-latest.json   (cold — implicit build + start)
#        gh-optivem test system  --system docker/typescript/monolith/system.json --tests system-test/typescript/tests-legacy.json   (warm — fast re-run path)
#        gh-optivem stop system  --system docker/typescript/monolith/system.json
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
BIN="$REPO_ROOT/gh-optivem.exe"

SYSTEM="docker/typescript/monolith/system.json"
TESTS_LATEST="system-test/typescript/tests-latest.json"
TESTS_LEGACY="system-test/typescript/tests-legacy.json"

echo "=== Step 1/5: Build gh-optivem ==="
( cd "$REPO_ROOT" && go build -o gh-optivem.exe . ) || {
  echo "FAILED: go build" >&2
  exit 1
}

if [ ! -f "$SHOP_DIR/$SYSTEM" ]; then
  echo "FAILED: shop system.json not found at $SHOP_DIR/$SYSTEM" >&2
  exit 1
fi

echo
echo "=== Step 2/5: stop system (ensure cold start) ==="
( cd "$SHOP_DIR" && "$BIN" stop system --system "$SYSTEM" ) || echo "warn: stop system failed (continuing — system may already be down)"

echo
echo "=== Step 3/5: test system — Latest (cold: implicit build + start + tests) ==="
LATEST_RC=0
( cd "$SHOP_DIR" && "$BIN" test system --system "$SYSTEM" --tests "$TESTS_LATEST" ) || LATEST_RC=$?

echo
echo "=== Step 4/5: test system — Legacy (warm: system already up, fast re-run) ==="
LEGACY_RC=0
( cd "$SHOP_DIR" && "$BIN" test system --system "$SYSTEM" --tests "$TESTS_LEGACY" ) || LEGACY_RC=$?

echo
echo "=== Step 5/5: stop system (cleanup) ==="
( cd "$SHOP_DIR" && "$BIN" stop system --system "$SYSTEM" ) || echo "warn: stop system failed (continuing)"

echo
echo "=== Summary ==="
printf "%-10s %s\n" "Latest:" "$([ $LATEST_RC -eq 0 ] && echo PASSED || echo "FAILED (rc=$LATEST_RC)")"
printf "%-10s %s\n" "Legacy:" "$([ $LEGACY_RC -eq 0 ] && echo PASSED || echo "FAILED (rc=$LEGACY_RC)")"

if [ $LATEST_RC -eq 0 ] && [ $LEGACY_RC -eq 0 ]; then
  exit 0
fi
exit 1

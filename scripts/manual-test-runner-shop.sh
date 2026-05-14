#!/usr/bin/env bash
# End-to-end test for the runner subcommands against the real shop repo's
# TypeScript / monolith layout. Exercises the implicit-build/start path that
# `gh optivem test run` provides (inspired by `dotnet test` / `gradle
# test`) by starting from a stopped state.
#
# Steps:
#   1. Rebuild the gh-optivem binary from this repo.
#   2. From shop/ root, using --config / -c to select the TypeScript /
#      monolith variant of gh-optivem.yaml:
#        gh-optivem -c gh-optivem.shop-ts-monolith.yaml         system stop   (best effort, cold start)
#        gh-optivem -c gh-optivem.shop-ts-monolith.yaml         test run      (cold — implicit build + start, tests-latest)
#        gh-optivem -c gh-optivem.shop-ts-monolith-legacy.yaml  test run      (warm — fast re-run path, tests-legacy)
#        gh-optivem -c gh-optivem.shop-ts-monolith.yaml         system stop
#   3. Print a per-phase pass/fail summary.
#
# Requires: docker, node 22+, the optivem workspace cloned alongside this
# repo (so shop/ is at ../shop relative to gh-optivem/), and shop
# carrying two variant gh-optivem yaml files — one pointing at
# system-test/typescript/tests-latest.json, one at tests-legacy.json — both
# with system_config: docker/typescript/monolith/systems.json. The
# --system-config / --test-config flags were collapsed into gh-optivem.yaml;
# variant selection now happens by swapping the config file via -c.
#
# Always tries to stop the system at the end (best-effort cleanup), even on
# prior failure. Exits non-zero if any test phase failed.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKSPACE_ROOT="$(cd "$REPO_ROOT/.." && pwd)"
SHOP_DIR="$WORKSPACE_ROOT/shop"
BIN="$REPO_ROOT/gh-optivem.exe"

CFG_LATEST="gh-optivem.shop-ts-monolith.yaml"
CFG_LEGACY="gh-optivem.shop-ts-monolith-legacy.yaml"

echo "=== Step 1/5: Build gh-optivem ==="
( cd "$REPO_ROOT" && go build -o gh-optivem.exe . ) || {
  echo "FAILED: go build" >&2
  exit 1
}

for cfg in "$CFG_LATEST" "$CFG_LEGACY"; do
  if [ ! -f "$SHOP_DIR/$cfg" ]; then
    echo "FAILED: shop variant config not found at $SHOP_DIR/$cfg" >&2
    echo "  Create the two TypeScript / monolith variant yamls in shop with system_config: docker/typescript/monolith/systems.json and test_config: pointing at tests-latest.json / tests-legacy.json respectively." >&2
    exit 1
  fi
done

echo
echo "=== Step 2/5: system stop (ensure cold start) ==="
( cd "$SHOP_DIR" && "$BIN" -c "$CFG_LATEST" system stop ) || echo "warn: system stop failed (continuing — system may already be down)"

echo
echo "=== Step 3/5: test run — Latest (cold: implicit build + start + tests) ==="
LATEST_RC=0
( cd "$SHOP_DIR" && "$BIN" -c "$CFG_LATEST" test run ) || LATEST_RC=$?

echo
echo "=== Step 4/5: test run — Legacy (warm: system already up, fast re-run) ==="
LEGACY_RC=0
( cd "$SHOP_DIR" && "$BIN" -c "$CFG_LEGACY" test run ) || LEGACY_RC=$?

echo
echo "=== Step 5/5: system stop (cleanup) ==="
( cd "$SHOP_DIR" && "$BIN" -c "$CFG_LATEST" system stop ) || echo "warn: system stop failed (continuing)"

echo
echo "=== Summary ==="
printf "%-10s %s\n" "Latest:" "$([ $LATEST_RC -eq 0 ] && echo PASSED || echo "FAILED (rc=$LATEST_RC)")"
printf "%-10s %s\n" "Legacy:" "$([ $LEGACY_RC -eq 0 ] && echo PASSED || echo "FAILED (rc=$LEGACY_RC)")"

if [ $LATEST_RC -eq 0 ] && [ $LEGACY_RC -eq 0 ]; then
  exit 0
fi
exit 1

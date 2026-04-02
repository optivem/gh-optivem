#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

# Required: TEST_OWNER=<github-user-or-org>
# Optional: TEST_NO_CLEANUP=1 to keep repos after test (for inspection)
# Usage: TEST_OWNER=valentinajemuovic bash scripts/test-system.sh

if [ -z "${TEST_OWNER:-}" ]; then
  echo "ERROR: TEST_OWNER environment variable is required" >&2
  exit 1
fi

TESTS=(
  # "TestValidMonolithConfigurations/monolith_monorepo_java_java"
  "TestValidMultitierConfigurations/multitier_monorepo_java_react_java"
)

RUN_PATTERN=$(IFS="|"; echo "${TESTS[*]}")

go test -tags=system ./internal/config/ -v -timeout 30m -run "$RUN_PATTERN"

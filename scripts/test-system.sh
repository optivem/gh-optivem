#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

# Set TEST_NO_CLEANUP=1 to keep repos after test (for inspection)
# Usage: TEST_NO_CLEANUP=1 bash scripts/test-system.sh

TESTS=(
  # "TestValidMonolithConfigurations/monolith_monorepo_java_java"
  "TestValidMultitierConfigurations/multitier_monorepo_java_react_java"
)

RUN_PATTERN=$(IFS="|"; echo "${TESTS[*]}")

go test -tags=system ./internal/config/ -v -timeout 30m -run "$RUN_PATTERN"

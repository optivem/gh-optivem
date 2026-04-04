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
  # All 4 arch/strategy combos × all 3 backend langs (same test lang)
  # NOTE: cross-language tests (system lang != test lang) require docker-compose
  # env var fixups that are not yet implemented — tracked as a known issue.
  # "TestValidMonolithConfigurations/monolith_monorepo_java_java"
  # "TestValidMonolithConfigurations/monolith_multirepo_dotnet_dotnet"
  "TestValidMultitierConfigurations/multitier_monorepo_typescript_react_typescript"
  # "TestValidMultitierConfigurations/multitier_multirepo_java_react_java"
)

RUN_PATTERN=$(IFS="|"; echo "${TESTS[*]}")

go test -tags=system ./internal/config/ -v -timeout 6h -run "$RUN_PATTERN"

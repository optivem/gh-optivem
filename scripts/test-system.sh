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
  # All 4 arch/strategy combos × all 3 backend langs × mixed test langs
  # Monolith monorepo: java system, dotnet tests (cross-lang: port + image fixup)
  "TestValidMonolithConfigurations/monolith_monorepo_java_dotnet"
  # Monolith multirepo: dotnet system, typescript tests (cross-lang + multirepo)
  "TestValidMonolithConfigurations/monolith_multirepo_dotnet_typescript"
  # Multitier monorepo: typescript backend, java tests (cross-lang: port 3000 vs 8080)
  "TestValidMultitierConfigurations/multitier_monorepo_typescript_react_java"
  # Multitier multirepo: java backend, dotnet tests (cross-lang + multirepo)
  "TestValidMultitierConfigurations/multitier_multirepo_java_react_dotnet"
)

RUN_PATTERN=$(IFS="|"; echo "${TESTS[*]}")

go test -tags=system ./internal/config/ -v -timeout 6h -run "$RUN_PATTERN"

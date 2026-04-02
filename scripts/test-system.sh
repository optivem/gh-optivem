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
  # One per arch/strategy, covering java, dotnet, typescript backends
  "TestValidMonolithConfigurations/monolith_monorepo_java_java"
  "TestValidMonolithConfigurations/monolith_monorepo_dotnet_dotnet"
  "TestValidMonolithConfigurations/monolith_multirepo_typescript_typescript"
  "TestValidMultitierConfigurations/multitier_monorepo_java_react_java"
  "TestValidMultitierConfigurations/multitier_monorepo_dotnet_react_dotnet"
  "TestValidMultitierConfigurations/multitier_multirepo_typescript_react_typescript"
  # Cross-language (system lang != test lang)
  "TestValidMonolithConfigurations/monolith_monorepo_java_dotnet"
  "TestValidMultitierConfigurations/multitier_monorepo_java_react_typescript"
)

RUN_PATTERN=$(IFS="|"; echo "${TESTS[*]}")

go test -tags=system ./internal/config/ -v -timeout 6h -run "$RUN_PATTERN"

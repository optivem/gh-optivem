#!/usr/bin/env bash
set -euo pipefail

# Default test runner. Caps parallel package builds via -p ${GO_TEST_P:-2} —
# mainly a Windows mitigation; on Linux/macOS pass GO_TEST_P=0 to use NumCPU.
# Usage: bash scripts/test.sh ./internal/atdd/...

exec go test -p "${GO_TEST_P:-2}" "$@"

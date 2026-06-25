#!/usr/bin/env bash
# Wrapper for `go test` that enforces a parallelism cap and forces a
# conscious opt-in for repo-wide runs. See CONTRIBUTING.md
# "Windows: keep `go test ./...` fast".
#
# Usage:
#   bash scripts/test.sh ./internal/atdd/runtime/clauderun     # one package
#   bash scripts/test.sh ./internal/atdd/...                   # subtree (capped)
#   bash scripts/test.sh --all ./...                           # repo-wide (opt-in)
#
# Knobs:
#   GO_TEST_P=N   cap parallel package builds (default 2; set 0 for NumCPU)

set -euo pipefail

p="${GO_TEST_P:-2}"
all=0
filtered=()

if [[ $# -eq 0 ]]; then
    cat >&2 <<'EOF'
scripts/test.sh: no arguments.
Usage:
  bash scripts/test.sh ./internal/atdd/runtime/clauderun     # one package
  bash scripts/test.sh ./internal/atdd/...                   # subtree (capped)
  bash scripts/test.sh --all ./...                           # repo-wide (opt-in)
EOF
    exit 2
fi

for arg in "$@"; do
    if [[ "$arg" == "--all" ]]; then
        all=1
        continue
    fi
    filtered+=("$arg")
done

for arg in "${filtered[@]}"; do
    if [[ "$arg" == "./..." && "$all" -ne 1 ]]; then
        cat >&2 <<'EOF'
scripts/test.sh: refusing `./...` without --all.
Repo-wide `go test` link-storms Windows. Either:
  - scope to a subtree: bash scripts/test.sh ./internal/atdd/...
  - or opt in:          bash scripts/test.sh --all ./...
EOF
        exit 2
    fi
done

has_p=0
for a in "${filtered[@]}"; do
    case "$a" in
        -p|-p=*) has_p=1 ;;
        *) ;;  # other args don't affect -p detection
    esac
done

cmd=(go test)
[[ "$has_p" -eq 0 ]] && cmd+=(-p "$p")
cmd+=("${filtered[@]}")

echo "+ ${cmd[*]}" >&2
exec "${cmd[@]}"

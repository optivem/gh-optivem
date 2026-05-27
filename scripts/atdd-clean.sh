#!/usr/bin/env bash
set -euo pipefail

# Wrap body in `{ ... } && exit` so bash parses the entire script up-front.
# Without this, bash maintains a file offset and re-reads from disk between
# commands; if the file is edited during a long-running command, bash's offset
# desyncs and emits phantom syntax errors after the long command returns.
{

# Wraps `gh optivem system clean` against the consumer repo to drop volumes +
# locally-built images from prior rehearsals. Personal dev workflow for the
# plan author — not a CLI feature consumers need.
#
# Usage:
#   bash atdd-clean.sh [--config <yaml>]
#
#   --config:  path (relative to the consumer worktree) of the gh-optivem.yaml
#              variant to clean against. Default: gh-optivem-monolith-typescript.yaml.
#              The shop template commits one yaml per stack (monolith/multitier
#              × typescript/java/dotnet × legacy); pick the one matching the
#              stack whose state you want cleared.
#
# Workflow:
#   1. Build gh-optivem.exe from this repo (so clean exercises uncommitted
#      local changes, not the installed `gh optivem`). Matters because
#      `system clean` reads systems.yaml via the same path resolution as the
#      rest of the binary; an outdated installed copy would silently scope to
#      the wrong systems.
#   2. `gh optivem system clean` against the consumer repo with the chosen
#      config. Drops volumes + locally-built images; registry-pulled images
#      are preserved — same scope as `./gradlew clean`. Project-scoped: only
#      cleans stacks listed in the *current* config's systems.yaml, so
#      switching configs across sessions can leave the other stack's state
#      behind. Exits with the clean step's own rc — failures are not swallowed.
#
# The consumer repo is always resolved as a sibling of gh-optivem named per
# REHEARSAL_REPO (e.g. ../shop). The script can be invoked from any CWD — it
# does not consult the current working tree.

# === REHEARSAL CONFIG === (edit these for your setup)
REHEARSAL_REPO="shop"
REHEARSAL_DEFAULT_CONFIG="gh-optivem-monolith-typescript.yaml"
# === END REHEARSAL CONFIG ===

GH_OPTIVEM_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$GH_OPTIVEM_ROOT/gh-optivem.exe"

if [[ -t 1 ]]; then
  C_BOLD=$'\033[1m'; C_CYAN=$'\033[36m'; C_RESET=$'\033[0m'
else
  C_BOLD=''; C_CYAN=''; C_RESET=''
fi
PREFIX="${C_CYAN}[atdd-clean]${C_RESET}"
log() { echo "${PREFIX} $*"; }

usage() {
  echo "Usage: $0 [--config <yaml>]" >&2
}

CONFIG="$REHEARSAL_DEFAULT_CONFIG"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      sed -n '10,38p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    -c|--config)
      if [[ $# -lt 2 ]]; then
        echo "ERROR: $1 requires a value" >&2
        exit 2
      fi
      CONFIG="$2"
      shift 2
      ;;
    --config=*)
      CONFIG="${1#--config=}"
      shift
      ;;
    -*)
      echo "ERROR: unknown flag: $1" >&2
      usage
      exit 2
      ;;
    *)
      echo "ERROR: unexpected argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

CONSUMER_ROOT="$(cd "$GH_OPTIVEM_ROOT/.." && pwd)/$REHEARSAL_REPO"
if [[ ! -d "$CONSUMER_ROOT/.git" ]]; then
  echo "ERROR: consumer repo not found at $CONSUMER_ROOT" >&2
  echo "Expected sibling of $GH_OPTIVEM_ROOT named '$REHEARSAL_REPO'." >&2
  exit 2
fi

log "${C_BOLD}Clean:${C_RESET}"
log "  consumer:    $CONSUMER_ROOT"
log "  config:      $CONFIG"
log "  built from:  $GH_OPTIVEM_ROOT"
log "  binary:      $BIN"

log "Building gh-optivem..."
BUILD_LOG="$(mktemp)"
if ! ( cd "$GH_OPTIVEM_ROOT" && go build -o gh-optivem.exe . ) >"$BUILD_LOG" 2>&1; then
  cat "$BUILD_LOG" >&2
  log "go build failed — aborting before clean."
  if grep -q 'build constraints exclude all Go files' "$BUILD_LOG"; then
    log ""
    log "Likely cause: CGO is disabled in your Go env (tree-sitter bindings need CGO)."
    log "  Check:  go env CGO_ENABLED      (expect: 1)"
    log "  Fix:    go env -w CGO_ENABLED=1"
    log "  Then re-run this script."
  elif grep -qE '(C compiler "gcc" not found|gcc.*executable file not found)' "$BUILD_LOG"; then
    log ""
    log "Likely cause: no C compiler on PATH (tree-sitter bindings need CGO + gcc)."
    log "  Install (Windows):  scoop install gcc        (or: choco install mingw, admin shell)"
    log "  Install (macOS):    xcode-select --install"
    log "  Install (Linux):    apt install gcc          (or your distro equivalent)"
    log "  Verify:             gcc --version            (should print a version)"
    log "  Then re-run this script (open a fresh terminal so PATH picks up gcc)."
  fi
  rm -f "$BUILD_LOG"
  exit 1
fi
rm -f "$BUILD_LOG"

log "Cleaning local docker state (volumes + locally-built images; registry images preserved)..."
( cd "$CONSUMER_ROOT" && GH_OPTIVEM_CONFIG="$CONSUMER_ROOT/$CONFIG" "$BIN" system clean )

} && exit 0

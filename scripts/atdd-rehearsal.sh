#!/usr/bin/env bash
set -euo pipefail

# Wrap body in `{ ... } && exit` so bash parses the entire script up-front.
# Without this, bash maintains a file offset and re-reads from disk between
# commands; if the file is edited during a long-running command (e.g. an
# implement-ticket walk that takes 30+ minutes), bash's offset desyncs and
# emits phantom syntax errors after the long command returns.
{

# Wraps `gh optivem atdd implement-ticket` in a throwaway git worktree on a
# rehearsal branch. Personal dev workflow for the plan author — not a CLI
# feature consumers need.
#
# Usage:
#   bash atdd-rehearsal.sh <issue-num> [label]
#
#   issue-num: GitHub issue number, or full issue URL — forwarded as-is to
#              `gh optivem atdd implement-ticket --issue ...`.
#   label:     optional [A-Za-z0-9_-]+ tacked onto the worktree id for
#              sortability (e.g. "ticket-61", "follow-up").
#
# Workflow:
#   1. Build gh-optivem.exe from this repo (so the rehearsal exercises
#      uncommitted local changes, not the installed `gh optivem`).
#   2. Resolve <id> = <ts>[-<label>], where <ts> = date +%Y%m%d-%H%M%S.
#   3. From the consumer repo (CWD), create a sibling worktree at
#      ../rehearsal-<id> on a new branch rehearsal/<id>.
#   4. Inside the new worktree, run `<gh-optivem>/gh-optivem.exe config init …`
#      to materialise gh-optivem.yaml (the shop template doesn't commit it;
#      implement-ticket needs it to resolve project URL and scope axes).
#      Commit the YAML so the rehearsal branch carries a coherent history.
#   5. cd into it and run:
#        <gh-optivem>/gh-optivem.exe atdd implement-ticket --issue <issue-num>
#   6. On exit (success, failure, or interrupt), prompt the user to delete
#      the worktree + branch (default: yes).
#
# The consumer repo is always resolved as a sibling of gh-optivem named
# per REHEARSAL_REPO (e.g. ../shop). The script can be invoked from any
# CWD — it does not consult the current working tree.

# === REHEARSAL CONFIG === (edit these for your setup)
REHEARSAL_OWNER="optivem"
REHEARSAL_REPO="shop"
REHEARSAL_SYSTEM_NAME="Page Turner"
REHEARSAL_ARCH="monolith"
REHEARSAL_REPO_STRATEGY="monorepo"
REHEARSAL_MONOLITH_LANG="typescript"
REHEARSAL_PROJECT_URL="https://github.com/orgs/optivem/projects/20"

# Tier paths matching shop's worktree layout. `gh optivem config init` no
# longer derives paths — every caller passes them explicitly. Shop nests
# system code under system/<arch>/<lang>/ and tests under system-test/<lang>/
# so the rehearsal worktree (which is shop's tree) needs these spellings.
REHEARSAL_SYSTEM_PATH="system/${REHEARSAL_ARCH}/${REHEARSAL_MONOLITH_LANG}"
REHEARSAL_SYSTEM_TEST_PATH="system-test/${REHEARSAL_MONOLITH_LANG}"
REHEARSAL_STUBS_PATH="external-systems/external-stub"
REHEARSAL_SIMULATORS_PATH="external-systems/external-real-sim"
# === END REHEARSAL CONFIG ===

GH_OPTIVEM_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$GH_OPTIVEM_ROOT/gh-optivem.exe"

if [[ -t 1 ]]; then
  C_BOLD=$'\033[1m'; C_CYAN=$'\033[36m'; C_RESET=$'\033[0m'
else
  C_BOLD=''; C_CYAN=''; C_RESET=''
fi
PREFIX="${C_CYAN}[atdd-rehearsal]${C_RESET}"
log() { echo "${PREFIX} $*"; }

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  sed -n '11,36p' "$0" | sed 's/^# \{0,1\}//'
  exit 0
fi

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "Usage: $0 <issue-num> [label]" >&2
  exit 2
fi
ISSUE="$1"
LABEL="${2:-}"

if [[ -n "$LABEL" && ! "$LABEL" =~ ^[A-Za-z0-9_-]+$ ]]; then
  echo "ERROR: label must match [A-Za-z0-9_-]+ (got: $LABEL)" >&2
  exit 2
fi

CONSUMER_ROOT="$(cd "$GH_OPTIVEM_ROOT/.." && pwd)/$REHEARSAL_REPO"
if [[ ! -d "$CONSUMER_ROOT/.git" ]]; then
  echo "ERROR: consumer repo not found at $CONSUMER_ROOT" >&2
  echo "Expected sibling of $GH_OPTIVEM_ROOT named '$REHEARSAL_REPO'." >&2
  exit 2
fi

TS="$(date +%Y%m%d-%H%M%S)"
if [[ -n "$LABEL" ]]; then
  ID="${TS}-${LABEL}"
else
  ID="${TS}"
fi
WORKTREE_PATH="$(cd "$(dirname "$CONSUMER_ROOT")" && pwd)/rehearsal-${ID}"
BRANCH="rehearsal/${ID}"

cleanup() {
  local rc=$?
  cd "$CONSUMER_ROOT"
  if [[ ! -d "$WORKTREE_PATH" ]]; then
    return $rc
  fi
  echo ""
  local ans
  read -r -p "${C_BOLD}Delete worktree $WORKTREE_PATH and branch $BRANCH? [Y/n]${C_RESET} " ans || ans="y"
  case "${ans:-y}" in
    [Nn]*)
      log "Keeping $WORKTREE_PATH (branch $BRANCH)."
      ;;
    *)
      git -C "$CONSUMER_ROOT" worktree remove --force "$WORKTREE_PATH" || true
      git -C "$CONSUMER_ROOT" branch -D "$BRANCH" 2>/dev/null || true
      log "Removed $WORKTREE_PATH (branch $BRANCH)."
      ;;
  esac
  return $rc
}

# Rehearsal-only configuration. The binary prints the consumer-facing
# bits (project URL, scope, etc.) once it's invoked; here we surface only
# the values the script itself materialises (worktree + branch) plus the
# fact that we're exercising a freshly-built binary out of GH_OPTIVEM_ROOT
# rather than whatever `gh optivem` is installed on PATH.
log "${C_BOLD}Rehearsal:${C_RESET}"
log "  worktree:    $WORKTREE_PATH"
log "  branch:      $BRANCH"
if [[ -n "$LABEL" ]]; then
  log "  label:       $LABEL"
fi
log "  built from:  $GH_OPTIVEM_ROOT"
log "  binary:      $BIN"

log "Building gh-optivem..."
BUILD_LOG="$(mktemp)"
if ! ( cd "$GH_OPTIVEM_ROOT" && go build -o gh-optivem.exe . ) >"$BUILD_LOG" 2>&1; then
  cat "$BUILD_LOG" >&2
  log "go build failed — aborting before worktree."
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

log "Creating worktree at $WORKTREE_PATH on branch $BRANCH..."
if ! git -C "$CONSUMER_ROOT" worktree add -b "$BRANCH" "$WORKTREE_PATH"; then
  log "worktree add failed — aborting."
  exit 1
fi

# Trap installed *after* worktree creation: pre-worktree failures do not
# trigger the cleanup prompt.
trap cleanup EXIT

# The shop template doesn't commit gh-optivem.yaml (real users get one from
# `gh optivem init`), so the worktree starts without it. Materialise the file
# via `config init` and commit it so the rehearsal branch carries a coherent
# history that matches what a real init-scaffolded repo would look like.
log "Writing gh-optivem.yaml into worktree..."
( cd "$WORKTREE_PATH" && "$BIN" config init \
    --owner "$REHEARSAL_OWNER" \
    --repo "$REHEARSAL_REPO" \
    --system-name "$REHEARSAL_SYSTEM_NAME" \
    --arch "$REHEARSAL_ARCH" \
    --repo-strategy "$REHEARSAL_REPO_STRATEGY" \
    --monolith-lang "$REHEARSAL_MONOLITH_LANG" \
    --project-url "$REHEARSAL_PROJECT_URL" \
    --system-path "$REHEARSAL_SYSTEM_PATH" \
    --system-test-path "$REHEARSAL_SYSTEM_TEST_PATH" \
    --stubs-path "$REHEARSAL_STUBS_PATH" \
    --simulators-path "$REHEARSAL_SIMULATORS_PATH" )

log "Committing gh-optivem.yaml to rehearsal branch..."
( cd "$WORKTREE_PATH" \
    && git add gh-optivem.yaml \
    && git commit -m "Add gh-optivem.yaml for rehearsal" )

log "Running implement-ticket --issue $ISSUE in $WORKTREE_PATH..."
RC=0
( cd "$WORKTREE_PATH" && "$BIN" atdd implement-ticket --issue "$ISSUE" ) || RC=$?

if [[ $RC -eq 0 ]]; then
  log "implement-ticket succeeded."
else
  log "implement-ticket exited with rc=$RC."
fi

exit $RC

} && exit 0

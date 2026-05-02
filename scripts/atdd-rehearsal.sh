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
#   4. cd into it and run:
#        <gh-optivem>/gh-optivem.exe atdd implement-ticket --issue <issue-num>
#   5. On exit (success, failure, or interrupt), prompt the user to delete
#      the worktree + branch (default: yes).
#
# Run from inside the consumer repo's working tree (e.g. shop/). The script
# discovers the consumer repo via `git rev-parse --show-toplevel` from CWD;
# if you are not in a git tree, it errors out.

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

CONSUMER_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || {
  echo "ERROR: $PWD is not inside a git working tree." >&2
  echo "Run this script from the consumer repo (e.g. shop/)." >&2
  exit 2
}

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

log "Building gh-optivem..."
if ! ( cd "$GH_OPTIVEM_ROOT" && go build -o gh-optivem.exe . ); then
  log "go build failed — aborting before worktree."
  exit 1
fi

log "Creating worktree at $WORKTREE_PATH on branch $BRANCH..."
if ! git -C "$CONSUMER_ROOT" worktree add -b "$BRANCH" "$WORKTREE_PATH"; then
  log "worktree add failed — aborting."
  exit 1
fi

# Trap installed *after* worktree creation: pre-worktree failures do not
# trigger the cleanup prompt.
trap cleanup EXIT

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

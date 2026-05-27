#!/usr/bin/env bash
set -euo pipefail

# Wrap body in `{ ... } && exit` so bash parses the entire script up-front.
# Without this, bash maintains a file offset and re-reads from disk between
# commands; if the file is edited during a long-running command (e.g. an
# implement walk that takes 30+ minutes), bash's offset desyncs and
# emits phantom syntax errors after the long command returns.
{

# Wraps `gh optivem implement` in a throwaway git worktree on a
# rehearsal branch. Personal dev workflow for the plan author — not a CLI
# feature consumers need.
#
# Usage:
#   bash atdd-rehearsal.sh <issue-num> [label] [--config <yaml>]
#
#   issue-num: GitHub issue number, or full issue URL — forwarded as-is to
#              `gh optivem implement --issue ...`.
#   label:     optional [A-Za-z0-9_-]+ tacked onto the worktree id for
#              sortability (e.g. "ticket-61", "follow-up").
#   --config:  path (relative to the consumer worktree) of the gh-optivem.yaml
#              variant to exercise. Default: gh-optivem-monolith-typescript.yaml.
#              The shop template commits one yaml per stack (monolith/multitier
#              × typescript/java/dotnet × legacy); pick the one matching the
#              ticket you're rehearsing.
#
# Workflow:
#   1. Build gh-optivem.exe from this repo (so the rehearsal exercises
#      uncommitted local changes, not the installed `gh optivem`).
#   2. `gh optivem system clean` against the consumer repo to drop
#      volumes + locally-built images from prior rehearsals (registry-
#      pulled images are preserved — same scope as `./gradlew clean`).
#      Project-scoped: only cleans stacks listed in the *current* config's
#      systems.yaml, so switching configs across sessions can leave the
#      other stack's state behind. Non-fatal: failure (e.g. docker daemon
#      down) warns and continues.
#   3. Resolve <id> = <ts>[-<label>], where <ts> = date +%Y%m%d-%H%M%S.
#   4. From the consumer repo, create a worktree one level above the
#      academy folder at ../../rehearsal-<id> on a new branch
#      rehearsal/<id>. Placed outside the academy on purpose: a worktree
#      inside the academy makes `gh optivem commit` resolve ModeWorkspace
#      against academy.code-workspace and skip the worktree silently.
#      Preflight still works in this location because repolocator's
#      mono-repo branch walks up from CWD for .git instead of guessing
#      the parent dir. The chosen --config yaml is already committed in
#      shop, so it lands in the worktree automatically — no copy or
#      init step needed.
#   5. cd into it and run, with $GH_OPTIVEM_CONFIG pointing at the chosen yaml:
#        <gh-optivem>/gh-optivem.exe implement --issue <issue-num>
#   6. On exit (success, failure, or interrupt), prompt the user to delete
#      the worktree + branch (default: yes).
#
# The consumer repo is always resolved as a sibling of gh-optivem named
# per REHEARSAL_REPO (e.g. ../shop). The script can be invoked from any
# CWD — it does not consult the current working tree.

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
PREFIX="${C_CYAN}[atdd-rehearsal]${C_RESET}"
log() { echo "${PREFIX} $*"; }

# prompt_yn <prompt>
# Matches internal/promptio.ConfirmYN: explicit y/n required, no Enter
# shortcut. Loops on unrecognized input (including bare Enter) so a stray
# keystroke never silently accepts or declines. Case-insensitive. Returns 0
# for yes, 1 for no (or on EOF, matching the "silence = no" terminator).
prompt_yn() {
  local prompt="$1"
  local ans lc
  while true; do
    if ! read -r -p "${C_BOLD}${prompt} [y/n]:${C_RESET} " ans; then
      return 1
    fi
    lc="$(printf '%s' "$ans" | tr '[:upper:]' '[:lower:]')"
    case "$lc" in
      y|yes) return 0 ;;
      n|no)  return 1 ;;
      *)     echo "Please answer y or n." >&2 ;;
    esac
  done
}

usage() {
  echo "Usage: $0 <issue-num> [label] [--config <yaml>]" >&2
}

ISSUE=""
LABEL=""
CONFIG="$REHEARSAL_DEFAULT_CONFIG"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      sed -n '12,42p' "$0" | sed 's/^# \{0,1\}//'
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
    --)
      shift
      while [[ $# -gt 0 ]]; do
        if [[ -z "$ISSUE" ]]; then ISSUE="$1"
        elif [[ -z "$LABEL" ]]; then LABEL="$1"
        else echo "ERROR: unexpected argument: $1" >&2; exit 2
        fi
        shift
      done
      ;;
    -*)
      echo "ERROR: unknown flag: $1" >&2
      usage
      exit 2
      ;;
    *)
      if [[ -z "$ISSUE" ]]; then
        ISSUE="$1"
      elif [[ -z "$LABEL" ]]; then
        LABEL="$1"
      else
        echo "ERROR: unexpected argument: $1" >&2
        usage
        exit 2
      fi
      shift
      ;;
  esac
done

if [[ -z "$ISSUE" ]]; then
  usage
  exit 2
fi

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
# Worktree lives one level *above* the academy folder (sibling of academy),
# not next to the consumer repo. Reason: when the worktree sits inside the
# academy dir, `gh optivem commit` from inside it walks up to
# academy.code-workspace and resolves ModeWorkspace, iterating the declared
# academy repos and silently ignoring the worktree. Placing the worktree
# outside the academy keeps walk-up from finding that workspace file, so
# the resolver falls through to ModeProject (gh-optivem.yaml inside the
# worktree) → ModeSingleRepo on the worktree itself.
# Preflight stays happy because repolocator's mono-repo branch walks up
# from the worktree's CWD for .git, finding the worktree (whose .git is a
# file pointer) regardless of its directory name.
WORKTREE_PATH="$(cd "$(dirname "$CONSUMER_ROOT")/.." && pwd)/rehearsal-${ID}"
BRANCH="rehearsal/${ID}"

cleanup() {
  local rc=$?
  cd "$CONSUMER_ROOT"
  if [[ ! -d "$WORKTREE_PATH" ]]; then
    return $rc
  fi
  echo ""
  if prompt_yn "Delete worktree $WORKTREE_PATH and branch $BRANCH?"; then
    git -C "$CONSUMER_ROOT" worktree remove --force "$WORKTREE_PATH" || true
    git -C "$CONSUMER_ROOT" branch -D "$BRANCH" 2>/dev/null || true
    log "Removed $WORKTREE_PATH (branch $BRANCH)."
  else
    log "Keeping $WORKTREE_PATH (branch $BRANCH)."
  fi
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
log "  config:      $CONFIG"
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

# Drop volumes + locally-built images from prior rehearsals before the new
# worktree is created. Run against the *parent* consumer repo (the worktree
# doesn't exist yet); the docker daemon is global so this clears state
# regardless of which checkout we run from. Project-scoped (per the current
# config's systems.yaml) — registry-pulled bases are preserved.
log "Cleaning local docker state from prior rehearsals (volumes + locally-built images; registry images preserved)..."
( cd "$CONSUMER_ROOT" && GH_OPTIVEM_CONFIG="$CONSUMER_ROOT/$CONFIG" "$BIN" system clean ) || log "warn: system clean failed (continuing)"

log "Creating worktree at $WORKTREE_PATH on branch $BRANCH..."
if ! git -C "$CONSUMER_ROOT" worktree add -b "$BRANCH" "$WORKTREE_PATH"; then
  log "worktree add failed — aborting."
  exit 1
fi

# Trap installed *after* worktree creation: pre-worktree failures do not
# trigger the cleanup prompt.
trap cleanup EXIT

CONFIG_FULL="$WORKTREE_PATH/$CONFIG"
if [[ ! -f "$CONFIG_FULL" ]]; then
  log "ERROR: config file not found in worktree: $CONFIG_FULL"
  log "Expected one of the gh-optivem-*.yaml variants committed in $REHEARSAL_REPO."
  exit 2
fi

log "Running implement --issue $ISSUE in $WORKTREE_PATH..."
RC=0
( cd "$WORKTREE_PATH" && GH_OPTIVEM_CONFIG="$CONFIG_FULL" "$BIN" implement --issue "$ISSUE" ) || RC=$?

if [[ $RC -eq 0 ]]; then
  log "implement succeeded."
else
  log "implement exited with rc=$RC."
fi

exit $RC

} && exit 0

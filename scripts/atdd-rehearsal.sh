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
#   bash atdd-rehearsal.sh <issue-num> [label] [--config <yaml>] [--auto] [--headless]
#
#   issue-num:  GitHub issue number, or full issue URL — forwarded as-is to
#               `gh optivem implement --issue ...`.
#   label:      optional [A-Za-z0-9_-]+ slug that overrides the fetched
#               issue title in the worktree id. By default, the script
#               fetches the issue title from the consumer repo via
#               `gh issue view` and slugifies it; pass [label] to skip
#               that and use your own slug instead (e.g. "follow-up").
#   --config:   path (relative to the consumer worktree) of the gh-optivem.yaml
#               variant to exercise. Default: gh-optivem-monolith-typescript.yaml.
#               The shop template commits one yaml per stack (monolith/multitier
#               × typescript/java/dotnet × legacy); pick the one matching the
#               ticket you're rehearsing.
#   --auto:     auto-approve every prompt except commit/fix (forwarded to the
#               binary as a root flag, before `implement`).
#   --headless: run each claude subagent as `claude -p` instead of an
#               interactive session (forwarded to `implement`). Combine with
#               --auto for fully autonomous mode.
#
# Workflow:
#   1. Build gh-optivem.exe from this repo (so the rehearsal exercises
#      uncommitted local changes, not the installed `gh optivem`).
#   2. `gh optivem system clean` against the consumer repo to drop
#      volumes + locally-built images from prior rehearsals (registry-
#      pulled images are preserved — same scope as `./gradlew clean`).
#      Project-scoped: only cleans stacks listed in the *current* config's
#      systems.yaml, so switching configs across sessions can leave the
#      other stack's state behind. Fatal: failure aborts the rehearsal
#      before worktree creation.
#   3. Resolve <id> = <ts>-<issue>-<slug>, where <ts> = date +%Y%m%d-%H%M%S,
#      <issue> is the numeric issue id (extracted from the URL if needed),
#      and <slug> is the explicit [label] arg or, if absent, the issue
#      title fetched via `gh issue view` from the consumer repo, lowercased
#      and slugified to [a-z0-9-]+ (capped at ~40 chars on a word boundary).
#      The title fetch is a hard prerequisite — if `gh issue view` fails
#      (auth, network, wrong repo, nonexistent issue), the rehearsal aborts
#      before any worktree is created. Pass [label] to skip the fetch.
#      <ts> keeps the id unique across same-ticket reruns.
#   4. From the consumer repo, create a worktree under
#      <academy>/worktrees/rehearsal-<id> on a new branch rehearsal/<id>.
#      Grouped under `worktrees/` so a single multi-root VS Code
#      workspace can surface every rehearsal without per-run setup.
#      Safe to live inside the academy because the workspace resolver
#      (internal/workspace.resolveFrom) applies a CWD-membership check
#      to walk-up matches: the surrounding *.code-workspace is honored
#      only when CWD's git repo is one of its folders[] entries, so a
#      worktree that is not declared in the workspace falls through to
#      ModeSingleRepo on the worktree itself rather than being silently
#      replaced by the workspace's declared repos. The chosen --config
#      yaml is already committed in shop, so it lands in the worktree
#      automatically — no copy or init step needed.
#   5. cd into it and run, with $GH_OPTIVEM_CONFIG pointing at the chosen yaml:
#        <gh-optivem>/gh-optivem.exe implement --issue <issue-num> \
#            --log-file <worktree>.log
#      The log is written to a SIBLING of the worktree (not inside it) so
#      it survives the cleanup-prompt deletion as a postmortem record.
#      It captures the full audit trail (Debug + every other level) plus
#      the trace stream — terminal verbosity flags do not filter the file.
#   5. On exit (success, failure, or interrupt), prompt the user to delete
#      the worktree + branch (default: yes). The .log file is always kept.
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

# Convert a POSIX path to the host's native style for display. On
# Git Bash / MSYS2, `pwd` returns /c/... which Windows terminals and
# editors can't resolve as clickable links; cygpath -w produces
# C:\... that they can. On Linux/macOS cygpath is absent, so the
# raw POSIX path is already native — pass it through unchanged.
display_path() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$1"
  else
    printf '%s' "$1"
  fi
}

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
  echo "Usage: $0 <issue-num> [label] [--config <yaml>] [--auto] [--headless]" >&2
}

ISSUE=""
LABEL=""
CONFIG="$REHEARSAL_DEFAULT_CONFIG"
AUTO=0
HEADLESS=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      sed -n '12,62p' "$0" | sed 's/^# \{0,1\}//'
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
    --auto)
      AUTO=1
      shift
      ;;
    --headless)
      HEADLESS=1
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

# Build the rehearsal id: <ts>-<issue>-<slug>. Leading with the timestamp
# makes `rehearsal-*` dirs and `rehearsal/*` branches sort
# chronologically, so the newest run is always last and "latest
# rehearsal" is trivial to spot. <issue> still anchors the id to a
# specific ticket — grep `rehearsal-*-61-*` to find every run for issue
# #61 — and <slug> (the explicit [label] arg or the slugified issue
# title) carries human context. <ts> also keeps the id unique across
# same-ticket reruns.
TS="$(date +%Y%m%d-%H%M%S)"

# Extract numeric issue id from $ISSUE — accepts plain "61", "#61", or
# the full URL form gh issue view also takes. ${var##*/} strips a URL
# prefix; ${var##*#} strips a leading "#". For plain "61" both are
# no-ops.
ISSUE_NUM="${ISSUE##*/}"
ISSUE_NUM="${ISSUE_NUM##*#}"
if [[ ! "$ISSUE_NUM" =~ ^[0-9]+$ ]]; then
  echo "ERROR: could not extract numeric issue id from '$ISSUE'" >&2
  exit 2
fi

# Resolve <slug>: explicit [label] wins; otherwise fetch the issue title
# from the consumer repo via `gh issue view` and slugify it. The fetch
# is a hard prerequisite — failure (auth, network, wrong repo, typo'd
# issue number) aborts before any worktree is created, so a misnamed
# branch never reaches GitHub.
if [[ -n "$LABEL" ]]; then
  SLUG="$LABEL"
  TITLE=""
else
  log "Fetching issue title via gh issue view $ISSUE_NUM (from $REHEARSAL_REPO)..."
  if ! TITLE="$(cd "$CONSUMER_ROOT" && gh issue view "$ISSUE_NUM" --json title -q .title 2>&1)"; then
    echo "ERROR: gh issue view $ISSUE_NUM failed in $CONSUMER_ROOT:" >&2
    echo "$TITLE" >&2
    exit 1
  fi
  if [[ -z "$TITLE" ]]; then
    echo "ERROR: gh issue view $ISSUE_NUM returned an empty title" >&2
    exit 1
  fi
  # Slugify: lowercase, non-alphanumeric → "-", collapse runs, trim.
  SLUG="$(printf '%s' "$TITLE" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//')"
  # Cap at 40 chars on a word boundary: take first 40 chars, then strip
  # from the last "-" to the end. That drops the partial trailing word
  # (and any trailing dash from the truncation). If the slug has no "-"
  # in the first 40 chars (single very long word), ${slug%-*} is a
  # no-op and we keep the 40-char chunk as-is.
  if [[ ${#SLUG} -gt 40 ]]; then
    SLUG="${SLUG:0:40}"
    SLUG="${SLUG%-*}"
  fi
  if [[ -z "$SLUG" ]]; then
    echo "ERROR: issue title '$TITLE' slugified to empty string" >&2
    exit 1
  fi
fi

ID="${TS}-${ISSUE_NUM}-${SLUG}"
# Worktree lives under <academy>/worktrees/ — sibling of the consumer
# repo and the gh-optivem checkout. Safe inside the academy because the
# workspace resolver (internal/workspace.resolveFrom) walks up for a
# *.code-workspace file but then verifies CWD's git repo is one of its
# folders[] before committing to ModeWorkspace. The worktree's repo
# root is not declared in academy-workspace.code-workspace, so the
# membership check fails and the cascade falls through to ModeProject
# (gh-optivem.yaml inside the worktree) → ModeSingleRepo on the worktree
# itself. Grouping under `worktrees/` lets a single multi-root VS Code
# workspace surface every rehearsal without per-run setup.
WORKTREES_DIR="$(cd "$(dirname "$CONSUMER_ROOT")" && pwd)/worktrees"
mkdir -p "$WORKTREES_DIR"
WORKTREE_PATH="$WORKTREES_DIR/rehearsal-${ID}"
BRANCH="rehearsal/${ID}"
# Log lives next to the worktree, not inside it, so the cleanup prompt's
# `git worktree remove --force` does not nuke the postmortem record. The
# binary's --log-file captures every level (Debug → Fatal) plus the trace
# stream — strictly more than terminal output under any verbosity flag.
LOG_FILE="${WORKTREE_PATH}.log"

cleanup() {
  local rc=$?
  cd "$CONSUMER_ROOT"
  if [[ ! -d "$WORKTREE_PATH" ]]; then
    return $rc
  fi
  # REHEARSAL_CLEANUP overrides the interactive prompt so unattended callers
  # (e.g. atdd-rehearsal-loop.sh) never block on the EXIT trap:
  #   yes        → delete worktree+branch without asking
  #   no         → keep without asking
  #   on-success → delete only if the run exited 0; keep on failure so the
  #                broken worktree is left for inspection
  #   ask / unset → prompt (the default, interactive behaviour)
  local do_delete
  case "${REHEARSAL_CLEANUP:-ask}" in
    yes|y)      do_delete=0 ;;
    no|n)       do_delete=1 ;;
    on-success) if [[ $rc -eq 0 ]]; then do_delete=0; else do_delete=1; fi ;;
    *)          echo ""; if prompt_yn "Delete worktree $(display_path "$WORKTREE_PATH") and branch $BRANCH?"; then do_delete=0; else do_delete=1; fi ;;
  esac
  if [[ $do_delete -eq 0 ]]; then
    git -C "$CONSUMER_ROOT" worktree remove --force "$WORKTREE_PATH" || true
    git -C "$CONSUMER_ROOT" branch -D "$BRANCH" 2>/dev/null || true
    # Drop any stale .git/worktrees/* entries (e.g. if remove --force
    # failed partially, or the directory was wiped manually before the
    # prompt). Lingering metadata makes VS Code's git extension hang
    # refreshing Source Control for a path that no longer exists.
    git -C "$CONSUMER_ROOT" worktree prune 2>/dev/null || true
    log "Removed $(display_path "$WORKTREE_PATH") (branch $BRANCH)."
  else
    log "Keeping $(display_path "$WORKTREE_PATH") (branch $BRANCH)."
  fi
  return $rc
}

# Rehearsal-only configuration. The binary prints the consumer-facing
# bits (project URL, scope, etc.) once it's invoked; here we surface only
# the values the script itself materialises (worktree + branch) plus the
# fact that we're exercising a freshly-built binary out of GH_OPTIVEM_ROOT
# rather than whatever `gh optivem` is installed on PATH.
log "${C_BOLD}Rehearsal:${C_RESET}"
log "  issue:       #${ISSUE_NUM}"
if [[ -n "$TITLE" ]]; then
  log "  title:       $TITLE"
fi
if [[ -n "$LABEL" ]]; then
  log "  label:       $LABEL (override)"
fi
log "  worktree:    $(display_path "$WORKTREE_PATH")"
log "  branch:      $BRANCH"
log "  config:      $CONFIG"
log "  built from:  $(display_path "$GH_OPTIVEM_ROOT")"
log "  binary:      $(display_path "$BIN")"
log "  log file:    $(display_path "$LOG_FILE")"
if [[ $AUTO -eq 1 || $HEADLESS -eq 1 ]]; then
  MODE_BITS=""
  [[ $AUTO -eq 1 ]] && MODE_BITS="${MODE_BITS:+$MODE_BITS }--auto"
  [[ $HEADLESS -eq 1 ]] && MODE_BITS="${MODE_BITS:+$MODE_BITS }--headless"
  log "  mode flags:  $MODE_BITS"
fi

log "Installing gh-optivem (build + gh extension install)..."
if ! bash "$GH_OPTIVEM_ROOT/scripts/install.sh"; then
  log "install.sh failed — aborting before worktree."
  exit 1
fi

log "Cleaning local docker state (volumes + locally-built images; registry images preserved)..."
( cd "$CONSUMER_ROOT" && GH_OPTIVEM_CONFIG="$CONSUMER_ROOT/$CONFIG" "$BIN" system clean )

log "Creating worktree at $(display_path "$WORKTREE_PATH") on branch $BRANCH..."
if ! git -C "$CONSUMER_ROOT" worktree add -b "$BRANCH" "$WORKTREE_PATH"; then
  log "worktree add failed — aborting."
  exit 1
fi

# Trap installed *after* worktree creation: pre-worktree failures do not
# trigger the cleanup prompt.
trap cleanup EXIT

CONFIG_FULL="$WORKTREE_PATH/$CONFIG"
if [[ ! -f "$CONFIG_FULL" ]]; then
  log "ERROR: config file not found in worktree: $(display_path "$CONFIG_FULL")"
  log "Expected one of the gh-optivem-*.yaml variants committed in $REHEARSAL_REPO."
  exit 2
fi

# --auto is a root flag (before `implement`); --headless and --log-file
# are implement subcommand flags (after). Assemble two arrays so each
# lands in the right position when expanded.
ROOT_FLAGS=()
IMPL_FLAGS=(--log-file "$LOG_FILE")
[[ $AUTO -eq 1 ]] && ROOT_FLAGS+=(--auto)
[[ $HEADLESS -eq 1 ]] && IMPL_FLAGS+=(--headless)

log "Running implement --issue $ISSUE${IMPL_FLAGS[*]:+ ${IMPL_FLAGS[*]}}${ROOT_FLAGS[*]:+ (with root flags: ${ROOT_FLAGS[*]})} in $(display_path "$WORKTREE_PATH")..."

# Exit code the `implement` BINARY yields when it reaches a category:human
# approval gate on an unattended (no-TTY) run: a normal, expected pause that
# yields cleanly — NOT a crash. Mirrors ExitCodePendingHuman in
# internal/atdd/runtime/driver/driver.go (keep the two in sync).
EXIT_PENDING_HUMAN=2
# Exit code THIS WRAPPER yields upward for that pause. Deliberately distinct
# from the binary's 2: the wrapper already uses `exit 2` for its own usage /
# config errors (config-not-found, bad args — see the validation blocks
# above), so re-mapping the pause to a dedicated code lets the loop tell a
# real "awaiting human" pause apart from a wrapper error. Kept in sync with
# WRAPPER_EXIT_PENDING_HUMAN in atdd-rehearsal-loop.sh.
WRAPPER_EXIT_PENDING_HUMAN=32

RC=0
( cd "$WORKTREE_PATH" && GH_OPTIVEM_CONFIG="$CONFIG_FULL" "$BIN" "${ROOT_FLAGS[@]}" implement --issue "$ISSUE" --log-file "$LOG_FILE" "${IMPL_FLAGS[@]}" ) || RC=$?

if [[ $RC -eq 0 ]]; then
  VERDICT="$(grep -oE 'verdict=[a-z-]+' "$LOG_FILE" | tail -1)"
  log "implement finished (rc=0, ${VERDICT:-verdict=unknown}). See trace for test outcome."
  EXIT_RC=0
elif [[ $RC -eq $EXIT_PENDING_HUMAN ]]; then
  log "implement paused (rc=$RC): reached a human-approval gate with no operator TTY — did not complete; expected for an unattended rehearsal. Resume with an operator present. See trace."
  EXIT_RC=$WRAPPER_EXIT_PENDING_HUMAN
else
  log "implement crashed (rc=$RC). Runtime error, not a test failure — see trace."
  EXIT_RC=$RC
fi
log "Log file: $(display_path "$LOG_FILE")"

exit $EXIT_RC

} && exit 0

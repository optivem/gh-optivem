#!/usr/bin/env bash
set -euo pipefail

# Wrap body in `{ ... } && exit` so bash parses the entire script up-front.
# Matches atdd-rehearsal.sh's preamble — keeps the two scripts in lockstep
# so edits during a long run never desync bash's file offset.
{

# Recover from force-cancelled `atdd-rehearsal.sh` runs by cleaning up
# the worktree dirs, branches, and stale `.git/worktrees/` metadata it
# would normally drop in its EXIT trap. Personal dev tool — pairs with
# `gh optivem doctor --orphans`, which handles the binary-side process
# cleanup; this script handles the filesystem + git artefacts that only
# the rehearsal wrapper creates.
#
# Usage:
#   bash atdd-rehearsal-cleanup.sh                   # dry run: list what would be deleted
#   bash atdd-rehearsal-cleanup.sh --delete          # actually delete, then chain to
#                                                     gh optivem doctor --orphans
#   bash atdd-rehearsal-cleanup.sh --delete --force  # ALSO (a) delete skipped branches that
#                                                     have commits ahead of main, and (b)
#                                                     remove leftover *registered* rehearsal
#                                                     worktrees + their branches (each behind
#                                                     its own y/n confirmation, lists subjects
#                                                     first). NOT safe during a live rehearsal
#                                                     — see the safety note below.
#   bash atdd-rehearsal-cleanup.sh --delete --logs   # ALSO delete the rehearsal-*.log
#                                                     postmortem files under worktrees/
#                                                     (combine with --force for a full reset)
#   bash atdd-rehearsal-cleanup.sh --help            # show this help
#
# Workflow:
#   1. Resolve <WORKTREES_DIR> exactly as atdd-rehearsal.sh does (sibling
#      `worktrees/` of the consumer repo named per REHEARSAL_REPO).
#   2. List `rehearsal-*` directories under <WORKTREES_DIR>.
#   3. Cross-reference with `git -C <consumer-repo> worktree list
#      --porcelain`. Any directory NOT in the registered list is an orphan
#      dir. Any directory that IS registered AND lives under <WORKTREES_DIR>
#      with a `rehearsal-*` name is a leftover *registered* worktree from a
#      force-cancelled run whose EXIT trap never fired `git worktree remove`
#      — preserved by default (a live rehearsal's worktree is registered
#      too), removed only under --force (see step 6).
#   4. List `rehearsal/*` branches via `git -C <consumer-repo> branch
#      --list 'rehearsal/*'`. Any branch NOT checked out by a registered
#      worktree is an orphan candidate. Branches with commits ahead of
#      main are SKIPPED (printed as "has commits") so the operator can
#      investigate manually — mirrors the safety stance in
#      cleanup-orphans.sh:382-399. Pass --force to opt those skipped
#      branches back in as force-delete candidates (see step 6).
#   5. Print summary (orphan dir count, orphan branch count, skipped
#      count with reasons).
#   6. Default mode is --dry-run — prints what WOULD happen and exits 0
#      without touching anything. Pass --delete to do the real work:
#      prompt y/n, then `rm -rf` orphan dirs, `git branch -D` orphan
#      branches, `git worktree prune` to drop stale `.git/worktrees/<id>`
#      metadata, and finally `exec gh optivem doctor --orphans` to chain
#      into the binary-side process cleanup. Adding --force to --delete
#      ALSO (a) force-deletes the skipped (commits-ahead) orphan branches
#      and (b) removes the leftover registered rehearsal worktrees from
#      step 3 (`git worktree remove --force` + `git branch -D`), each
#      behind its OWN separate y/n prompt that first prints the latest
#      commit subject so unsynced work is visible before it is dropped.
#      Adding --logs ALSO deletes the rehearsal-*.log postmortem files
#      under <WORKTREES_DIR>, behind their own y/n prompt.
#
# Safe to run with a live rehearsal in progress IN THE DEFAULT (and
# --logs) MODE: the cross-reference against `git worktree list` means any
# worktree currently registered (i.e. currently active under
# atdd-rehearsal.sh) is preserved, and the chained doctor sweep classifies
# its claude.exe as "parent alive, skip" rather than prompting to kill.
#
# --force BREAKS this guarantee: it removes ALL registered rehearsal
# worktrees, including one a live rehearsal is mid-run on. Only pass
# --force when no rehearsal is active.
#
# The doctor chain at the end uses `gh optivem` resolved from PATH (the
# installed binary), NOT the rehearsal's freshly-built `gh-optivem.exe`
# which lives in a sibling repo and is scoped to that rehearsal run.

# === REHEARSAL CONFIG === (keep aligned with atdd-rehearsal.sh)
REHEARSAL_REPO="shop"
# === END REHEARSAL CONFIG ===

GH_OPTIVEM_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [[ -t 1 ]]; then
  C_BOLD=$'\033[1m'; C_CYAN=$'\033[36m'; C_DIM=$'\033[2m'; C_RESET=$'\033[0m'
else
  C_BOLD=''; C_CYAN=''; C_DIM=''; C_RESET=''
fi
PREFIX="${C_CYAN}[atdd-rehearsal-cleanup]${C_RESET}"
log() { echo "${PREFIX} $*"; }

# prompt_yn <prompt>
# Same shape as atdd-rehearsal.sh:88, which mirrors internal/promptio.ConfirmYN:
# explicit y/n required, no Enter shortcut, loops on unrecognised input,
# returns 0 for yes / 1 for no (or EOF).
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
  sed -n '9,78p' "$0" | sed 's/^# \{0,1\}//'
}

DRY_RUN=1
FORCE=0
DELETE_LOGS=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --delete)
      DRY_RUN=0
      shift
      ;;
    --force)
      FORCE=1
      shift
      ;;
    --logs)
      DELETE_LOGS=1
      shift
      ;;
    *)
      echo "ERROR: unknown argument: $1" >&2
      echo "Usage: $0 [--dry-run | --delete] [--force] [--logs]" >&2
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

# Resolve WORKTREES_DIR exactly the way atdd-rehearsal.sh:207 does so the
# two scripts stay in lockstep. Sibling `worktrees/` of the consumer repo.
WORKTREES_DIR="$(cd "$(dirname "$CONSUMER_ROOT")" && pwd)/worktrees"

if [[ $DRY_RUN -eq 1 ]]; then
  log "${C_BOLD}Mode:${C_RESET} dry-run (pass --delete to actually delete)"
else
  log "${C_BOLD}Mode:${C_RESET} DELETE"
fi
log "  consumer:    $CONSUMER_ROOT"
log "  worktrees:   $WORKTREES_DIR"

# --- Discover registered worktrees ---
# `git worktree list --porcelain` prints `worktree <path>` entries; we
# only need the paths to subtract from the on-disk set. Normalise via
# `cd && pwd` so trailing-slash / case differences (Windows) do not
# defeat the equality check.
registered_paths=()
if [[ -d "$CONSUMER_ROOT/.git" ]]; then
  while IFS= read -r line; do
    if [[ "$line" =~ ^worktree[[:space:]]+(.+)$ ]]; then
      wp="${BASH_REMATCH[1]}"
      if [[ -d "$wp" ]]; then
        registered_paths+=("$(cd "$wp" && pwd)")
      else
        registered_paths+=("$wp")
      fi
    fi
  done < <(git -C "$CONSUMER_ROOT" worktree list --porcelain)
fi

is_registered() {
  local needle="$1"
  local p
  for p in "${registered_paths[@]+"${registered_paths[@]}"}"; do
    if [[ "$p" == "$needle" ]]; then
      return 0
    fi
  done
  return 1
}

# Resolve the comparison base for "commits ahead" counts once. Prefer
# origin/main (local main may be stale); fall back to local main; empty
# if neither exists (then everything counts as 0 ahead).
MAIN_BASE=""
if git -C "$CONSUMER_ROOT" rev-parse --verify --quiet origin/main >/dev/null; then
  MAIN_BASE="origin/main"
elif git -C "$CONSUMER_ROOT" rev-parse --verify --quiet main >/dev/null; then
  MAIN_BASE="main"
fi

# ahead_count <branch> — commits on <branch> not on MAIN_BASE (0 if no base).
ahead_count() {
  local branch="$1"
  if [[ -n "$MAIN_BASE" ]]; then
    git -C "$CONSUMER_ROOT" rev-list --count "$MAIN_BASE..$branch" 2>/dev/null || echo 0
  else
    echo 0
  fi
}

# --- Discover orphan worktree dirs ---
orphan_dirs=()
if [[ -d "$WORKTREES_DIR" ]]; then
  for dir in "$WORKTREES_DIR"/rehearsal-*/; do
    [[ -d "$dir" ]] || continue
    dir="${dir%/}"
    abs="$(cd "$dir" && pwd)"
    if ! is_registered "$abs"; then
      orphan_dirs+=("$abs")
    fi
  done
fi

# --- Discover orphan branches ---
# A rehearsal branch is orphan iff (a) it is NOT checked out by a
# registered worktree, AND (b) it has no commits ahead of main. The
# (a) condition matches atdd-rehearsal.sh's pairing of one worktree to
# one branch; the (b) condition mirrors cleanup-orphans.sh:382-399 —
# refuse to delete branches with unsynced work, surface them as skipped.
registered_branches=()
while IFS= read -r line; do
  if [[ "$line" =~ ^branch[[:space:]]+refs/heads/(.+)$ ]]; then
    registered_branches+=("${BASH_REMATCH[1]}")
  fi
done < <(git -C "$CONSUMER_ROOT" worktree list --porcelain)

is_registered_branch() {
  local needle="$1"
  local b
  for b in "${registered_branches[@]+"${registered_branches[@]}"}"; do
    if [[ "$b" == "$needle" ]]; then
      return 0
    fi
  done
  return 1
}

orphan_branches=()
skipped_branches=()
skipped_branch_names=()
while IFS= read -r br; do
  br="${br#  }"
  br="${br#\* }"
  br="$(echo "$br" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  [[ -z "$br" ]] && continue
  if is_registered_branch "$br"; then
    continue
  fi
  # Count commits on the branch that are NOT on main. A branch fully
  # merged into main counts 0 ahead and is an orphan; any commits ahead
  # mean unsynced work, so it is SKIPPED (mirrors cleanup-orphans.sh:382-399).
  ahead="$(ahead_count "$br")"
  if [[ "$ahead" != "0" ]]; then
    skipped_branches+=("$br (has $ahead commit(s) ahead of ${MAIN_BASE:-main})")
    skipped_branch_names+=("$br")
    continue
  fi
  orphan_branches+=("$br")
done < <(git -C "$CONSUMER_ROOT" branch --list 'rehearsal/*' --format='%(refname:short)')

# --- Discover registered rehearsal worktrees ---
# A registered worktree whose path lives under WORKTREES_DIR with a
# `rehearsal-*` basename is a leftover from a force-cancelled run: the
# EXIT trap that would have run `git worktree remove --force` never
# fired, so git still tracks both the dir and its branch. These fall
# through every other category — the orphan-dir scan skips registered
# paths, and is_registered_branch skips their branches. Preserved by
# default (a live rehearsal's worktree is registered too); only --force
# opts them into removal. Parse path+branch as a pair by tracking the
# current `worktree` line until its `branch` line arrives.
reg_wt_paths=()
reg_wt_branches=()
cur_path=""
while IFS= read -r line; do
  if [[ "$line" =~ ^worktree[[:space:]]+(.+)$ ]]; then
    cur_path="${BASH_REMATCH[1]}"
    if [[ -d "$cur_path" ]]; then
      cur_path="$(cd "$cur_path" && pwd)"
    fi
  elif [[ "$line" =~ ^branch[[:space:]]+refs/heads/(.+)$ ]]; then
    cur_br="${BASH_REMATCH[1]}"
    if [[ "$cur_path" == "$WORKTREES_DIR"/* && "$(basename "$cur_path")" == rehearsal-* ]]; then
      reg_wt_paths+=("$cur_path")
      reg_wt_branches+=("$cur_br")
    fi
  elif [[ -z "$line" ]]; then
    cur_path=""
  fi
done < <(git -C "$CONSUMER_ROOT" worktree list --porcelain)

# --- Discover rehearsal .log files (only relevant under --logs) ---
log_files=()
if [[ -d "$WORKTREES_DIR" ]]; then
  for f in "$WORKTREES_DIR"/rehearsal-*.log; do
    [[ -f "$f" ]] || continue
    log_files+=("$f")
  done
fi

# --- Summary ---
echo ""
log "${C_BOLD}Summary:${C_RESET}"
log "  orphan worktree dirs:        ${#orphan_dirs[@]}"
log "  orphan branches:             ${#orphan_branches[@]}"
log "  skipped branches:            ${#skipped_branches[@]}"
log "  registered rehearsal worktrees: ${#reg_wt_paths[@]}"
if [[ $DELETE_LOGS -eq 1 ]]; then
  log "  rehearsal .log files:        ${#log_files[@]}"
fi

if [[ ${#orphan_dirs[@]} -gt 0 ]]; then
  echo ""
  log "${C_BOLD}Orphan worktree dirs${C_RESET} (NOT registered with git worktree list):"
  for d in "${orphan_dirs[@]}"; do
    echo "  - $d"
  done
fi

if [[ ${#orphan_branches[@]} -gt 0 ]]; then
  echo ""
  log "${C_BOLD}Orphan branches${C_RESET} (no registered worktree, no commits ahead of main):"
  for b in "${orphan_branches[@]}"; do
    echo "  - $b"
  done
fi

if [[ ${#skipped_branches[@]} -gt 0 ]]; then
  echo ""
  if [[ $FORCE -eq 1 ]]; then
    log "${C_BOLD}Skipped${C_RESET} (commits ahead of main — ${C_BOLD}--force${C_RESET} will delete these):"
  else
    log "${C_BOLD}Skipped${C_RESET} (will NOT be touched — investigate manually, or pass --force):"
  fi
  for b in "${skipped_branches[@]}"; do
    echo "  - $b"
  done
fi

if [[ ${#reg_wt_paths[@]} -gt 0 ]]; then
  echo ""
  if [[ $FORCE -eq 1 ]]; then
    log "${C_BOLD}Registered rehearsal worktrees${C_RESET} (leftover from crashed runs — ${C_BOLD}--force${C_RESET} will remove these + their branches):"
  else
    log "${C_BOLD}Registered rehearsal worktrees${C_RESET} (will NOT be touched — may be a live rehearsal; pass --force to remove):"
  fi
  for i in "${!reg_wt_paths[@]}"; do
    echo "  - ${reg_wt_paths[$i]}"
    echo "      ${C_DIM}↳ branch ${reg_wt_branches[$i]} ($(ahead_count "${reg_wt_branches[$i]}") commit(s) ahead)${C_RESET}"
  done
fi

# Anything actionable? Orphan dirs/branches always count; skipped branches
# and registered worktrees only count when --force opts them back in; log
# files only count under --logs.
have_force_targets=0
if [[ $FORCE -eq 1 && ${#skipped_branch_names[@]} -gt 0 ]]; then
  have_force_targets=1
fi
if [[ $FORCE -eq 1 && ${#reg_wt_paths[@]} -gt 0 ]]; then
  have_force_targets=1
fi

have_log_targets=0
if [[ $DELETE_LOGS -eq 1 && ${#log_files[@]} -gt 0 ]]; then
  have_log_targets=1
fi

if [[ ${#orphan_dirs[@]} -eq 0 && ${#orphan_branches[@]} -eq 0 && $have_force_targets -eq 0 && $have_log_targets -eq 0 ]]; then
  echo ""
  log "${C_DIM}Nothing to delete on the script side.${C_RESET}"
  if [[ $DRY_RUN -eq 0 ]]; then
    log "Chaining to gh optivem doctor --orphans for process cleanup..."
    echo ""
    exec gh optivem doctor --orphans
  fi
  exit 0
fi

if [[ $DRY_RUN -eq 1 ]]; then
  echo ""
  # Suggest the flag combo that would act on everything listed above.
  rerun="--delete"
  if [[ ${#skipped_branch_names[@]} -gt 0 || ${#reg_wt_paths[@]} -gt 0 ]]; then
    rerun="$rerun --force"
  fi
  if [[ ${#log_files[@]} -gt 0 ]]; then
    rerun="$rerun --logs"
  fi
  log "${C_DIM}Dry run — nothing was deleted. Re-run with ${rerun} to act on the above.${C_RESET}"
  exit 0
fi

# --- Delete orphans (interactive) ---
if [[ ${#orphan_dirs[@]} -gt 0 || ${#orphan_branches[@]} -gt 0 ]]; then
  echo ""
  if prompt_yn "Delete ${#orphan_dirs[@]} orphan worktree dir(s) and ${#orphan_branches[@]} orphan branch(es)?"; then
    for d in "${orphan_dirs[@]+"${orphan_dirs[@]}"}"; do
      log "Removing $d"
      if ! rm -rf "$d"; then
        log "  ! rm -rf failed for $d (process likely still holds handles — run gh optivem doctor --orphans first)"
      fi
    done

    for b in "${orphan_branches[@]+"${orphan_branches[@]}"}"; do
      log "git branch -D $b"
      git -C "$CONSUMER_ROOT" branch -D "$b" 2>/dev/null || log "  ! branch -D failed for $b"
    done
  else
    log "Skipping orphan deletion."
    # Preserve original abort-then-exit unless --force or --logs still has work.
    if [[ $have_force_targets -eq 0 && $have_log_targets -eq 0 ]]; then
      log "Aborted by user."
      exit 0
    fi
  fi
fi

# --- Force-delete skipped (commits-ahead) orphan branches ---
# Behind a SECOND, explicit prompt. Each branch's latest commit subject is
# printed first so any unsynced work is visible before it is dropped.
if [[ $FORCE -eq 1 && ${#skipped_branch_names[@]} -gt 0 ]]; then
  echo ""
  log "${C_BOLD}--force:${C_RESET} the following ${#skipped_branch_names[@]} branch(es) have commits ahead of main:"
  for b in "${skipped_branch_names[@]}"; do
    subj="$(git -C "$CONSUMER_ROOT" log -1 --format='%h %s' "$b" 2>/dev/null || echo '(could not read tip)')"
    echo "  - $b"
    echo "      ${C_DIM}↳ $subj${C_RESET}"
  done
  echo ""
  if prompt_yn "FORCE-delete these ${#skipped_branch_names[@]} branch(es) WITH unsynced commits? Cannot be undone."; then
    for b in "${skipped_branch_names[@]}"; do
      log "git branch -D $b"
      git -C "$CONSUMER_ROOT" branch -D "$b" 2>/dev/null || log "  ! branch -D failed for $b"
    done
  else
    log "Skipping force-delete."
  fi
fi

# --- Force-remove registered (leftover) rehearsal worktrees ---
# Behind its OWN explicit prompt. Each worktree's branch + commits-ahead +
# latest commit subject is printed first. `git worktree remove --force`
# drops both the dir and the `.git/worktrees/<id>` metadata; the branch is
# then force-deleted. Only reached under --force (see safety note in header).
if [[ $FORCE -eq 1 && ${#reg_wt_paths[@]} -gt 0 ]]; then
  echo ""
  log "${C_BOLD}--force:${C_RESET} the following ${#reg_wt_paths[@]} registered rehearsal worktree(s) will be removed (worktree + branch):"
  for i in "${!reg_wt_paths[@]}"; do
    b="${reg_wt_branches[$i]}"
    subj="$(git -C "$CONSUMER_ROOT" log -1 --format='%h %s' "$b" 2>/dev/null || echo '(could not read tip)')"
    echo "  - ${reg_wt_paths[$i]}"
    echo "      ${C_DIM}↳ branch $b ($(ahead_count "$b") commit(s) ahead) — $subj${C_RESET}"
  done
  echo ""
  if prompt_yn "Remove these ${#reg_wt_paths[@]} registered worktree(s) and FORCE-delete their branch(es)? Cannot be undone."; then
    for i in "${!reg_wt_paths[@]}"; do
      p="${reg_wt_paths[$i]}"
      b="${reg_wt_branches[$i]}"
      log "git worktree remove --force $p"
      git -C "$CONSUMER_ROOT" worktree remove --force "$p" 2>/dev/null || log "  ! worktree remove failed for $p"
      log "git branch -D $b"
      git -C "$CONSUMER_ROOT" branch -D "$b" 2>/dev/null || log "  ! branch -D failed for $b"
    done
  else
    log "Skipping registered-worktree removal."
  fi
fi

# --- Delete rehearsal .log postmortem files (--logs) ---
if [[ $DELETE_LOGS -eq 1 && ${#log_files[@]} -gt 0 ]]; then
  echo ""
  if prompt_yn "Delete ${#log_files[@]} rehearsal .log file(s) under $WORKTREES_DIR? Cannot be undone."; then
    for f in "${log_files[@]}"; do
      log "rm -f $(basename "$f")"
      rm -f "$f" || log "  ! rm -f failed for $f"
    done
  else
    log "Keeping .log files."
  fi
fi

log "git worktree prune"
git -C "$CONSUMER_ROOT" worktree prune 2>/dev/null || true

echo ""
log "Chaining to gh optivem doctor --orphans for process cleanup..."
echo ""
exec gh optivem doctor --orphans

} && exit 0

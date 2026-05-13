#!/usr/bin/env bash
set -euo pipefail

# Wrap body in `{ ... } && exit` so bash parses the entire script up-front.
# Without this, bash maintains a file offset and re-reads from disk between
# commands; if the file is edited during a long-running command (e.g. the
# 60+ min `go run . init`), bash's offset desyncs and emits phantom syntax
# errors after the long command returns.
{

# Runs a manual scaffold against a randomly-named repo and, on success,
# deletes the created GitHub repos + SonarCloud projects via
# scripts/cleanup-orphans.sh. The local scaffold dir is deleted by
# `gh optivem init` itself (its default).
#
# Two-phase scaffolding: writes gh-optivem.yaml via `config init` first,
# then runs `init` against it. Project-stable flags (--system-name,
# --arch, langs, paths, project-url, etc.) go to `config init`;
# per-invocation flags (--keep-local, --report-bug, --verbose, --quiet,
# --log-file, --workdir, --shop-ref, --verify-level, --yes, the --no-*
# set) go to `init`. The split is hardcoded — passing an unknown
# flag is an error.
#
# On failure: nothing is deleted, so the scaffold dir + remote repos
# stay around for debugging.
#
# Usage:
#   bash scripts/manual-test.sh --owner <user> --system-name "Page Turner" \
#       --arch monolith --repo-strategy monorepo --monolith-lang java \
#       --project-url https://github.com/orgs/<user>/projects/N
#
#   bash scripts/manual-test.sh --no-cleanup --owner <user> ...
#
# The script supplies --repo (random name) and tier-path defaults; --no-cleanup
# is translated to --keep-local on init. Do not pass --repo or --keep-local
# yourself — they will conflict.
#
# Orphan cleanup if the script is killed mid-run:
#   bash scripts/cleanup-orphans.sh --owner <user> --repos --sonar \
#       --prefixes "manual-test-" --delete

cd "$(git rev-parse --show-toplevel)"

# Visual markers so manual-test output is distinguishable from a real `gh optivem init` run.
# Colors auto-disable when stdout is not a TTY (CI logs, pipes, redirects).
if [[ -t 1 ]]; then
  C_BOLD=$'\033[1m'; C_YELLOW=$'\033[33m'; C_CYAN=$'\033[36m'; C_RED=$'\033[31m'; C_RESET=$'\033[0m'
else
  C_BOLD=''; C_YELLOW=''; C_CYAN=''; C_RED=''; C_RESET=''
fi
PREFIX="${C_CYAN}[manual-test]${C_RESET}"
log()    { echo "${PREFIX} $*"; }
banner() {
  local color="$1" msg="$2"
  echo "${C_BOLD}${color}========================================================================${C_RESET}"
  echo "${C_BOLD}${color}  ${msg}${C_RESET}"
  echo "${C_BOLD}${color}========================================================================${C_RESET}"
}

log "Building binary..."
# Explicit check: the outer `{ ... } && exit 0` wrapper disables `set -e` inside
# the braces (bash suppresses errexit on the left side of &&), so a bare
# `go build` failure would not abort the script.
if ! go build -o gh-optivem.exe .; then
  log "Build failed — aborting before scaffold."
  exit 1
fi

NO_CLEANUP=0
OWNER=""
# Flags routed to `config init` (project-stable, written into gh-optivem.yaml).
CONFIG_INIT_FLAGS=()
# Flags routed to `init` (per-invocation only).
INIT_FLAGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-cleanup) NO_CLEANUP=1; shift ;;
    -h|--help)    sed -n '4,33p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    # Project-stable flags → config init
    --owner)             OWNER="$2"; CONFIG_INIT_FLAGS+=("$1" "$2"); shift 2 ;;
    --system-name)       CONFIG_INIT_FLAGS+=("$1" "$2"); shift 2 ;;
    --arch)              CONFIG_INIT_FLAGS+=("$1" "$2"); shift 2 ;;
    --repo-strategy)     CONFIG_INIT_FLAGS+=("$1" "$2"); shift 2 ;;
    --monolith-lang|--backend-lang|--frontend-lang|--test-lang) CONFIG_INIT_FLAGS+=("$1" "$2"); shift 2 ;;
    --license|--deploy)  CONFIG_INIT_FLAGS+=("$1" "$2"); shift 2 ;;
    --project-url)       CONFIG_INIT_FLAGS+=("$1" "$2"); shift 2 ;;
    # Tier-path flags are forwarded as-passed; the binary defaults
    # empty paths to the flat scaffold layout itself.
    --system-path|--system-test-path|--backend-path|--frontend-path|--stubs-path|--simulators-path)
                         CONFIG_INIT_FLAGS+=("$1" "$2"); shift 2 ;;
    # Per-invocation flags → init
    --verbose|-v|--quiet|-q|--yes|-y|--no-legacy|--no-local-tests|--no-local-sonar|--no-atdd|--no-project|--report-bug|--keep-local)
                         INIT_FLAGS+=("$1"); shift ;;
    --verify-level|--workdir|--shop-ref|--log-file) INIT_FLAGS+=("$1" "$2"); shift 2 ;;
    *) echo "ERROR: unknown flag $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$OWNER" ]]; then
  echo "ERROR: --owner is required" >&2
  exit 1
fi

if command -v openssl >/dev/null 2>&1; then
  SUFFIX=$(openssl rand -hex 8)
else
  SUFFIX=$(printf '%04x%04x%04x%04x' "$RANDOM" "$RANDOM" "$RANDOM" "$RANDOM")
fi
REPO="manual-test-${SUFFIX}"

# Inject --repo into the config-init flag set. Tier paths are no longer
# defaulted here — the binary itself fills empties with the flat scaffold
# layout, so any --*-path the caller passed is already in CONFIG_INIT_FLAGS
# and any path they omitted will be defaulted downstream.
CONFIG_INIT_FLAGS+=(--repo "$REPO")

if [[ "$NO_CLEANUP" == "1" ]]; then
  INIT_FLAGS+=(--keep-local)
  CLEANUP_DESC="none (--no-cleanup: keep local dir + GitHub repos + Sonar projects)"
else
  CLEANUP_DESC="full (local dir deleted by init; GitHub repos + Sonar projects deleted after)"
fi

banner "${C_YELLOW}" "MANUAL TEST RUN — ${REPO}"
log "Manual test repo:   $REPO"
log "Cleanup on success: $CLEANUP_DESC"
echo ""

# Phase 1 — write gh-optivem.yaml into a per-test work dir so manual-test
# runs in parallel don't stomp on each other. The pre-built binary at
# ./gh-optivem.exe (line above) is invoked with an absolute path so it
# works from outside this repo's module dir.
BIN="$(pwd)/gh-optivem.exe"
WORKDIR=$(mktemp -d -t manual-test-XXXXXX)
log "Writing gh-optivem.yaml in $WORKDIR..."
if ! ( cd "$WORKDIR" && "$BIN" config init "${CONFIG_INIT_FLAGS[@]}" ); then
  echo ""
  log "config init failed — yaml not written."
  banner "${C_RED}" "MANUAL TEST FAILED — ${REPO}"
  exit 1
fi

# Phase 2 — scaffold. Run init from $WORKDIR so it picks up the yaml just
# written.
if ! ( cd "$WORKDIR" && "$BIN" init "${INIT_FLAGS[@]}" ); then
  echo ""
  log "Scaffold failed — leaving local dir + GitHub repos + Sonar projects intact for debugging."
  log "Clean up later with:"
  log "  bash scripts/cleanup-orphans.sh --owner $OWNER --repos --sonar --prefixes \"manual-test-\" --delete"
  banner "${C_RED}" "MANUAL TEST FAILED — ${REPO}"
  exit 1
fi

if [[ "$NO_CLEANUP" == "1" ]]; then
  echo ""
  log "Done. --no-cleanup: local dir, GitHub repos, and Sonar projects kept."
  banner "${C_YELLOW}" "MANUAL TEST DONE — ${REPO}"
  exit 0
fi

echo ""
log "Scaffold succeeded. Deleting GitHub repos + Sonar projects for $REPO..."
if bash scripts/cleanup-orphans.sh \
    --owner "$OWNER" \
    --repos --sonar \
    --prefixes "$REPO" \
    --delete; then
  banner "${C_YELLOW}" "MANUAL TEST DONE — ${REPO}"
else
  status=$?
  banner "${C_RED}" "MANUAL TEST CLEANUP FAILED — ${REPO}"
  exit $status
fi

} && exit 0

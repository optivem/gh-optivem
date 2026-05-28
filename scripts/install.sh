#!/usr/bin/env bash
set -euo pipefail

# Builds gh-optivem.exe from this working copy and ensures `gh optivem`
# resolves to it. Run it any time you edit CLI-side source (cmd wiring,
# atdd commands, etc.) — without rebuilding, `gh optivem …` keeps running
# the previously built binary and silently masks your changes (cobra
# falls through to help text for subcommands the old binary doesn't
# know about, and `>` redirects then clobber any file with that help).
#
# Idempotent: safe to re-run. The first install command handles the
# common case (extension already path-linked here). The fallback covers
# the "extension is installed from a different source" case (e.g. you
# previously ran `gh extension install optivem/gh-optivem`).
#
# Usage:
#   bash scripts/install.sh

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if [[ -t 1 ]]; then
  C_CYAN=$'\033[36m'; C_RED=$'\033[31m'; C_RESET=$'\033[0m'
else
  C_CYAN=''; C_RED=''; C_RESET=''
fi
log() { echo "${C_CYAN}[install]${C_RESET} $*"; }
die() { echo "${C_RED}[install] ERROR:${C_RESET} $*" >&2; exit 1; }

# Without this, `set -e` aborts silently when a step fails — e.g. `gh extension
# install` failing on an unauthenticated machine leaves the user staring at
# gh's "please run gh auth login" hint with no indication the script bailed.
trap 'rc=$?; die "aborted at line $LINENO (exit $rc). See output above."' ERR

command -v go >/dev/null 2>&1 || die "go not found on PATH."
command -v gh >/dev/null 2>&1 || die "gh CLI not found on PATH."
gh auth status >/dev/null 2>&1 || die "gh is not authenticated. Run 'gh auth login' (or set GH_TOKEN) and re-run."

SHA=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
if ! git diff --quiet 2>/dev/null || ! git diff --cached --quiet 2>/dev/null; then
  SHA="${SHA}-dirty"
fi
DEV_VERSION="dev-${SHA}"
log "go build -o gh-optivem.exe . (version=${DEV_VERSION})"
go build -ldflags "-X github.com/optivem/gh-optivem/internal/version.Version=${DEV_VERSION}" -o gh-optivem.exe .

log "gh extension install (remove first if already installed)"
gh extension remove optivem 2>/dev/null || true
gh extension install --force .

log "verifying"
gh optivem --version

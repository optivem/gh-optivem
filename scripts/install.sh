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
  C_CYAN=$'\033[36m'; C_RESET=$'\033[0m'
else
  C_CYAN=''; C_RESET=''
fi
log() { echo "${C_CYAN}[install]${C_RESET} $*"; }

log "go build -o gh-optivem.exe ."
go build -o gh-optivem.exe .

log "gh extension install (remove first if already installed)"
gh extension remove optivem 2>/dev/null || true
gh extension install --force .

log "verifying"
gh optivem --version

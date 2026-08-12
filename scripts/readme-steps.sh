#!/usr/bin/env bash
# Every command in README.md, in README order, on a brand-new system.
#
# Run this in a throwaway VM to confirm the documented setup path still works.
# Stops at the first failure — that is the one worth reading.
#
#   bash scripts/readme-steps.sh
#
# scripts/vm-create.ps1 builds the clean-room Hyper-V guest this is meant for,
# and prints the Copy-VMFile command that gets this file into it. The guest is
# Windows, so this runs under Git Bash there.
#
# The section headers below are the README's own headings, in its own order, so
# the two can be read side by side. If you add a command to the README, add it
# here under the same heading; if the two drift, this file is wrong.

set -euo pipefail

OWNER="valentinajemuovic"
REPO="readme-steps-$(date +%Y%m%d-%H%M%S)"


# Prerequisites ===============================================================

# GitHub CLI
gh --version
# gh auth login   # interactive — run once by hand, then re-run this script
gh auth status

# Claude Code
# Not present on a clean Windows install. Both lines are commented out for the
# same reason as `gh auth login` above: sign-in opens a browser, so it is a
# one-time manual step. Run them once by hand, then re-run this script.
#
#   powershell -NoProfile -Command "irm https://claude.ai/install.ps1 | iex"
#   claude   # signs in; needs a Pro/Max/Team/Enterprise plan — free has no Claude Code
#
# That is the native-Windows installer invoked through a PowerShell shim, since
# this script runs under Git Bash. claude.ai/install.sh is the macOS/Linux/WSL
# installer and is NOT the right one here.
claude --version


# Install =====================================================================

gh extension install optivem/gh-optivem
gh optivem --version


# Local environment setup =====================================================

# The README runs this first, before the tools below are installed, precisely
# so it reports everything missing in one pass. On a brand-new system it is
# therefore EXPECTED to fail here — hence `|| true`. The re-run at the end of
# "Environment variables" is the one that must pass.
gh optivem environment verify || true

# Bash
bash --version

# Go
go version

# actionlint
go install github.com/rhysd/actionlint/cmd/actionlint@v1
# `go install` drops the binary in $(go env GOPATH)/bin and never edits PATH —
# the Go installer only puts the toolchain itself there. On a brand-new system
# the next line fails with "command not found" without this. The README says
# the same under its actionlint bullet: "Add that directory to PATH yourself."
#
# cygpath because on Windows `go env GOPATH` answers in native form
# (C:\Users\you\go) and PATH here is colon-separated, so appending it raw splits
# at the drive letter into "C" and "\Users\you\go/bin" — two entries, neither a
# directory. cygpath -u yields /c/Users/you/go/bin, which survives the split.
# The VM guest is Windows, so this is the path that actually gets exercised.
if command -v cygpath >/dev/null 2>&1; then
    export PATH="$PATH:$(cygpath -u "$(go env GOPATH)")/bin"
else
    export PATH="$PATH:$(go env GOPATH)/bin"
fi
actionlint --version

# Docker
docker --version

# Your project's language toolchain
java -version
dotnet --version
npm --version


# Environment variables =======================================================

gh optivem environment show

# "Then re-run gh optivem environment verify — it live-checks each token
# against its provider, so it will now pass."
gh optivem environment verify


# Claude Code setup ===========================================================

gh optivem claude setup


# Generate your project =======================================================

gh optivem init --owner "$OWNER" --repo "$REPO" --system-name "Book Shop" --repo-strategy monorepo --arch multitier --backend-lang dotnet --frontend-lang typescript --test-lang java


# Clone project repository ====================================================

gh repo clone "$OWNER/$REPO"
cd "$REPO"


# Verify your project =========================================================

gh optivem system-test setup
gh optivem system start
gh optivem system-test run --sample
gh optivem system stop


# Implement a ticket ==========================================================
#
# Needs an issue with Gherkin acceptance criteria on the Project board, so it
# cannot run against the repo this script just created. Do it by hand:
#
#   gh optivem implement 56
#   gh optivem --auto implement 56 --headless


# Upgrade, Uninstall ==========================================================
#
# Not run — they would undo the install above:
#
#   gh extension upgrade optivem
#   gh extension remove optivem


echo "readme-steps: done — remember to delete $OWNER/$REPO"

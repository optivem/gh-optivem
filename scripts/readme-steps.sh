#!/usr/bin/env bash
# Every command in README.md, in README order, on a brand-new system.
#
# Run this in a throwaway VM to confirm the documented setup path still works.
# Stops at the first failure — that is the one worth reading.
#
#   bash scripts/readme-steps.sh
#
# scripts/vm-machine-create.ps1 builds the clean-room Hyper-V guest this is meant for,
# and scripts/vm-steps-copy.ps1 gets this file into it. The guest is Windows, so
# this runs under Git Bash there.
#
# The section headers below are the README's own headings, in its own order, so
# the two can be read side by side. If you add a command to the README, add it
# here under the same heading; if the two drift, this file is wrong. The one
# exception is "Machine setup", which has no README counterpart — see the note
# there.

set -euo pipefail

OWNER="valentinajemuovic"
REPO="readme-steps-$(date +%Y%m%d-%H%M%S)"


# Machine setup ===============================================================
#
# Not a README heading. The README's Prerequisites bullets link out to each
# tool's own install page rather than naming commands, so these are the commands
# for the Windows clean-room guest, in the order they have to happen.
#
# Nothing here is executed — do these by hand in the fresh guest; the
# Prerequisites checks below confirm they took. Steps marked [interactive] need
# you at the keyboard, answering prompts or signing in through a browser. The
# rest run unattended once started.
#
#   1. Git for Windows — supplies the Git Bash this script runs under, so run
#      this one from PowerShell or CMD:
#
#        winget install --id Git.Git -e --source winget
#
#   2. GitHub CLI:
#
#        winget install --id GitHub.cli --source winget
#
#   3. [interactive] GitHub CLI sign-in — prompts, then a browser:
#
#        gh auth login
#
#   4. Claude Code — the native-Windows installer, run through a PowerShell
#      shim because this file is Git Bash. claude.ai/install.sh is the
#      macOS/Linux/WSL installer and is NOT the right one here:
#
#        powershell -NoProfile -Command "irm https://claude.ai/install.ps1 | iex"
#
#   5. [interactive] Claude Code sign-in — opens a browser. Needs a Pro, Max,
#      Team, or Enterprise plan; the free Claude.ai plan has no Claude Code:
#
#        claude
#
# Open a fresh shell afterwards so the installers' PATH edits are visible, then
# run this script.


# Prerequisites ===============================================================
#
# Verification only — "Machine setup" above is what installs these.

# GitHub CLI
gh --version
gh auth status

# Claude Code
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

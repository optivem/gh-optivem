#!/usr/bin/env bash
# Every command in README.md, in README order, on a brand-new system.
#
# Run this in a throwaway VM to confirm the documented setup path still works.
# Stops at the first failure — that is the one worth reading.
#
#   bash scripts/readme-steps.sh
#
# It installs tools, signs you in, and creates a GitHub repo, so point it at a
# clean-room guest rather than your workstation. Two steps wait on the keyboard,
# so run it from a console, not a pipe.
#
# scripts/vm-machine-create.ps1 builds the clean-room Hyper-V guest this is meant
# for, scripts/vm-scripts-copy.ps1 gets this file into it, and
# scripts/readme-setup.ps1 installs the Git Bash it runs under once inside.
#
# The section headers below are the README's own headings, in its own order, so
# the two can be read side by side. If you add a command to the README, add it
# here under the same heading; if the two drift, this file is wrong. The one
# exception is "Machine setup", which has no README counterpart — see the note
# there.

set -euo pipefail


# Logging =====================================================================
#
# A run that fails does so in a VM console, where scrollback is the worst place
# to read it from, so everything is teed to a timestamped file next to this
# script: logs/<YYYYMMDD-HHMMSS>-readme-steps.log
#
# The two interactive steps are deliberately NOT captured. Piping their output
# takes away their terminal, and a raw-mode TUI - the Claude sign-in - will not
# run without one. Fds 3 and 4 keep a handle on the real console for them, so
# they draw on screen and skip the log. A browser sign-in is not worth capturing.

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
log_dir="${GH_OPTIVEM_LOG_DIR:-$script_dir/logs}"
mkdir -p "$log_dir"
log_file="$log_dir/$(date +%Y%m%d-%H%M%S)-readme-steps.log"

exec 3>&1 4>&2                          # the real console, kept for the TUIs
exec > >(tee -a "$log_file") 2>&1

# The verdict goes through tee FIRST, while it is still the destination, so it
# lands in order at the end of the log; only then are the fds restored, which
# closes tee's input and lets it flush and exit. Appending to the file directly
# instead raced tee and put the verdict at the TOP of the log.
#
# `wait` on tee is deliberately not used: it blocks until every fd feeding it is
# closed, and a script that hangs at exit in a VM console is worse than one that
# loses a trailing line.
finish() {
    rc=$?
    if [ "$rc" -eq 0 ]; then
        echo "readme-steps: done."
    else
        echo "readme-steps: FAILED (exit $rc)."
    fi
    exec 1>&3 2>&4
    if [ "$rc" -eq 0 ]; then
        echo "readme-steps: log written to $log_file"
    else
        echo "readme-steps: FAILED (exit $rc). Log: $log_file" >&2
    fi
}
trap finish EXIT

echo "readme-steps: logging to $log_file"


OWNER="valentinajemuovic"
REPO="readme-steps-$(date +%Y%m%d-%H%M%S)"


# Machine setup ===============================================================
#
# Not a README heading. The README's Prerequisites bullets link out to each
# tool's own install page rather than naming commands, so these are the commands
# for the Windows clean-room guest, in the order they have to happen.
#
# These run. Steps marked [interactive] need you at the keyboard — answering
# prompts or signing in through a browser — so start this script from a console,
# not a pipe. Each step is skipped when the tool is already there, so re-running
# after a fix costs nothing.
#
# Still NOT covered here: Go, Docker Desktop, Java, .NET, and Node. The
# "Local environment setup" section below verifies them but nothing installs
# them, so a clean guest stops at `go version`. Install those by hand first.

# Installers write PATH into the registry; this process keeps the copy it was
# started with, so a freshly installed tool stays "command not found" for the
# rest of the run. That is what the old "open a fresh shell afterwards" note was
# working around. Re-read the registry instead.
refresh_path() {
    local win
    win=$(powershell.exe -NoProfile -Command \
        '[Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [Environment]::GetEnvironmentVariable("Path","User")' \
        | tr -d '\r')
    if [ -z "$win" ]; then
        # Appending an empty list would put "" on PATH, which bash reads as the
        # current directory. Fail instead.
        echo "readme-steps: could not read PATH back from the registry." >&2
        exit 1
    fi
    # cygpath -up turns the semicolon-separated Windows list into the
    # colon-separated one bash needs, drive letters and all.
    PATH="$PATH:$(cygpath -up "$win")"
    export PATH
    hash -r   # bash caches lookups; without this the earlier miss sticks
}

require_tty() {
    if [ ! -t 0 ]; then
        echo "readme-steps: $1 needs the keyboard. Run this script from a console, not a pipe." >&2
        exit 1
    fi
}

# $1 probe command, $2 winget id, $3 label
winget_install() {
    if command -v "$1" >/dev/null 2>&1; then
        echo "machine setup: $3 already installed"
        return
    fi
    echo "machine setup: installing $3 ..."
    # Without the --accept-* flags winget stops on a licence prompt the first
    # time it sees a source or package - which on a fresh guest is always.
    winget install --id "$2" -e --source winget \
        --accept-source-agreements --accept-package-agreements
    refresh_path
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "machine setup: $3 installed but '$1' is still not on PATH." >&2
        exit 1
    fi
}

# 1. Git for Windows supplies the Git Bash this script runs under, so it cannot
#    install itself from in here. scripts/readme-setup.ps1 does it from
#    PowerShell, which is why that is the first thing you run in the guest.
if ! command -v git >/dev/null 2>&1; then
    echo "machine setup: no git. From PowerShell: powershell -ExecutionPolicy Bypass -File C:\\Users\\Public\\readme-setup.ps1" >&2
    exit 1
fi
git --version

# Windows 11 ships winget as App Installer, but a guest straight off the ISO
# sometimes has a version too old to run until the Store updates it.
if ! command -v winget >/dev/null 2>&1; then
    echo "machine setup: no winget. Open Microsoft Store, update 'App Installer', then re-run." >&2
    exit 1
fi

# 2. GitHub CLI
winget_install gh GitHub.cli "GitHub CLI"

# 3. [interactive] GitHub CLI sign-in — prompts, then a browser.
if gh auth status >/dev/null 2>&1; then
    echo "machine setup: gh already signed in"
else
    require_tty "gh auth login"
    gh auth login >&3 2>&4   # real console, not the log - see "Logging" above
fi

# 4. Claude Code — the native-Windows installer, run through a PowerShell shim
#    because this file is Git Bash. claude.ai/install.sh is the macOS/Linux/WSL
#    installer and is NOT the right one here.
if command -v claude >/dev/null 2>&1; then
    echo "machine setup: Claude Code already installed"
else
    echo "machine setup: installing Claude Code ..."
    powershell.exe -NoProfile -Command "irm https://claude.ai/install.ps1 | iex"
    refresh_path
    if ! command -v claude >/dev/null 2>&1; then
        echo "machine setup: Claude Code installed but 'claude' is still not on PATH." >&2
        exit 1
    fi
fi

# 5. [interactive] Claude Code sign-in — opens a browser. Needs a Pro, Max,
#    Team, or Enterprise plan; the free Claude.ai plan has no Claude Code.
#
#    ~/.claude.json gains an "oauthAccount" entry once sign-in completes, which
#    answers "is this machine signed in" without spending a request. On a clean
#    guest it is always absent.
if grep -q '"oauthAccount"' "$HOME/.claude.json" 2>/dev/null; then
    echo "machine setup: Claude Code already signed in"
else
    require_tty "the Claude Code sign-in"
    echo "machine setup: starting Claude Code — sign in, then type /exit to come back here."
    # Git Bash usually runs under mintty, which is not a Windows console; the
    # raw-mode TUI needs winpty in front of it. Git for Windows ships winpty.
    # >&3 2>&4 for the same reason: a TUI with a pipe for stdout will not draw.
    if command -v winpty >/dev/null 2>&1; then
        winpty claude >&3 2>&4
    else
        claude >&3 2>&4
    fi
fi


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


# The trap prints the verdict and the log path; this is the part it cannot know.
echo "readme-steps: remember to delete $OWNER/$REPO"

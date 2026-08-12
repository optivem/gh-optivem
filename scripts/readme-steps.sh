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

# Success is ASSERTED, never inferred. A trap on EXIT alone cannot tell "the
# script finished" from "the shell was killed": bash runs the EXIT trap on a
# fatal signal too, and $? is then the status of the last command that DID
# complete - 0, when that was a successful install. A run on 2026-08-12 died
# seconds after the GitHub CLI install and the log it left behind said "done."
#
# So `completed` is set on the last line of the script and nowhere else, and
# anything reaching the trap without it is a failure whatever $? claims.
completed=0
signal=''
current_step='startup'

# What is running right now, for the log and for the verdict. Without it the two
# interactive steps are invisible: they draw on the real console and never reach
# the log by design, so a run that dies in one leaves no trace of where it got to.
step() {
    current_step="$1"
    echo "readme-steps: [$1]"
}

# Untrapped, SIGINT would kill the shell with status 0 and the EXIT trap would
# call that a clean run - so an operator's Ctrl+C reported success. Naming the
# signal here also turns "it stopped" into "it was interrupted", which is the
# distinction worth having at 2am in a VM console.
on_signal() {
    signal="$1"
    exit "$2"   # into the EXIT trap below, with a status that says which
}
trap 'on_signal INT 130'  INT
trap 'on_signal TERM 143' TERM
trap 'on_signal HUP 129'  HUP

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
    ok=0
    if [ -n "$signal" ]; then
        verdict="readme-steps: INTERRUPTED by SIG$signal during [$current_step]. Nothing after that step ran."
    elif [ "$completed" -eq 1 ] && [ "$rc" -eq 0 ]; then
        ok=1
        verdict="readme-steps: done."
    else
        # A killed shell arrives here carrying the status of the last command
        # that succeeded. Never let that 0 pass for a verdict.
        if [ "$rc" -eq 0 ]; then rc=1; fi
        verdict="readme-steps: FAILED (exit $rc) during [$current_step]."
    fi
    echo "$verdict"
    exec 1>&3 2>&4
    if [ "$ok" -eq 1 ]; then
        echo "readme-steps: log written to $log_file"
    else
        echo "$verdict Log: $log_file" >&2
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
# not a pipe. Each step first asks the tool for its version and is skipped when
# it answers, so re-running after a fix costs nothing.
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

# Is this tool here, and does it run? Asked before every install, and again
# after, and the answer is written down either way - "not installed" is as much
# a result as "installed", and on a guest that is supposed to be clean it is the
# one that confirms the checkpoint was restored.
#
# The question is put to the tool itself rather than to `command -v`, which only
# reports that a file of that name sits on PATH - true of a half-finished
# install and of a shim whose target has gone, and it prints no version. Running
# the tool's own version command answers the stronger question and leaves the
# version in the log, which is where the next step's failure gets diagnosed.
#
# $1 label, $2.. the version command
probe_tool() {
    local label="$1"
    shift
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "machine setup: $label NOT installed"
        return 1
    fi
    local out
    if ! out=$("$@" 2>&1); then
        # On PATH but not runnable. Treated as absent so the caller reinstalls,
        # which is the likely fix - but said out loud, because "not installed"
        # would be a lie and the reinstall is then unexplained.
        echo "machine setup: $label is on PATH ($(command -v "$1")) but '$*' failed:" >&2
        printf '%s\n' "$out" >&2
        return 1
    fi
    echo "machine setup: $label installed - $(printf '%s\n' "$out" | head -n 1)"
    return 0
}

# $1 winget id, $2 label, $3.. version command
winget_install() {
    local id="$1" label="$2"
    shift 2
    if probe_tool "$label" "$@"; then
        return
    fi
    echo "machine setup: installing $label ..."
    # Without the --accept-* flags winget stops on a licence prompt the first
    # time it sees a source or package - which on a fresh guest is always.
    winget install --id "$id" -e --source winget \
        --accept-source-agreements --accept-package-agreements
    refresh_path
    if ! probe_tool "$label" "$@"; then
        echo "machine setup: $label was installed but '$*' still does not run." >&2
        exit 1
    fi
}

# 1. Git for Windows supplies the Git Bash this script runs under, so it cannot
#    install itself from in here. scripts/readme-setup.ps1 does it from
#    PowerShell, which is why that is the first thing you run in the guest.
step 'machine setup: git'
if ! probe_tool 'Git' git --version; then
    echo "machine setup: no git. From PowerShell: powershell -ExecutionPolicy Bypass -File C:\\Users\\Public\\readme-setup.ps1" >&2
    exit 1
fi

# Windows 11 ships winget as App Installer, but a guest straight off the ISO
# sometimes has a version too old to run until the Store updates it - which is
# why this asks winget for its version rather than just for its presence.
if ! probe_tool 'winget' winget --version; then
    echo "machine setup: no usable winget. Open Microsoft Store, update 'App Installer', then re-run." >&2
    exit 1
fi

# 2. GitHub CLI
step 'machine setup: GitHub CLI install'
winget_install GitHub.cli "GitHub CLI" gh --version

# 3. [interactive] GitHub CLI sign-in — prompts, then a browser.
step 'machine setup: GitHub CLI sign-in [interactive, off-log]'
if gh auth status >/dev/null 2>&1; then
    echo "machine setup: gh already signed in"
else
    require_tty "gh auth login"
    # Real console, not the log - see "Logging" above. gh draws its prompts on
    # stderr, so neither stream can be diverted here without breaking them; the
    # exit code is the only part of this step that can be written down, so it is.
    login_rc=0
    gh auth login >&3 2>&4 || login_rc=$?
    echo "machine setup: gh auth login exited $login_rc"
    if [ "$login_rc" -ne 0 ]; then exit "$login_rc"; fi
fi

# 4. Claude Code — the native-Windows installer, run through a PowerShell shim
#    because this file is Git Bash. claude.ai/install.sh is the macOS/Linux/WSL
#    installer and is NOT the right one here.
step 'machine setup: Claude Code install'
if ! probe_tool 'Claude Code' claude --version; then
    echo "machine setup: installing Claude Code ..."
    powershell.exe -NoProfile -Command "irm https://claude.ai/install.ps1 | iex"
    refresh_path
    if ! probe_tool 'Claude Code' claude --version; then
        echo "machine setup: Claude Code was installed but 'claude --version' still does not run." >&2
        exit 1
    fi
fi

# 5. [interactive] Claude Code sign-in — opens a browser. Needs a Pro, Max,
#    Team, or Enterprise plan; the free Claude.ai plan has no Claude Code.
#
#    ~/.claude.json gains an "oauthAccount" entry once sign-in completes, which
#    answers "is this machine signed in" without spending a request. On a clean
#    guest it is always absent.
step 'machine setup: Claude Code sign-in [interactive, off-log]'
if grep -q '"oauthAccount"' "$HOME/.claude.json" 2>/dev/null; then
    echo "machine setup: Claude Code already signed in"
else
    require_tty "the Claude Code sign-in"
    echo "machine setup: starting Claude Code — sign in, then type /exit to come back here."
    # Git Bash usually runs under mintty, which is not a Windows console; the
    # raw-mode TUI needs winpty in front of it. Git for Windows ships winpty.
    # >&3 2>&4 for the same reason: a TUI with a pipe for stdout will not draw.
    # As with the sign-in above, the exit code is all this step can log.
    claude_rc=0
    if command -v winpty >/dev/null 2>&1; then
        winpty claude >&3 2>&4 || claude_rc=$?
    else
        claude >&3 2>&4 || claude_rc=$?
    fi
    echo "machine setup: Claude Code exited $claude_rc"
    if [ "$claude_rc" -ne 0 ]; then exit "$claude_rc"; fi
fi


# Prerequisites ===============================================================
#
# Verification only — "Machine setup" above is what installs these.

step 'Prerequisites'

# GitHub CLI
gh --version
gh auth status

# Claude Code
claude --version


# Install =====================================================================

step 'Install'
gh extension install optivem/gh-optivem
gh optivem --version


# Local environment setup =====================================================

# The README runs this first, before the tools below are installed, precisely
# so it reports everything missing in one pass. On a brand-new system it is
# therefore EXPECTED to fail here — hence `|| true`. The re-run at the end of
# "Environment variables" is the one that must pass.
step 'Local environment setup'
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

step 'Environment variables'
gh optivem environment show

# "Then re-run gh optivem environment verify — it live-checks each token
# against its provider, so it will now pass."
gh optivem environment verify


# Claude Code setup ===========================================================

step 'Claude Code setup'
gh optivem claude setup


# Generate your project =======================================================

step 'Generate your project'
gh optivem init --owner "$OWNER" --repo "$REPO" --system-name "Book Shop" --repo-strategy monorepo --arch multitier --backend-lang dotnet --frontend-lang typescript --test-lang java


# Clone project repository ====================================================

step 'Clone project repository'
gh repo clone "$OWNER/$REPO"
cd "$REPO"


# Verify your project =========================================================

step 'Verify your project'
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

# The only assignment of `completed` in the file, and it has to stay last: it is
# what tells the EXIT trap this run reached the end under its own power rather
# than being killed on the way. Adding a step below this line hides it from the
# verdict - add it above.
completed=1

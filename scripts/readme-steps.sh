#!/usr/bin/env bash
# Every command in README.md, in README order, on a brand-new system.
#
# Run this in a throwaway VM to confirm the documented setup path still works.
# Stops at the first failure — that is the one worth reading.
#
#   bash scripts/readme-steps.sh
#
# It installs tools, signs you in, creates a GitHub repo, and finishes by
# walking a real ticket through the ATDD pipeline — so point it at a clean-room
# guest rather than your workstation, and expect it to take the best part of an
# hour and to spend Claude usage. Two steps wait on the keyboard, so run it from
# a console, not a pipe.
#
# scripts/vm-machine-create.ps1 builds the clean-room Hyper-V guest this is meant
# for, scripts/vm-scripts-copy.ps1 gets this file into it, and
# scripts/readme-setup.ps1 installs the Git Bash it runs under once inside.
#
# SHAPE
#
# The README is the specification, so it is also the table of contents:
#
#   - one `readme_<heading>` function per README heading, holding that section's
#     commands in the README's own order;
#   - one `ensure_<tool>_installed` per tool the README asks you to have, so
#     "what does this file do about Go" has one place to look;
#   - a "Run" block at the very bottom that is nothing but the call list. Read
#     it side by side with the README's contents — they are the same list.
#
# If you add a command to the README, add it to the function named after its
# heading; if a heading appears in one and not the other, this file is wrong.
# The one exception is machine_setup, which has no README counterpart — see the
# note there.

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

# The worked example the README's "Implement a ticket" section points at by
# name. Copied rather than reinvented: acceptance criteria written here would
# be this script's idea of the shape the agents expect, which is exactly the
# thing the walkthrough is supposed to be checking.
SOURCE_TICKET_REPO='optivem/shop'
SOURCE_TICKET=72

# The copy's number in the generated repo, set by copy_source_ticket, and the
# board it is filed on, set by add_ticket_to_project_board. Both feed the
# cleanup reminder at the end.
ticket_number=''
project_url=''


# Helpers =====================================================================

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

# Does winget's package database say this package is installed? A PATH-free
# answer, which is the whole point of asking it: `winget list --id <id> -e`
# exits 0 when the package is installed and 20
# (APPINSTALLER_CLI_ERROR_NO_APPLICATIONS_FOUND) when it is not, whatever this
# shell's PATH happens to contain.
#
# $1 winget id
package_installed() {
    winget list --id "$1" -e >/dev/null 2>&1
}

# $1 winget id, $2 label, $3.. version command
winget_install() {
    local id="$1" label="$2"
    shift 2
    if probe_tool "$label" "$@"; then
        return
    fi
    # probe_tool only sees this shell's PATH; the package database does not. Ask
    # it before concluding anything is missing, because installing a package that
    # is already there cannot succeed - winget answers "no upgrade applicable"
    # and under `set -e` that ends the run. On 2026-08-12 it did, on a guest whose
    # gh worked, and the log said "GitHub CLI NOT installed" one line above
    # winget's "Found an existing package already installed".
    #
    # Only for the not-on-PATH case: a tool that IS on PATH and does not run
    # still falls through to the install, which is the likely fix for it.
    if ! command -v "$1" >/dev/null 2>&1 && package_installed "$id"; then
        echo "machine setup: $label is installed ($id is in the winget package database) but not on this shell's PATH - skipping the install."
        return
    fi
    echo "machine setup: installing $label ..."
    # Without the --accept-* flags winget stops on a licence prompt the first
    # time it sees a source or package - which on a fresh guest is always.
    local rc=0
    winget install --id "$id" -e --source winget \
        --accept-source-agreements --accept-package-agreements || rc=$?
    echo "machine setup: winget install $id exited $rc"
    # That status is written down, not acted on. Several of winget's non-zero
    # codes leave the package installed, so the re-probe below is what decides -
    # the file's own "Success is ASSERTED, never inferred" rule, applied to
    # installs. The status still reaches the log, and the error message, so a
    # genuine failure names its cause.
    refresh_path
    if ! probe_tool "$label" "$@"; then
        echo "machine setup: $label is still not runnable after the install ('$*' does not run; winget exited $rc)." >&2
        exit 1
    fi
}

# For the README bullets that link to a download page instead of naming an
# install command: Bash, Go, Docker, and the language toolchains. This script
# does not invent a command the README does not have, so a guest without them
# stops here - but it stops naming the tool and the same link the README would
# have sent you to, rather than as a bare "go: command not found".
#
# $1 label, $2 download URL, $3.. version command
require_tool() {
    local label="$1" url="$2"
    shift 2
    if probe_tool "$label" "$@"; then
        return
    fi
    echo "machine setup: $label is required and this script does not install it. Install it from $url and re-run." >&2
    exit 1
}

# `go install` drops the binary in $(go env GOPATH)/bin and never edits PATH —
# the Go installer only puts the toolchain itself there. On a brand-new system
# actionlint is "command not found" straight after installing it without this.
# The README says the same under its actionlint bullet: "Add that directory to
# PATH yourself."
#
# cygpath because on Windows `go env GOPATH` answers in native form
# (C:\Users\you\go) and PATH here is colon-separated, so appending it raw splits
# at the drive letter into "C" and "\Users\you\go/bin" — two entries, neither a
# directory. cygpath -u yields /c/Users/you/go/bin, which survives the split.
# The VM guest is Windows, so this is the path that actually gets exercised.
add_go_bin_to_path() {
    local gopath
    gopath="$(go env GOPATH)"
    if command -v cygpath >/dev/null 2>&1; then
        gopath="$(cygpath -u "$gopath")"
    fi
    export PATH="$PATH:$gopath/bin"
    hash -r
}


# Inventory ===================================================================
#
# Read-only, and first: one line per tool this file deals with, before anything
# is installed. On a guest that is meant to be clean every line should read NOT
# installed, which makes that block the proof the checkpoint was restored; on a
# guest that is not clean it is the explanation for whatever happens further
# down, in the one place nobody has to scroll to find.
#
# Nothing here exits. Enforcement stays with the ensure_*/require_* functions
# below, which run in README order and stop where the README's reader would -
# this pass only reports, so the whole picture is in the log even when the run
# is about to stop on the first missing tool.
#
# probe_tool is reused rather than reimplemented: one probe in the file means
# the inventory cannot disagree with the check that gates an install.

inventory() {
    step 'inventory'
    echo "readme-steps: what this guest already has, before anything is installed:"
    probe_tool 'Git'         git --version        || true
    probe_tool 'winget'      winget --version     || true
    probe_tool 'GitHub CLI'  gh --version         || true
    probe_tool 'Claude Code' claude --version     || true
    probe_tool 'Bash'        bash --version       || true
    probe_tool 'Go'          go version           || true
    probe_tool 'actionlint'  actionlint --version || true
    probe_tool 'Docker'      docker --version     || true
    probe_tool 'Java'        java -version        || true
    probe_tool '.NET'        dotnet --version     || true
    probe_tool 'Node.js'     npm --version        || true
}


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

machine_setup() {
    ensure_git_installed
    ensure_winget_usable
    ensure_gh_installed
    ensure_gh_signed_in
    ensure_claude_installed
    ensure_claude_signed_in
}

# Git for Windows supplies the Git Bash this script runs under, so it cannot
# install itself from in here. scripts/readme-setup.ps1 does it from PowerShell,
# which is why that is the first thing you run in the guest.
ensure_git_installed() {
    step 'machine setup: git'
    if probe_tool 'Git' git --version; then
        return
    fi
    echo "machine setup: no git. From PowerShell: powershell -ExecutionPolicy Bypass -File C:\\Users\\Public\\readme-setup.ps1" >&2
    exit 1
}

# Windows 11 ships winget as App Installer, but a guest straight off the ISO
# sometimes has a version too old to run until the Store updates it - which is
# why this asks winget for its version rather than just for its presence.
ensure_winget_usable() {
    step 'machine setup: winget'
    if probe_tool 'winget' winget --version; then
        return
    fi
    echo "machine setup: no usable winget. Open Microsoft Store, update 'App Installer', then re-run." >&2
    exit 1
}

ensure_gh_installed() {
    step 'machine setup: GitHub CLI install'
    winget_install GitHub.cli 'GitHub CLI' gh --version
}

# [interactive] Prompts, then a browser.
ensure_gh_signed_in() {
    step 'machine setup: GitHub CLI sign-in [interactive, off-log]'
    if gh auth status >/dev/null 2>&1; then
        echo "machine setup: gh already signed in"
        return
    fi
    require_tty "gh auth login"
    # Real console, not the log - see "Logging" above. gh draws its prompts on
    # stderr, so neither stream can be diverted here without breaking them; the
    # exit code is the only part of this step that can be written down, so it is.
    local login_rc=0
    gh auth login >&3 2>&4 || login_rc=$?
    echo "machine setup: gh auth login exited $login_rc"
    if [ "$login_rc" -ne 0 ]; then exit "$login_rc"; fi
}

# The native-Windows installer, run through a PowerShell shim because this file
# is Git Bash. claude.ai/install.sh is the macOS/Linux/WSL installer and is NOT
# the right one here.
ensure_claude_installed() {
    step 'machine setup: Claude Code install'
    if probe_tool 'Claude Code' claude --version; then
        return
    fi
    echo "machine setup: installing Claude Code ..."
    powershell.exe -NoProfile -Command "irm https://claude.ai/install.ps1 | iex"
    refresh_path
    if ! probe_tool 'Claude Code' claude --version; then
        echo "machine setup: Claude Code was installed but 'claude --version' still does not run." >&2
        exit 1
    fi
}

# [interactive] Opens a browser. Needs a Pro, Max, Team, or Enterprise plan; the
# free Claude.ai plan has no Claude Code.
#
# ~/.claude.json gains an "oauthAccount" entry once sign-in completes, which
# answers "is this machine signed in" without spending a request. On a clean
# guest it is always absent.
ensure_claude_signed_in() {
    step 'machine setup: Claude Code sign-in [interactive, off-log]'
    if grep -q '"oauthAccount"' "$HOME/.claude.json" 2>/dev/null; then
        echo "machine setup: Claude Code already signed in"
        return
    fi
    require_tty "the Claude Code sign-in"
    echo "machine setup: starting Claude Code — sign in, then type /exit to come back here."
    # Git Bash usually runs under mintty, which is not a Windows console; the
    # raw-mode TUI needs winpty in front of it. Git for Windows ships winpty.
    # >&3 2>&4 for the same reason: a TUI with a pipe for stdout will not draw.
    # As with the sign-in above, the exit code is all this step can log.
    local claude_rc=0
    if command -v winpty >/dev/null 2>&1; then
        winpty claude >&3 2>&4 || claude_rc=$?
    else
        claude >&3 2>&4 || claude_rc=$?
    fi
    echo "machine setup: Claude Code exited $claude_rc"
    if [ "$claude_rc" -ne 0 ]; then exit "$claude_rc"; fi
}


# Prerequisites ===============================================================
#
# Verification only — machine_setup above is what installs these.

readme_prerequisites() {
    step 'Prerequisites'

    # GitHub CLI
    gh --version
    gh auth status

    # Claude Code
    claude --version
}


# Install =====================================================================

readme_install() {
    step 'Install'
    gh extension install optivem/gh-optivem
    gh optivem --version
}


# Local environment setup =====================================================

readme_local_environment_setup() {
    step 'Local environment setup'

    # The README runs this first, before the tools below are installed, precisely
    # so it reports everything missing in one pass. On a brand-new system it is
    # therefore EXPECTED to fail here — hence `|| true`. The re-run in
    # readme_environment_variables is the one that must pass.
    gh optivem environment verify || true

    ensure_bash_installed
    ensure_go_installed
    ensure_actionlint_installed
    ensure_docker_installed
    ensure_language_toolchains
}

ensure_bash_installed() {
    require_tool 'Bash' 'https://git-scm.com/download/win' bash --version
}

# The README's own note: Go only matters because actionlint below is installed
# with it.
ensure_go_installed() {
    require_tool 'Go' 'https://go.dev/dl/' go version
}

# The one README bullet that names an install command rather than a download
# page, so this is the only tool down here that `ensure` can actually install.
ensure_actionlint_installed() {
    if probe_tool 'actionlint' actionlint --version; then
        return
    fi
    echo "machine setup: installing actionlint ..."
    go install github.com/rhysd/actionlint/cmd/actionlint@v1
    add_go_bin_to_path
    if ! probe_tool 'actionlint' actionlint --version; then
        echo "machine setup: actionlint was installed but is not on PATH. Add $(go env GOPATH)/bin to PATH and re-run." >&2
        exit 1
    fi
}

ensure_docker_installed() {
    require_tool 'Docker' 'https://docs.docker.com/get-started/get-docker/' docker --version
}

# The README says to check only the languages you need. All three are checked
# because readme_generate_your_project scaffolds with all three.
ensure_language_toolchains() {
    require_tool 'Java'    'https://adoptium.net/'                 java -version
    require_tool '.NET'    'https://dotnet.microsoft.com/download' dotnet --version
    require_tool 'Node.js' 'https://nodejs.org/'                   npm --version
}


# Environment variables =======================================================

readme_environment_variables() {
    step 'Environment variables'
    gh optivem environment show

    # "Then re-run gh optivem environment verify — it live-checks each token
    # against its provider, so it will now pass."
    gh optivem environment verify
}


# Claude Code setup ===========================================================

readme_claude_code_setup() {
    step 'Claude Code setup'
    gh optivem claude setup
}


# Generate your project =======================================================

readme_generate_your_project() {
    step 'Generate your project'
    gh optivem init --owner "$OWNER" --repo "$REPO" --system-name "Book Shop" --repo-strategy monorepo --arch multitier --backend-lang dotnet --frontend-lang typescript --test-lang java
}


# Clone project repository ====================================================

readme_clone_project_repository() {
    step 'Clone project repository'
    gh repo clone "$OWNER/$REPO"
    # A bash function has no working directory of its own, so this `cd` moves the
    # whole script - which is the point. Everything after this runs in the
    # generated repo, exactly as the README's reader would be sitting in it.
    cd "$REPO"
}


# Verify your project =========================================================

readme_verify_your_project() {
    step 'Verify your project'
    gh optivem system-test setup
    gh optivem system start
    gh optivem system-test run --sample
    gh optivem system stop
}


# Implement a ticket ==========================================================
#
# The README's own instructions, followed literally: "create a ticket in your
# repository and add it to the GitHub Project board `init` created ... copy
# optivem/shop#72 as a worked example of the shape the agents expect."
#
# By a wide margin the longest step in this file: a full RED-to-GREEN ATDD walk
# that dispatches a Claude agent at each stage and runs the real build, system,
# and test commands in between. Tens of minutes, and it spends Claude usage. It
# runs last, so nothing that already passed is at risk if it fails.

readme_implement_a_ticket() {
    step 'Implement a ticket'
    copy_source_ticket
    add_ticket_to_project_board

    # The README shows the plain form first, then the unattended one:
    #
    #   gh optivem implement 56
    #   gh optivem --auto implement 56 --headless
    #
    # Only the second runs. Both walk the same ticket through the same
    # pipeline, and the first walk implements it, so a second would find
    # nothing left to do. The unattended form is the one that can finish
    # without somebody sitting in front of the VM answering prompts for the
    # length of the walk.
    gh optivem --auto implement "$ticket_number" --headless
}

copy_source_ticket() {
    echo "implement: copying $SOURCE_TICKET_REPO#$SOURCE_TICKET ..."
    local title body url
    title="$(gh issue view "$SOURCE_TICKET" --repo "$SOURCE_TICKET_REPO" --json title --jq .title)"
    body="$(gh issue view "$SOURCE_TICKET" --repo "$SOURCE_TICKET_REPO" --json body --jq .body)"
    if [ -z "$title" ] || [ -z "$body" ]; then
        echo "implement: $SOURCE_TICKET_REPO#$SOURCE_TICKET came back with an empty title or body. It is a public issue, so check 'gh auth status' first." >&2
        exit 1
    fi

    # --body-file - rather than --body "$body": the acceptance criteria are a
    # multi-line Gherkin block, and this is the one form that cannot be reshaped
    # by argument handling on the way through.
    url="$(printf '%s' "$body" | gh issue create --repo "$OWNER/$REPO" --title "$title" --body-file -)"

    # `gh issue create` answers with the new issue's URL, and the number is its
    # last segment.
    ticket_number="${url##*/}"
    case "$ticket_number" in
        ''|*[!0-9]*)
            echo "implement: could not read an issue number out of 'gh issue create' (got '$url')." >&2
            exit 1
            ;;
    esac
    echo "implement: created $OWNER/$REPO#$ticket_number - $title"
}

# `implement` looks the issue up among the board's items and fails with
# "issue #N not found on project" if it is not there, so this is not optional.
add_ticket_to_project_board() {
    local owner number

    # init created the board and wrote it back into gh-optivem.yaml, which is a
    # better answer than matching a board title in `gh project list` - two
    # boards may share the title, and only one of them is the one init used.
    #
    # Scoped to the top-level `project:` block so no other `url:` in the file
    # can answer by accident. Read from the repo root, which the script is
    # already sitting in: readme_clone_project_repository cd'd there.
    project_url="$(awk '
        /^project:/     { in_project = 1; next }
        /^[^[:space:]]/ { in_project = 0 }
        in_project && $1 == "url:" { print $2; exit }
    ' gh-optivem.yaml)"

    # https://github.com/users/<login>/projects/<n>, or /orgs/ for an org.
    owner="$( printf '%s\n' "$project_url" | sed -nE 's#^https://github\.com/(users|orgs)/([^/]+)/projects/[0-9]+/?$#\2#p')"
    number="$(printf '%s\n' "$project_url" | sed -nE 's#^https://github\.com/(users|orgs)/[^/]+/projects/([0-9]+)/?$#\2#p')"
    if [ -z "$owner" ] || [ -z "$number" ]; then
        echo "implement: no usable project URL under 'project:' in gh-optivem.yaml (read '$project_url'). init writes it there when it creates the board." >&2
        exit 1
    fi

    echo "implement: adding issue #$ticket_number to project $owner/$number"
    gh project item-add "$number" --owner "$owner" \
        --url "https://github.com/$OWNER/$REPO/issues/$ticket_number"
}


# Upgrade, Uninstall ==========================================================
#
# Not run — they would undo the install above:
#
#   gh extension upgrade optivem
#   gh extension remove optivem


# Run =========================================================================
#
# README.md's table of contents, in its order, plus the one line that has no
# README counterpart. If these two lists stop matching, this file is wrong.

# Before the first probe, not just after an install. This shell was handed a
# PATH snapshot taken when its parent window opened, which can predate anything
# a previous run installed - on 2026-08-12 that made a working gh read as "NOT
# installed" and the install that followed ended the run. The post-install
# refreshes further down stay: those cover what THIS run adds.
echo "readme-steps: reading PATH back from the registry before the first probe"
refresh_path

inventory                         # no README counterpart - read-only, reports only
machine_setup                     # no README counterpart - see the section above

readme_prerequisites
readme_install
readme_local_environment_setup
readme_environment_variables
readme_claude_code_setup
readme_generate_your_project
readme_clone_project_repository
readme_verify_your_project
readme_implement_a_ticket

# The trap prints the verdict and the log path; this is the part it cannot know.
# The board is named after the system rather than the repo, so init REUSES it
# across runs instead of making a new one - every run leaves another item on the
# same board, and deleting the repo does not clear them.
echo "readme-steps: remember to delete $OWNER/$REPO"
if [ -n "$project_url" ]; then
    echo "readme-steps: and to remove this run's item from $project_url"
fi

# The only assignment of `completed` in the file, and it has to stay last: it is
# what tells the EXIT trap this run reached the end under its own power rather
# than being killed on the way. Adding a step below this line hides it from the
# verdict - add it above.
completed=1

<#
.SYNOPSIS
  Run INSIDE the clean-room VM: install Git for Windows, which supplies the Git
  Bash that scripts/readme-steps.sh needs, then hand the walkthrough over to it.

.DESCRIPTION
  Step 5 of the install-test loop, and the first thing you run in the guest:

    scripts/vm-iso-download.ps1        # obtain a Windows 11 ISO
    scripts/vm-host-status.ps1         # read-only report - where does the host stand
    scripts/vm-hyperv-enable.ps1       # one-time host setup
    scripts/vm-machine-create.ps1      # build a clean VM from a Windows ISO
    scripts/vm-checkpoint-create.ps1   # freeze the clean install as 'clean-baseline'
    scripts/vm-scripts-copy.ps1        # copy the walkthrough into the guest
    scripts/readme-setup.ps1           # run INSIDE the VM: this script
    scripts/readme-steps.sh            # run INSIDE the VM: every README command
    scripts/vm-checkpoint-restore.ps1  # revert to the baseline for the next run
    scripts/vm-machine-delete.ps1      # tear the VM down
    scripts/vm-hyperv-disable.ps1      # turn Hyper-V back off

  readme-steps.sh is a bash script, and a clean Windows guest has no bash. It
  cannot install the shell it runs under, so that one step has to happen out
  here, in the shell Windows ships with.

  This is deliberately the ONLY thing that runs in PowerShell. Everything else
  the README asks for is installed by readme-steps.sh itself, where it sits next
  to the command that verifies it.

  Two details that otherwise cost a VM cycle each:

    - Git for Windows puts bash.exe in Git\bin but only adds Git\cmd to PATH, so
      `bash` is still not a command after a successful install. This script
      resolves the absolute path instead of trusting PATH.
    - The guest has Windows PowerShell 5.1 and no pwsh, and its execution policy
      refuses unsigned .ps1 files, so this has to be started as:

        powershell -ExecutionPolicy Bypass -File C:\Users\Public\readme-setup.ps1

  Installing Git machine-wide raises a UAC prompt when the shell is not already
  elevated. That is expected; accept it.

.PARAMETER Run
  Start readme-steps.sh as soon as Git is in place, rather than printing the
  command and stopping. The walkthrough signs you in to GitHub and Claude and
  creates a real repository, so it is opt-in.

.PARAMETER StepsPath
  The walkthrough to hand over to. Defaults to readme-steps.sh next to this
  script, which is where vm-scripts-copy.ps1 puts the pair.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File C:\Users\Public\readme-setup.ps1

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File C:\Users\Public\readme-setup.ps1 -Run
#>
[CmdletBinding()]
param(
    [switch]$Run,
    [string]$StepsPath = ''
)

$ErrorActionPreference = 'Stop'

# --- Preconditions -------------------------------------------------------------
if (-not $StepsPath) {
    $StepsPath = Join-Path $PSScriptRoot 'readme-steps.sh'
}
if (-not (Test-Path -LiteralPath $StepsPath -PathType Leaf)) {
    Write-Error "No walkthrough at '$StepsPath'. Copy it in from the host first: pwsh -File scripts/vm-scripts-copy.ps1. Or pass -StepsPath."
}

# --- Git for Windows -----------------------------------------------------------
# bash.exe, not git.exe, is the thing that has to exist: git alone would leave
# readme-steps.sh unrunnable, which is the whole point of this script.
function Find-Bash {
    $onPath = Get-Command bash.exe -ErrorAction SilentlyContinue
    if ($onPath) { return $onPath.Source }

    # Git\bin is not added to PATH by the default install, so look where the
    # installer actually puts it - machine scope first, then a user-scope install.
    $candidates = @(
        (Join-Path $env:ProgramFiles 'Git\bin\bash.exe'),
        (Join-Path ${env:ProgramFiles(x86)} 'Git\bin\bash.exe'),
        (Join-Path $env:LOCALAPPDATA 'Programs\Git\bin\bash.exe')
    )
    foreach ($c in $candidates) {
        if ($c -and (Test-Path -LiteralPath $c -PathType Leaf)) { return $c }
    }
    return $null
}

$bash = Find-Bash
if ($bash) {
    Write-Host "readme-setup: Git Bash already installed ($bash)" -ForegroundColor Green
} else {
    if (-not (Get-Command winget.exe -ErrorAction SilentlyContinue)) {
        Write-Error "No winget. Windows 11 ships it as App Installer, but a guest straight off the ISO sometimes has a version too old to run: open Microsoft Store, update 'App Installer', then re-run. Failing that, install Git for Windows by hand from https://git-scm.com/download/win"
    }

    Write-Host 'readme-setup: installing Git for Windows ...' -ForegroundColor Cyan
    Write-Host '  Accept the UAC prompt if one appears.' -ForegroundColor DarkGray

    # Without the --accept-* flags winget stops on a licence prompt the first
    # time it sees a source or package, which on a fresh guest is always.
    winget.exe install --id Git.Git -e --source winget `
        --accept-source-agreements --accept-package-agreements
    if ($LASTEXITCODE -ne 0) {
        # $ErrorActionPreference does not catch native exit codes, so this check
        # is the only thing standing between a failed install and a confusing
        # "no bash" error three lines down.
        Write-Error "winget install Git.Git exited $LASTEXITCODE. Install Git for Windows by hand from https://git-scm.com/download/win and re-run."
    }

    # The installer wrote PATH into the registry; this process still holds the
    # copy it started with.
    $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' +
                [Environment]::GetEnvironmentVariable('Path', 'User')

    $bash = Find-Bash
    if (-not $bash) {
        Write-Error "Git installed but no bash.exe was found under Program Files\Git\bin or %LOCALAPPDATA%\Programs\Git\bin. Open a new PowerShell window and re-run; if it still cannot be found, the install did not include Git Bash."
    }
    Write-Host ''
    Write-Host "readme-setup: Git Bash installed ($bash)" -ForegroundColor Green
}

& $bash --version

# --- Hand over -----------------------------------------------------------------
if ($Run) {
    Write-Host ''
    Write-Host 'readme-setup: starting the README walkthrough ...' -ForegroundColor Cyan
    Write-Host '  It waits on the keyboard twice: the GitHub sign-in and the Claude sign-in.' -ForegroundColor DarkGray
    Write-Host ''
    & $bash $StepsPath
    exit $LASTEXITCODE
}

Write-Host ''
Write-Host 'Next' -ForegroundColor Cyan
Write-Host @"
  1. Run the README walkthrough:

       & '$bash' '$StepsPath'

     It waits on the keyboard twice - the GitHub sign-in and the Claude sign-in -
     so leave it in the foreground. Pass -Run to this script to chain straight
     into it next time.

  2. It stops at the first failure. That is the one worth reading: it means
     README.md and a brand-new machine disagree.
"@

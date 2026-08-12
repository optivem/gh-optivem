<#
.SYNOPSIS
  Pull the guest's run logs back to the host, where you can actually read them.

.DESCRIPTION
  Step 6 of the install-test loop:

    scripts/vm-iso-download.ps1        # obtain a Windows 11 ISO
    scripts/vm-host-status.ps1         # read-only report - where does the host stand
    scripts/vm-hyperv-enable.ps1       # one-time host setup
    scripts/vm-machine-create.ps1      # build a clean VM from a Windows ISO
    scripts/vm-checkpoint-create.ps1   # freeze the clean install as 'clean-baseline'
    scripts/vm-scripts-copy.ps1        # copy the walkthrough into the guest
    scripts/readme-setup.ps1           # run INSIDE the VM: install Git Bash
    scripts/readme-steps.sh            # run INSIDE the VM: every README command
    scripts/vm-logs-copy.ps1           # this script
    scripts/vm-checkpoint-restore.ps1  # revert to the baseline for the next run
    scripts/vm-machine-delete.ps1      # tear the VM down
    scripts/vm-hyperv-disable.ps1      # turn Hyper-V back off

  Reading a failed run in the VM console means squinting at scrollback in a
  window you cannot copy out of. Both guest-side scripts write a timestamped log
  instead; this brings those files to the host.

  Copy-VMFile, which vm-scripts-copy.ps1 uses, only goes host to guest - there is
  no guest-to-host direction. The way back is PowerShell Direct: a remoting
  session over the VMBus, no network involved. That costs one thing the rest of
  the loop does not need, GUEST CREDENTIALS, because the session logs in as a
  user inside the VM. On the clean-room guest that is the local account created
  during Windows setup. You are prompted unless -GuestCredential is passed.

  The VM has to be running and booted to the desktop. PowerShell Direct talks to
  a service that does not exist mid-setup, and a call made too early can sit
  waiting rather than fail.

  Run this BEFORE vm-checkpoint-restore.ps1. The revert discards everything the
  guest wrote, logs included, and there is no undo.

.PARAMETER Name
  VM to pull from. Defaults to gh-optivem-install-test.

.PARAMETER GuestPath
  Directory inside the guest holding the logs. Defaults to
  C:\Users\Public\logs, which is where the guest-side scripts write them.

.PARAMETER Destination
  Host directory to copy into. Defaults to .vm-logs at the repo root, which is
  gitignored. Files keep their names, and those already carry a timestamp.

.PARAMETER GuestCredential
  Credentials of an account inside the guest. Prompted for when not supplied.

.EXAMPLE
  pwsh -File scripts/vm-logs-copy.ps1

.EXAMPLE
  pwsh -File scripts/vm-logs-copy.ps1 -Destination D:\runs\attempt-3
#>
[CmdletBinding()]
param(
    [string]      $Name        = 'gh-optivem-install-test',
    [string]      $GuestPath   = 'C:\Users\Public\logs',
    [string]      $Destination = '',
    [pscredential]$GuestCredential
)

$ErrorActionPreference = 'Stop'

function Test-Elevated {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal $id).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
}

# --- Preconditions -----------------------------------------------------------
if (-not (Test-Elevated)) {
    Write-Error "The Hyper-V cmdlets require an elevated shell. Open PowerShell as Administrator and re-run: pwsh -File `"$PSCommandPath`" -Name `"$Name`""
}

$vm = Get-VM -Name $Name -ErrorAction SilentlyContinue
if (-not $vm) {
    Write-Error "No VM named '$Name'. Build one first: pwsh -File scripts/vm-machine-create.ps1 -IsoPath <path-to-win11.iso>"
}
if ($vm.State -ne 'Running') {
    Write-Error "VM '$Name' is $($vm.State). PowerShell Direct needs it running and booted to the desktop: Start-VM -Name '$Name'"
}

if (-not $Destination) {
    $Destination = Join-Path (Split-Path -Parent $PSScriptRoot) '.vm-logs'
}
if (-not (Test-Path -LiteralPath $Destination)) {
    New-Item -ItemType Directory -Path $Destination -Force | Out-Null
}
$Destination = (Resolve-Path -LiteralPath $Destination).Path

if (-not $GuestCredential) {
    Write-Host ''
    Write-Host "Sign in to the guest '$Name'." -ForegroundColor Cyan
    Write-Host '  This is the account you created during Windows setup, not a host account.' -ForegroundColor DarkGray
    $GuestCredential = Get-Credential -Message "Account inside the VM '$Name'"
}

# --- Session -------------------------------------------------------------------
Write-Host ''
Write-Host "Connecting to '$Name' over the VMBus ..." -ForegroundColor Cyan

$session = $null
try {
    $session = New-PSSession -VMName $Name -Credential $GuestCredential
} catch {
    Write-Error "Could not open a PowerShell Direct session to '$Name': $($_.Exception.Message). Check that the guest is booted to the desktop, and that the credentials are for an account INSIDE the VM (a local account created during Windows setup, not a host or Microsoft account)."
}

try {
    # Asking the guest first turns "no logs yet" into a sentence rather than a
    # Copy-Item stack trace about a path that does not exist.
    $files = Invoke-Command -Session $session -ScriptBlock {
        param($p)
        if (-not (Test-Path -LiteralPath $p)) { return $null }
        Get-ChildItem -LiteralPath $p -File | Select-Object Name, Length, LastWriteTime
    } -ArgumentList $GuestPath

    if (-not $files) {
        Write-Host ''
        Write-Host "Nothing at '$GuestPath' in the guest." -ForegroundColor Yellow
        Write-Host '  The guest-side scripts write their logs there on the first run, so this is' -ForegroundColor DarkGray
        Write-Host '  what an untouched guest looks like. Pass -GuestPath if you moved them.' -ForegroundColor DarkGray
        return
    }

    Write-Host ''
    Write-Host ("Copying {0} file(s) to {1}:" -f @($files).Count, $Destination) -ForegroundColor Cyan
    foreach ($f in @($files)) {
        Write-Host ("  {0,-40} {1,8:N0} bytes  {2}" -f $f.Name, $f.Length, $f.LastWriteTime) -ForegroundColor DarkGray
    }

    Copy-Item -FromSession $session -Path (Join-Path $GuestPath '*') `
        -Destination $Destination -Recurse -Force
} finally {
    if ($session) { Remove-PSSession $session }
}

Write-Host ''
Write-Host 'Copied.' -ForegroundColor Green

# --- Next steps ----------------------------------------------------------------
$newest = Get-ChildItem -LiteralPath $Destination -File |
    Sort-Object LastWriteTime -Descending | Select-Object -First 1

Write-Host ''
Write-Host 'Next' -ForegroundColor Cyan
Write-Host @"
  1. Read the newest one:

       Get-Content '$(if ($newest) { $newest.FullName } else { Join-Path $Destination '<log>' })' | Select-Object -Last 40

  2. Only then reset the guest. The revert discards the guest's copies, so
     anything not pulled across by now is gone:

       pwsh -File scripts/vm-checkpoint-restore.ps1 -Name '$Name'
"@

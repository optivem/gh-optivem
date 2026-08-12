<#
.SYNOPSIS
  Revert the VM to its baseline checkpoint, discarding everything the last test
  run did to it.

.DESCRIPTION
  Step 5 of the install-test loop:

    scripts/vm-iso-download.ps1        # obtain a Windows 11 ISO
    scripts/vm-host-status.ps1         # read-only report — where does the host stand
    scripts/vm-hyperv-enable.ps1       # one-time host setup
    scripts/vm-machine-create.ps1      # build a clean VM from a Windows ISO
    scripts/vm-checkpoint-create.ps1   # freeze the clean install as 'clean-baseline'
    scripts/readme-steps.sh            # run INSIDE the VM: every README command
    scripts/vm-checkpoint-restore.ps1  # this script
    scripts/vm-machine-delete.ps1      # tear the VM down
    scripts/vm-hyperv-disable.ps1      # turn Hyper-V back off

  This is what makes the loop cheap. Installing Windows costs an hour; reverting
  costs seconds, so a README test is something you can afford to run again after
  every fix rather than once.

  Everything the guest did since the checkpoint is discarded — installed tools,
  cloned repos, files you wrote inside the VM. Anything worth keeping has to come
  out to the host first. The checkpoint itself is NOT consumed: revert to it as
  often as you like.

.PARAMETER Name
  VM to reset. Defaults to gh-optivem-install-test.

.PARAMETER SnapshotName
  Checkpoint to revert to. Defaults to clean-baseline.

.PARAMETER Yes
  Skip the confirmation prompt. For unattended use only.

.PARAMETER Start
  Start the VM once it has been reverted, and print the console command.

.EXAMPLE
  pwsh -File scripts/vm-checkpoint-restore.ps1

.EXAMPLE
  pwsh -File scripts/vm-checkpoint-restore.ps1 -SnapshotName after-prereqs -Yes -Start
#>
[CmdletBinding()]
param(
    [string]$Name         = 'gh-optivem-install-test',
    [string]$SnapshotName = 'clean-baseline',
    [switch]$Yes,
    [switch]$Start
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

$snap = Get-VMSnapshot -VMName $Name -Name $SnapshotName -ErrorAction SilentlyContinue
if (-not $snap) {
    $available = @(Get-VMSnapshot -VMName $Name -ErrorAction SilentlyContinue)
    if ($available) {
        $list = ($available | ForEach-Object { "$($_.Name) ($($_.CreationTime))" }) -join '; '
        Write-Error "No checkpoint named '$SnapshotName' on '$Name'. Available: $list. Pass -SnapshotName with one of those."
    }
    Write-Error "VM '$Name' has no checkpoints, so there is nothing to revert to. Take a baseline first: pwsh -File scripts/vm-checkpoint-create.ps1 -Name '$Name'"
}

# --- What is about to be discarded -------------------------------------------
Write-Host ''
Write-Host ("Reverting '{0}' to '{1}'" -f $Name, $SnapshotName) -ForegroundColor Cyan
Write-Host ("  Checkpoint taken : {0}" -f $snap.CreationTime)
Write-Host ("  Current state    : {0}" -f $vm.State)
Write-Host ''
Write-Host 'Everything the guest has done since that checkpoint will be discarded:' -ForegroundColor Yellow
Write-Host '  installed tools, cloned repos, and any files written inside the VM.' -ForegroundColor Yellow
Write-Host ("The checkpoint itself is kept — you can revert to it again later.") -ForegroundColor DarkGray
Write-Host ''

if (-not $Yes) {
    $answer = Read-Host "Revert now? [y/N]"
    if ($answer -notmatch '^(y|yes)$') {
        Write-Host 'Cancelled — the VM was left as it is.' -ForegroundColor Green
        exit 0
    }
}

# --- Revert ------------------------------------------------------------------
# -TurnOff rather than a graceful shutdown: the running state is about to be
# thrown away regardless, so there is nothing for the guest to flush.
if ($vm.State -ne 'Off') {
    Write-Host "Stopping '$Name' ..." -ForegroundColor Cyan
    Stop-VM -Name $Name -TurnOff -Force
}

Write-Host "Restoring checkpoint '$SnapshotName' ..." -ForegroundColor Cyan
Restore-VMCheckpoint -VMName $Name -Name $SnapshotName -Confirm:$false

$vm = Get-VM -Name $Name
Write-Host ''
Write-Host ("'{0}' is back at '{1}' (state: {2})." -f $Name, $SnapshotName, $vm.State) -ForegroundColor Green

# --- Next steps --------------------------------------------------------------
$stepsPath = Join-Path $PSScriptRoot 'readme-steps.sh'

if ($Start) {
    if ($vm.State -eq 'Off') {
        Write-Host "Starting '$Name' ..." -ForegroundColor Cyan
        Start-VM -Name $Name
    }
    Write-Host ''
    Write-Host 'Open the console with:' -ForegroundColor Cyan
    Write-Host ("  vmconnect.exe localhost '{0}'" -f $Name)
} else {
    Write-Host ''
    Write-Host 'Next' -ForegroundColor Cyan
    Write-Host @"
  1. Start it and open the console:

       Start-VM -Name '$Name'
       vmconnect.exe localhost '$Name'

  2. Copy the README walkthrough in and run the test again:

       Copy-VMFile -Name '$Name' -SourcePath '$stepsPath' ``
         -DestinationPath 'C:\Users\Public\readme-steps.sh' -CreateFullPath -FileSource Host
"@
}

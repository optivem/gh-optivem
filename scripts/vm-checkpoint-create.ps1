<#
.SYNOPSIS
  Freeze the VM's current state as a named checkpoint - the baseline every later
  test run reverts to.

.DESCRIPTION
  Step 3 of the install-test loop:

    scripts/vm-iso-download.ps1        # obtain a Windows 11 ISO
    scripts/vm-host-status.ps1         # read-only report - where does the host stand
    scripts/vm-hyperv-enable.ps1       # one-time host setup
    scripts/vm-machine-create.ps1      # build a clean VM from a Windows ISO
    scripts/vm-checkpoint-create.ps1   # this script
    scripts/vm-scripts-copy.ps1        # copy the walkthrough into the guest
    scripts/readme-setup.ps1           # run INSIDE the VM: install Git Bash
    scripts/readme-steps.sh            # run INSIDE the VM: every README command
    scripts/vm-logs-copy.ps1           # pull the guest run logs back to the host
    scripts/vm-checkpoint-restore.ps1  # revert to the baseline for the next run
    scripts/vm-machine-delete.ps1      # tear the VM down
    scripts/vm-hyperv-disable.ps1      # turn Hyper-V back off

  Installing Windows costs an hour; reverting to a checkpoint costs seconds. Run
  this once, immediately after Windows setup finishes and before you install
  anything else, and every subsequent README test starts from the same machine.

  The guest is shut down first, on purpose. A checkpoint of a running VM captures
  RAM as well as disk, so restoring it drops you back into a mid-session machine
  rather than a cold boot - fine for pausing work, wrong for a baseline that is
  supposed to be identical every time. Pass -AllowRunning if you really want the
  running-state variant.

  Whatever patch level you freeze is what every future run starts from, so decide
  about Windows Update BEFORE taking the baseline, not after.

  Restoring never consumes the checkpoint: scripts/vm-checkpoint-restore.ps1 can revert to it
  as many times as you like, and it survives host reboots.

.PARAMETER Name
  VM to checkpoint. Defaults to gh-optivem-install-test.

.PARAMETER SnapshotName
  Checkpoint name. Defaults to clean-baseline, which is what vm-checkpoint-restore.ps1 looks
  for. Use a second name to layer a later point (e.g. after-prereqs) without
  disturbing the clean one.

.PARAMETER AllowRunning
  Checkpoint without shutting the guest down. Captures memory, so restores resume
  mid-session instead of cold-booting.

.PARAMETER Force
  Replace an existing checkpoint of the same name.

.PARAMETER ShutdownTimeoutSeconds
  How long to wait for the guest to shut down before giving up.

.EXAMPLE
  pwsh -File scripts/vm-checkpoint-create.ps1

.EXAMPLE
  pwsh -File scripts/vm-checkpoint-create.ps1 -SnapshotName after-prereqs
#>
[CmdletBinding()]
param(
    [string]$Name                   = 'gh-optivem-install-test',
    [string]$SnapshotName           = 'clean-baseline',
    [switch]$AllowRunning,
    [switch]$Force,
    [int]   $ShutdownTimeoutSeconds = 300
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

# Production checkpoints ask the guest for a VSS-consistent snapshot; standard
# checkpoints capture the disk byte-for-byte. Only the latter gives back exactly
# the machine you froze, which is the entire premise of the baseline.
if ($vm.CheckpointType -ne 'Standard') {
    Write-Warning "VM '$Name' is set to $($vm.CheckpointType) checkpoints, which restore a VSS-consistent snapshot rather than the disk byte-for-byte. Switching it to Standard."
    Set-VM -Name $Name -CheckpointType Standard
}

$existing = Get-VMSnapshot -VMName $Name -Name $SnapshotName -ErrorAction SilentlyContinue
if ($existing) {
    if (-not $Force) {
        Write-Error "Checkpoint '$SnapshotName' already exists on '$Name' (taken $($existing.CreationTime)). Pass -SnapshotName to add a separate one, or -Force to replace it. Replacing discards the old baseline permanently."
    }
    Write-Host "Removing the existing '$SnapshotName' checkpoint (-Force) ..." -ForegroundColor Yellow
    Remove-VMSnapshot -VMName $Name -Name $SnapshotName -IncludeAllChildSnapshots

    # Removal merges differencing disks asynchronously; checkpointing mid-merge
    # can leave the new checkpoint sitting on a disk chain that is still moving.
    $deadline = (Get-Date).AddMinutes(15)
    while ((Get-VM -Name $Name).Status -match 'merg' -and (Get-Date) -lt $deadline) {
        Write-Host '  merging ...' -ForegroundColor DarkGray
        Start-Sleep -Seconds 5
    }
    if ((Get-VM -Name $Name).Status -match 'merg') {
        Write-Error 'Disk merge is still running after 15 minutes. Re-run once Hyper-V Manager shows the VM idle.'
    }
}

# --- Shut the guest down -----------------------------------------------------
$vm = Get-VM -Name $Name
if ($vm.State -ne 'Off') {
    if ($AllowRunning) {
        Write-Warning "Checkpointing '$Name' while it is $($vm.State). This captures memory, so restores will resume mid-session rather than cold-booting."
    } else {
        $shutdownSvc = Get-VMIntegrationService -VMName $Name -Name 'Shutdown' -ErrorAction SilentlyContinue
        if (-not $shutdownSvc -or -not $shutdownSvc.Enabled) {
            Write-Error "Cannot shut '$Name' down from the host: the Shutdown integration service is unavailable or disabled (Windows setup has usually not finished installing it yet). Shut the guest down from inside Windows and re-run, or pass -AllowRunning."
        }

        Write-Host "Shutting '$Name' down for a cold baseline ..." -ForegroundColor Cyan
        Stop-VM -Name $Name -Force

        $deadline = (Get-Date).AddSeconds($ShutdownTimeoutSeconds)
        while ((Get-VM -Name $Name).State -ne 'Off' -and (Get-Date) -lt $deadline) {
            Start-Sleep -Seconds 3
        }
        if ((Get-VM -Name $Name).State -ne 'Off') {
            Write-Error "'$Name' did not shut down within $ShutdownTimeoutSeconds seconds - the guest may be holding a dialog open. Shut it down from inside Windows and re-run, raise -ShutdownTimeoutSeconds, or pass -AllowRunning."
        }
    }
}

# --- Checkpoint --------------------------------------------------------------
$vm = Get-VM -Name $Name
Write-Host ''
Write-Host ("Checkpointing '{0}' as '{1}' (state: {2}) ..." -f $Name, $SnapshotName, $vm.State) -ForegroundColor Cyan
Checkpoint-VM -Name $Name -SnapshotName $SnapshotName

$snap = Get-VMSnapshot -VMName $Name -Name $SnapshotName
Write-Host ''
Write-Host ("Checkpoint '{0}' taken at {1}." -f $snap.Name, $snap.CreationTime) -ForegroundColor Green

$all = @(Get-VMSnapshot -VMName $Name)
if ($all.Count -gt 1) {
    Write-Host ''
    Write-Host 'Checkpoints on this VM:' -ForegroundColor Cyan
    foreach ($s in $all) {
        Write-Host ("  {0,-24} {1}" -f $s.Name, $s.CreationTime) -ForegroundColor DarkGray
    }
}

# --- Next steps --------------------------------------------------------------
$copyScript = Join-Path $PSScriptRoot 'vm-scripts-copy.ps1'

Write-Host ''
Write-Host 'Next' -ForegroundColor Cyan
Write-Host @"
  1. Copy the README walkthrough into the guest and run the test:

       pwsh -File $copyScript -Name '$Name' -Start

     Then in the guest, follow README.md by hand (the honest test), or run
     ``bash /c/Users/Public/readme-steps.sh`` once Git Bash is installed.

  2. Reset for the next attempt - seconds, versus rebuilding from the ISO:

       pwsh -File scripts/vm-checkpoint-restore.ps1 -Name '$Name'

  Do not delete '$SnapshotName' by hand. Removing a checkpoint merges it into the
  parent disk, which destroys the reset point permanently. scripts/vm-machine-delete.ps1
  is the only thing that should ever take it away.
"@

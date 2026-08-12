<#
.SYNOPSIS
  Delete an install-test VM and every file it left behind.

.DESCRIPTION
  Step 8 of the install-test loop:

    scripts/vm-iso-download.ps1        # obtain a Windows 11 ISO
    scripts/vm-host-status.ps1         # read-only report - where does the host stand
    scripts/vm-hyperv-enable.ps1       # one-time host setup
    scripts/vm-machine-create.ps1      # build a clean VM from a Windows ISO
    scripts/vm-checkpoint-create.ps1   # freeze the clean install as 'clean-baseline'
    scripts/vm-scripts-copy.ps1        # copy the walkthrough into the guest
    scripts/readme-setup.ps1           # run INSIDE the VM: install Git Bash
    scripts/readme-steps.sh            # run INSIDE the VM: every README command
    scripts/vm-logs-copy.ps1           # pull the guest run logs back to the host
    scripts/vm-checkpoint-restore.ps1  # revert to the baseline for the next run
    scripts/vm-machine-delete.ps1      # this script
    scripts/vm-hyperv-disable.ps1      # turn Hyper-V back off

  This is destructive and irreversible: the VM, its checkpoints and its virtual
  disks are all removed. It prompts before doing anything unless you pass -Yes.

  If you only want a fresh Windows to test against again, you almost certainly
  want a revert instead - seconds rather than a full reinstall:

    pwsh -File scripts/vm-checkpoint-restore.ps1 -Name <name>

  Remove-VM on its own leaves the VHDX and any differencing disks on disk, so
  this script collects the disk paths first, merges checkpoints, and then cleans
  up the files.

.PARAMETER Name
  VM to delete. Defaults to gh-optivem-install-test.

.PARAMETER Yes
  Skip the confirmation prompt. For unattended use only.

.PARAMETER KeepDisks
  Remove the VM but leave its virtual disks on disk.

.EXAMPLE
  pwsh -File scripts/vm-machine-delete.ps1

.EXAMPLE
  pwsh -File scripts/vm-machine-delete.ps1 -Name my-other-vm -Yes
#>
[CmdletBinding()]
param(
    [string]$Name = 'gh-optivem-install-test',
    [switch]$Yes,
    [switch]$KeepDisks
)

$ErrorActionPreference = 'Stop'

function Test-Elevated {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal $id).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
}

if (-not (Test-Elevated)) {
    Write-Error "The Hyper-V cmdlets require an elevated shell. Open PowerShell as Administrator and re-run: pwsh -File `"$PSCommandPath`" -Name `"$Name`""
}

$vm = Get-VM -Name $Name -ErrorAction SilentlyContinue
if (-not $vm) {
    Write-Host "No VM named '$Name' - nothing to do." -ForegroundColor Yellow
    exit 0
}

# --- What is about to be destroyed -------------------------------------------
# Collected before Remove-VM, because afterwards there is nothing left to ask.
$disks       = @(Get-VMHardDiskDrive -VMName $Name | Select-Object -ExpandProperty Path)
$checkpoints = @(Get-VMSnapshot -VMName $Name -ErrorAction SilentlyContinue)
$vmRoot      = $vm.Path

Write-Host ''
Write-Host 'About to permanently delete:' -ForegroundColor Yellow
Write-Host ("  VM          : {0}  (state: {1})" -f $vm.Name, $vm.State)
Write-Host ("  Config path : {0}" -f $vmRoot)
if ($checkpoints) {
    Write-Host ("  Checkpoints : {0}" -f (($checkpoints | Select-Object -ExpandProperty Name) -join ', '))
} else {
    Write-Host '  Checkpoints : none'
}
if ($KeepDisks) {
    Write-Host '  Disks       : kept (-KeepDisks)'
    foreach ($d in $disks) { Write-Host ("                {0}" -f $d) -ForegroundColor DarkGray }
} elseif ($disks) {
    Write-Host '  Disks       : DELETED'
    foreach ($d in $disks) {
        $sizeGb = if (Test-Path -LiteralPath $d) { '{0:N1} GB' -f ((Get-Item -LiteralPath $d).Length / 1GB) } else { 'missing' }
        Write-Host ("                {0}  ({1})" -f $d, $sizeGb) -ForegroundColor DarkGray
    }
} else {
    Write-Host '  Disks       : none attached'
}
Write-Host ''

if (-not $Yes) {
    Write-Host "To keep the VM and just reset it to a clean state instead, cancel and run:" -ForegroundColor DarkGray
    Write-Host "  pwsh -File scripts/vm-checkpoint-restore.ps1 -Name '$Name'" -ForegroundColor DarkGray
    Write-Host ''
    $answer = Read-Host "Type the VM name ('$Name') to confirm deletion"
    if ($answer -ne $Name) {
        Write-Host 'Cancelled - nothing was deleted.' -ForegroundColor Green
        exit 0
    }
}

# --- Stop --------------------------------------------------------------------
# -TurnOff rather than a graceful shutdown: the guest is disposable, and a
# throwaway VM that ignores the shutdown request should not block cleanup.
if ($vm.State -ne 'Off') {
    Write-Host "Stopping '$Name' ..." -ForegroundColor Cyan
    Stop-VM -Name $Name -TurnOff -Force
}

# --- Merge checkpoints -------------------------------------------------------
# Each checkpoint parks the VM on a differencing .avhdx. Removing them merges
# those back into the parent VHDX; doing it before Remove-VM means the disk list
# collected above still describes real files afterwards.
if ($checkpoints) {
    Write-Host ("Removing {0} checkpoint(s) and merging differencing disks ..." -f $checkpoints.Count) -ForegroundColor Cyan
    Get-VMSnapshot -VMName $Name | Remove-VMSnapshot -IncludeAllChildSnapshots

    # The merge runs asynchronously; Remove-VM during a merge can orphan avhdx
    # files, so wait it out.
    $deadline = (Get-Date).AddMinutes(15)
    while ((Get-VM -Name $Name).Status -match 'merg' -and (Get-Date) -lt $deadline) {
        Write-Host '  merging ...' -ForegroundColor DarkGray
        Start-Sleep -Seconds 5
    }
    if ((Get-VM -Name $Name).Status -match 'merg') {
        Write-Warning 'Disk merge is still running after 15 minutes. Leaving the VM in place - re-run this script once Hyper-V Manager shows it idle.'
        exit 1
    }
}

# --- Remove ------------------------------------------------------------------
Write-Host "Removing VM '$Name' ..." -ForegroundColor Cyan
Remove-VM -Name $Name -Force

if (-not $KeepDisks) {
    foreach ($d in $disks) {
        if (Test-Path -LiteralPath $d) {
            Write-Host ("Deleting {0}" -f $d) -ForegroundColor Cyan
            Remove-Item -LiteralPath $d -Force
        }
    }
}

# Hyper-V leaves the VM's config directory behind. Remove it only if this VM was
# its sole occupant - shared VM roots hold other machines' configuration.
if ($vmRoot -and (Test-Path -LiteralPath $vmRoot)) {
    $leftovers = @(Get-ChildItem -LiteralPath $vmRoot -Recurse -File -ErrorAction SilentlyContinue)
    if ($leftovers.Count -eq 0) {
        Remove-Item -LiteralPath $vmRoot -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host ("Removed empty config directory {0}" -f $vmRoot) -ForegroundColor DarkGray
    } else {
        Write-Host ("Left {0} in place - it still contains {1} file(s)." -f $vmRoot, $leftovers.Count) -ForegroundColor DarkGray
    }
}

Write-Host ''
Write-Host "'$Name' deleted." -ForegroundColor Green
Write-Host "Rebuild with: pwsh -File scripts/vm-machine-create.ps1 -IsoPath <path-to-win11.iso>"

<#
.SYNOPSIS
  Turn Hyper-V back off — with a report of everything on this machine that will
  stop working when you do.

.DESCRIPTION
  The counterpart to scripts/vm-enable-hyperv.ps1, for when you are done with
  install testing and want the host back the way it was.

    scripts/vm-status.ps1            # read-only report — where does the host stand
    scripts/vm-enable-hyperv.ps1     # one-time host setup
    scripts/vm-create.ps1            # build a clean VM from a Windows ISO
    scripts/readme-steps.sh          # run INSIDE the VM: every README command
    scripts/vm-delete.ps1            # tear the VM down
    scripts/vm-disable-hyperv.ps1    # this script

  Read this before running it. Hyper-V is not only a VM host: it is the
  hypervisor that WSL 2, Docker Desktop, Windows Sandbox, Windows Subsystem for
  Android and Virtualization-Based Security all sit on top of. Turning it off
  breaks every one of them. This script inventories which of those you actually
  use and refuses to proceed silently.

  Two levers, and the difference matters:

    Feature removal (default)   Uninstalls Microsoft-Hyper-V-All. Reversible only
                                by re-running vm-enable-hyperv.ps1 and rebooting
                                again.

    -BootOnly                   Leaves the features installed but stops the
                                hypervisor from launching at boot
                                (bcdedit /set hypervisorlaunchtype off). One
                                command and a reboot to undo. Prefer this if you
                                are reclaiming resources temporarily rather than
                                uninstalling for good.

  VirtualMachinePlatform is deliberately left alone by default, because WSL 2
  requires it and most people who want "Hyper-V off" still want WSL. Pass
  -IncludeVirtualMachinePlatform to remove it too.

.PARAMETER Check
  Report the dependency inventory and change nothing. Does not require elevation.

.PARAMETER BootOnly
  Stop the hypervisor launching at boot instead of uninstalling the features.

.PARAMETER IncludeVirtualMachinePlatform
  Also disable VirtualMachinePlatform. This breaks WSL 2.

.PARAMETER Yes
  Skip the confirmation prompt.

.EXAMPLE
  pwsh -File scripts/vm-disable-hyperv.ps1 -Check

.EXAMPLE
  # In an elevated PowerShell — reversible route:
  pwsh -File scripts/vm-disable-hyperv.ps1 -BootOnly
#>
[CmdletBinding()]
param(
    [switch]$Check,
    [switch]$BootOnly,
    [switch]$IncludeVirtualMachinePlatform,
    [switch]$Yes
)

$ErrorActionPreference = 'Stop'

function Test-Elevated {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal $id).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
}

# CIM rather than Get-WindowsOptionalFeature so -Check works unelevated.
function Get-FeatureState([string]$name) {
    $f = Get-CimInstance Win32_OptionalFeature -Filter "Name='$name'" -ErrorAction SilentlyContinue
    if (-not $f) { return 'NotPresent' }
    switch ($f.InstallState) { 1 { 'Enabled' } 2 { 'Disabled' } 3 { 'Absent' } default { 'Unknown' } }
}

Write-Host ''
Write-Host 'Current state' -ForegroundColor Cyan
$hypervisorRunning = (Get-CimInstance Win32_ComputerSystem).HypervisorPresent
Write-Host ("  Hypervisor running     : {0}" -f $hypervisorRunning)
Write-Host ("  Microsoft-Hyper-V-All  : {0}" -f (Get-FeatureState 'Microsoft-Hyper-V-All'))
Write-Host ("  VirtualMachinePlatform : {0}" -f (Get-FeatureState 'VirtualMachinePlatform'))

# --- What breaks -------------------------------------------------------------
$blockers = @()
$warnings = @()

# VMs. Disabling the hypervisor does not delete them, but they become
# unstartable and unmanageable until it is turned back on.
try {
    $vms = @(Get-VM -ErrorAction Stop)
    if ($vms.Count -gt 0) {
        $running = @($vms | Where-Object State -ne 'Off')
        $blockers += ("{0} Hyper-V VM(s) still defined: {1}" -f $vms.Count, (($vms | Select-Object -ExpandProperty Name) -join ', '))
        if ($running.Count -gt 0) {
            $blockers += ("{0} of them are not powered off" -f $running.Count)
        }
    }
} catch {
    # Module absent or unelevated — nothing useful to say, and not a blocker.
}

# WSL 2 rides on VirtualMachinePlatform + the hypervisor. wsl.exe writes UTF-16LE,
# which the console decodes as NUL-separated characters unless told otherwise.
try {
    $prevEncoding = [Console]::OutputEncoding
    [Console]::OutputEncoding = [Text.Encoding]::Unicode
    try {
        $distros = @(& wsl.exe -l -q 2>$null |
            ForEach-Object { ($_ -replace "`0", '').Trim() } |
            Where-Object { $_ })
    } finally {
        [Console]::OutputEncoding = $prevEncoding
    }
    if ($distros.Count -gt 0) {
        $warnings += ("WSL distributions will stop starting: {0}" -f ($distros -join ', '))
    }
} catch { }

# Docker Desktop's default backend is WSL 2; the Hyper-V backend is the hypervisor
# directly. Either way it stops working.
if (Get-Command docker -ErrorAction SilentlyContinue) {
    $warnings += 'Docker Desktop will not start (both its WSL 2 and Hyper-V backends need the hypervisor)'
}

# Windows Sandbox is a Hyper-V container.
if ((Get-FeatureState 'Containers-DisposableClientVM') -eq 'Enabled') {
    $warnings += 'Windows Sandbox will stop working'
}

# VBS / Memory Integrity keeps the hypervisor loaded regardless of the Hyper-V
# feature, so disabling Hyper-V will not actually reclaim the boot hypervisor.
try {
    $dg = Get-CimInstance -Namespace 'root\Microsoft\Windows\DeviceGuard' -ClassName Win32_DeviceGuard -ErrorAction Stop
    if ($dg.SecurityServicesRunning -and $dg.SecurityServicesRunning.Count -gt 0) {
        $warnings += 'Virtualization-Based Security (Core Isolation / Memory Integrity) is active — it keeps the hypervisor loaded even after Hyper-V is removed, so this will not free the virtualization overhead unless you also turn Memory Integrity off in Windows Security'
    }
} catch { }

if ($warnings) {
    Write-Host ''
    Write-Host 'This will break' -ForegroundColor Yellow
    foreach ($w in $warnings) { Write-Host "  - $w" -ForegroundColor Yellow }
}

if ($blockers) {
    Write-Host ''
    Write-Host 'Blocking' -ForegroundColor Red
    foreach ($b in $blockers) { Write-Host "  - $b" -ForegroundColor Red }
    Write-Host ''
    Write-Host 'Delete them first (scripts/vm-delete.ps1), or pass -Yes to leave them stranded until Hyper-V is re-enabled.' -ForegroundColor DarkGray
}

if (-not $warnings -and -not $blockers) {
    Write-Host ''
    Write-Host 'Nothing on this machine appears to depend on the hypervisor.' -ForegroundColor Green
}

if ($Check) {
    Write-Host ''
    Write-Host 'Report only (-Check) — nothing was changed.'
    exit 0
}

if ($blockers -and -not $Yes) {
    Write-Error 'Refusing to proceed while VMs are still defined. Delete them with scripts/vm-delete.ps1, or re-run with -Yes to override.'
}

if (-not (Test-Elevated)) {
    Write-Error "Disabling Hyper-V requires an elevated shell. Open PowerShell as Administrator and re-run: pwsh -File `"$PSCommandPath`""
}

# --- Confirm -----------------------------------------------------------------
$plan = if ($BootOnly) {
    'Stop the hypervisor launching at boot (features stay installed)'
} else {
    $f = @('Microsoft-Hyper-V-All')
    if ($IncludeVirtualMachinePlatform) { $f += 'VirtualMachinePlatform' }
    "Uninstall Windows feature(s): $($f -join ', ')"
}

Write-Host ''
Write-Host "Plan: $plan" -ForegroundColor Cyan
if (-not $BootOnly) {
    Write-Host 'Reversible with: pwsh -File scripts/vm-enable-hyperv.ps1 (plus a reboot)' -ForegroundColor DarkGray
}

if (-not $Yes) {
    Write-Host ''
    $answer = Read-Host "Type 'disable' to confirm"
    if ($answer -ne 'disable') {
        Write-Host 'Cancelled — nothing was changed.' -ForegroundColor Green
        exit 0
    }
}

# --- Apply -------------------------------------------------------------------
if ($BootOnly) {
    Write-Host 'Setting hypervisorlaunchtype off ...' -ForegroundColor Cyan
    & bcdedit.exe /set hypervisorlaunchtype off
    if ($LASTEXITCODE -ne 0) { Write-Error "bcdedit failed with exit code $LASTEXITCODE" }
    Write-Host ''
    Write-Host 'Done. Reboot to take effect.' -ForegroundColor Green
    Write-Host 'Undo with: bcdedit /set hypervisorlaunchtype auto   (then reboot)'
    exit 0
}

$targets = @('Microsoft-Hyper-V-All')
if ($IncludeVirtualMachinePlatform) { $targets += 'VirtualMachinePlatform' }

$rebootNeeded = $false
foreach ($name in $targets) {
    if ((Get-FeatureState $name) -ne 'Enabled') {
        Write-Host "$name is already disabled — skipping." -ForegroundColor DarkGray
        continue
    }
    Write-Host "Disabling $name ..." -ForegroundColor Cyan
    $result = Disable-WindowsOptionalFeature -Online -FeatureName $name -NoRestart
    if ($result.RestartNeeded) { $rebootNeeded = $true }
}

Write-Host ''
if ($rebootNeeded) {
    Write-Host 'Disabled. Reboot to take effect.' -ForegroundColor Yellow
} else {
    Write-Host 'Disabled.' -ForegroundColor Green
}
Write-Host 'Re-enable with: pwsh -File scripts/vm-enable-hyperv.ps1'

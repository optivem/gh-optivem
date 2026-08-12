<#
.SYNOPSIS
  Check — and optionally enable — the Hyper-V host features needed to run the
  install-test VM.

.DESCRIPTION
  Step 1 of the install-test loop:

    scripts/vm-iso-download.ps1        # obtain a Windows 11 ISO
    scripts/vm-host-status.ps1         # read-only report — where does the host stand
    scripts/vm-hyperv-enable.ps1       # this script — one-time host setup
    scripts/vm-machine-create.ps1      # build a clean VM from a Windows ISO
    scripts/vm-checkpoint-create.ps1   # freeze the clean install as 'clean-baseline'
    scripts/readme-steps.sh            # run INSIDE the VM: every README command
    scripts/vm-checkpoint-restore.ps1  # revert to the baseline for the next run
    scripts/vm-machine-delete.ps1      # tear the VM down
    scripts/vm-hyperv-disable.ps1      # turn Hyper-V back off

  Reporting is unprivileged; enabling is not. Run with -Check from any shell to
  see where you stand, then re-run elevated (no -Check) to turn anything missing
  on.

.PARAMETER Check
  Report only — never change anything. Does not require an elevated shell.

.EXAMPLE
  pwsh -File scripts/vm-hyperv-enable.ps1 -Check

.EXAMPLE
  # In an elevated PowerShell:
  pwsh -File scripts/vm-hyperv-enable.ps1
#>
[CmdletBinding()]
param(
    [switch]$Check
)

$ErrorActionPreference = 'Stop'

# Everything the install-test VM depends on. Hyper-V-All pulls in the hypervisor,
# the management tools and the PowerShell module; VirtualMachinePlatform is what
# nested virtualization (and therefore Docker Desktop inside the guest) rides on.
$features = @(
    @{ Name = 'Microsoft-Hyper-V-All';  Why = 'the hypervisor, Hyper-V Manager and the PowerShell module' }
    @{ Name = 'VirtualMachinePlatform'; Why = 'nested virtualization, so Docker Desktop runs inside the guest' }
)

function Test-Elevated {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal $id).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
}

# Get-WindowsOptionalFeature needs elevation; the CIM class does not, which is
# what lets -Check run from an ordinary shell.
function Get-FeatureState([string]$name) {
    $f = Get-CimInstance Win32_OptionalFeature -Filter "Name='$name'" -ErrorAction SilentlyContinue
    if (-not $f) { return 'NotPresent' }
    switch ($f.InstallState) {
        1       { 'Enabled' }
        2       { 'Disabled' }
        3       { 'Absent' }
        default { 'Unknown' }
    }
}

# --- Hardware capability -----------------------------------------------------
# Once Hyper-V is running the host OS is itself a guest, so Win32_Processor stops
# reporting the virtualization extensions honestly. HypervisorPresent is the
# reliable "already on" signal; the CPU flags only mean anything before that.
$cs  = Get-CimInstance Win32_ComputerSystem
$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1

Write-Host ''
Write-Host 'Host' -ForegroundColor Cyan
Write-Host ("  Windows edition      : {0}" -f (Get-CimInstance Win32_OperatingSystem).Caption)
Write-Host ("  Memory               : {0:N0} GB" -f ($cs.TotalPhysicalMemory / 1GB))
Write-Host ("  Hypervisor running   : {0}" -f $cs.HypervisorPresent)

if (-not $cs.HypervisorPresent) {
    Write-Host ("  CPU virtualization   : {0}" -f $cpu.VirtualizationFirmwareEnabled)
    Write-Host ("  SLAT (required)      : {0}" -f $cpu.SecondLevelAddressTranslationExtensions)
    if ($cpu.VirtualizationFirmwareEnabled -eq $false) {
        Write-Warning 'Virtualization is disabled in firmware. Enable VT-x / AMD-V (often "SVM Mode" or "Intel Virtualization Technology") in the BIOS/UEFI first — no amount of Windows configuration substitutes for it.'
    }
}

# --- Feature state -----------------------------------------------------------
Write-Host ''
Write-Host 'Features' -ForegroundColor Cyan

$missing = @()
foreach ($f in $features) {
    $state = Get-FeatureState $f.Name
    $ok    = $state -eq 'Enabled'
    $color = if ($ok) { 'Green' } else { 'Yellow' }
    Write-Host ("  {0,-28} : {1}" -f $f.Name, $state) -ForegroundColor $color
    Write-Host ("  {0,-28}   ({1})" -f '', $f.Why) -ForegroundColor DarkGray
    if (-not $ok) { $missing += $f.Name }
}

Write-Host ''

if ($missing.Count -eq 0) {
    Write-Host 'All required features are enabled — next: scripts/vm-machine-create.ps1' -ForegroundColor Green
    exit 0
}

if ($Check) {
    Write-Host ("Missing: {0}" -f ($missing -join ', ')) -ForegroundColor Yellow
    Write-Host 'Re-run this script from an elevated PowerShell (without -Check) to enable them.'
    exit 1
}

if (-not (Test-Elevated)) {
    Write-Error @"
Enabling Windows features requires an elevated shell.

Open PowerShell as Administrator and re-run:
  pwsh -File "$PSCommandPath"

Or run with -Check to report the current state without elevation.
"@
}

# --- Enable ------------------------------------------------------------------
# -NoRestart so both features go on in one pass and the reboot happens once, at
# the end, rather than mid-list.
$rebootNeeded = $false
foreach ($name in $missing) {
    Write-Host "Enabling $name ..." -ForegroundColor Cyan
    $result = Enable-WindowsOptionalFeature -Online -FeatureName $name -All -NoRestart
    if ($result.RestartNeeded) { $rebootNeeded = $true }
}

Write-Host ''
if ($rebootNeeded) {
    Write-Host 'Enabled. A reboot is required before the Hyper-V cmdlets will work.' -ForegroundColor Yellow
    Write-Host 'Reboot, then run: pwsh -File scripts/vm-machine-create.ps1 -IsoPath <path-to-win11.iso>'
} else {
    Write-Host 'Enabled — next: scripts/vm-machine-create.ps1' -ForegroundColor Green
}

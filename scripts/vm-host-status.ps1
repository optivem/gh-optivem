<#
.SYNOPSIS
  Report whether Hyper-V is enabled on this host, and what is running on it.

.DESCRIPTION
  Read-only. Changes nothing, needs no elevation. Run it any time you want to
  know where the host stands before reaching for one of the scripts that does
  change something:

    scripts/vm-iso-download.ps1        # obtain a Windows 11 ISO
    scripts/vm-host-status.ps1         # this script - read-only report
    scripts/vm-hyperv-enable.ps1       # one-time host setup
    scripts/vm-machine-create.ps1      # build a clean VM from a Windows ISO
    scripts/vm-checkpoint-create.ps1   # freeze the clean install as 'clean-baseline'
    scripts/readme-steps.sh            # run INSIDE the VM: every README command
    scripts/vm-checkpoint-restore.ps1  # revert to the baseline for the next run
    scripts/vm-machine-delete.ps1      # tear the VM down
    scripts/vm-hyperv-disable.ps1      # turn Hyper-V back off

  "Enabled" is two separate questions and this reports both, because they can
  disagree: the Windows feature can be installed while the hypervisor is
  configured not to launch at boot (bcdedit hypervisorlaunchtype off), and
  Virtualization-Based Security can keep the hypervisor loaded even with the
  Hyper-V feature removed.

.PARAMETER Quiet
  Print nothing; just set the exit code.

.OUTPUTS
  Exit code 0 if Hyper-V is enabled and the hypervisor is running, 1 otherwise -
  so this can gate other scripts.

.EXAMPLE
  pwsh -File scripts/vm-host-status.ps1

.EXAMPLE
  pwsh -File scripts/vm-host-status.ps1 -Quiet; if ($LASTEXITCODE -eq 0) { 'ready' }
#>
[CmdletBinding()]
param(
    [switch]$Quiet
)

$ErrorActionPreference = 'Stop'

function Write-Line($text, $color = 'Gray') {
    if (-not $Quiet) { Write-Host $text -ForegroundColor $color }
}

# Win32_OptionalFeature rather than Get-WindowsOptionalFeature: same answer,
# no elevation required.
function Get-FeatureState([string]$name) {
    $f = Get-CimInstance Win32_OptionalFeature -Filter "Name='$name'" -ErrorAction SilentlyContinue
    if (-not $f) { return 'NotPresent' }
    switch ($f.InstallState) { 1 { 'Enabled' } 2 { 'Disabled' } 3 { 'Absent' } default { 'Unknown' } }
}

function Test-Elevated {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal $id).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
}

# --- Host --------------------------------------------------------------------
$os                = Get-CimInstance Win32_OperatingSystem
$cs                = Get-CimInstance Win32_ComputerSystem
$hypervisorRunning = [bool]$cs.HypervisorPresent
$sysDrive          = Get-PSDrive -Name ($env:SystemDrive.TrimEnd(':'))

Write-Line ''
Write-Line 'Host' 'Cyan'
Write-Line ("  Windows              : {0}" -f $os.Caption)
Write-Line ("  Memory               : {0:N0} GB" -f ($cs.TotalPhysicalMemory / 1GB))
Write-Line ("  Free on {0,-12} : {1:N0} GB" -f $env:SystemDrive, ($sysDrive.Free / 1GB))

# Hyper-V needs a Pro/Enterprise/Education SKU; Home cannot run it at all.
if ($os.Caption -match 'Home') {
    Write-Line '  NOTE: Windows Home cannot run Hyper-V. Use Windows Sandbox or a third-party hypervisor (VirtualBox, VMware Workstation Player).' 'Yellow'
}

# --- Features ----------------------------------------------------------------
$hyperV  = Get-FeatureState 'Microsoft-Hyper-V-All'
$vmp     = Get-FeatureState 'VirtualMachinePlatform'
$sandbox = Get-FeatureState 'Containers-DisposableClientVM'

Write-Line ''
Write-Line 'Features' 'Cyan'
foreach ($row in @(
    @{ Label = 'Microsoft-Hyper-V-All';        State = $hyperV;  Note = 'hypervisor + Hyper-V Manager + PowerShell module' }
    @{ Label = 'VirtualMachinePlatform';       State = $vmp;     Note = 'nested virtualization and WSL 2' }
    @{ Label = 'Containers-DisposableClientVM'; State = $sandbox; Note = 'Windows Sandbox' }
)) {
    $color = if ($row.State -eq 'Enabled') { 'Green' } else { 'Yellow' }
    Write-Line ("  {0,-30} : {1}" -f $row.Label, $row.State) $color
    Write-Line ("  {0,-30}   ({1})" -f '', $row.Note) 'DarkGray'
}

# --- Hypervisor --------------------------------------------------------------
Write-Line ''
Write-Line 'Hypervisor' 'Cyan'
Write-Line ("  Running now          : {0}" -f $hypervisorRunning) $(if ($hypervisorRunning) { 'Green' } else { 'Yellow' })

# Boot policy. The feature being installed does not guarantee the hypervisor
# launches - hypervisorlaunchtype is the switch that decides.
try {
    $bcd = & bcdedit.exe /enum '{current}' 2>$null
    $launch = ($bcd | Select-String -Pattern 'hypervisorlaunchtype').ToString()
    if ($launch) {
        Write-Line ("  Boot policy          : {0}" -f ($launch -replace '\s+', ' ').Trim())
    } else {
        Write-Line '  Boot policy          : auto (default - not explicitly set)'
    }
} catch {
    Write-Line '  Boot policy          : unreadable (bcdedit needs elevation)' 'DarkGray'
}

if (-not $hypervisorRunning) {
    # These CPU flags only report honestly while the host is not itself a guest.
    $cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
    Write-Line ("  CPU virtualization   : {0}" -f $cpu.VirtualizationFirmwareEnabled)
    Write-Line ("  SLAT (required)      : {0}" -f $cpu.SecondLevelAddressTranslationExtensions)
    if ($cpu.VirtualizationFirmwareEnabled -eq $false) {
        Write-Line '  Virtualization is off in firmware - enable VT-x / AMD-V ("SVM Mode") in BIOS/UEFI first.' 'Yellow'
    }
}

# VBS keeps the hypervisor loaded independently of the Hyper-V feature, which is
# why "Hyper-V disabled but hypervisor running" is a real and confusing state.
try {
    $dg = Get-CimInstance -Namespace 'root\Microsoft\Windows\DeviceGuard' -ClassName Win32_DeviceGuard -ErrorAction Stop
    if ($dg.SecurityServicesRunning -and $dg.SecurityServicesRunning.Count -gt 0) {
        Write-Line '  VBS / Memory Integrity is active - it loads the hypervisor regardless of the Hyper-V feature.' 'DarkGray'
    }
} catch { }

# --- VMs ---------------------------------------------------------------------
Write-Line ''
Write-Line 'Virtual machines' 'Cyan'
if ($hyperV -ne 'Enabled') {
    Write-Line '  (Hyper-V not enabled)' 'DarkGray'
} elseif (-not (Get-Module -ListAvailable -Name Hyper-V)) {
    Write-Line '  (Hyper-V PowerShell module not installed)' 'DarkGray'
} else {
    try {
        $vms = @(Get-VM -ErrorAction Stop)
        if ($vms.Count -eq 0) {
            Write-Line '  none' 'DarkGray'
        } else {
            foreach ($vm in $vms) {
                $snaps = @(Get-VMSnapshot -VMName $vm.Name -ErrorAction SilentlyContinue)
                $snapText = if ($snaps.Count -gt 0) { ($snaps | Select-Object -ExpandProperty Name) -join ', ' } else { 'no checkpoints' }
                $color = if ($vm.State -eq 'Running') { 'Green' } else { 'Gray' }
                Write-Line ("  {0,-32} {1,-9} {2:N0} GB RAM, {3} CPU  [{4}]" -f $vm.Name, $vm.State, ($vm.MemoryStartup / 1GB), $vm.ProcessorCount, $snapText) $color
            }
        }
    } catch {
        if (Test-Elevated) {
            Write-Line ("  (could not enumerate: {0})" -f $_.Exception.Message) 'DarkGray'
        } else {
            Write-Line '  (needs an elevated shell, or membership of the Hyper-V Administrators group)' 'DarkGray'
        }
    }
}

# --- Verdict -----------------------------------------------------------------
$ready = ($hyperV -eq 'Enabled') -and $hypervisorRunning

Write-Line ''
if ($ready) {
    Write-Line 'Hyper-V is ENABLED and the hypervisor is running.' 'Green'
    Write-Line '  Create a test VM : pwsh -File scripts/vm-machine-create.ps1 -IsoPath <win11.iso>   (elevated)'
    Write-Line '  Turn it back off : pwsh -File scripts/vm-hyperv-disable.ps1 -Check'
} elseif ($hyperV -eq 'Enabled' -and -not $hypervisorRunning) {
    Write-Line 'Hyper-V is INSTALLED but the hypervisor is not running.' 'Yellow'
    Write-Line '  Usually a pending reboot, or hypervisorlaunchtype set to off above.'
    Write-Line '  Fix: bcdedit /set hypervisorlaunchtype auto   (elevated, then reboot)'
} else {
    Write-Line 'Hyper-V is DISABLED.' 'Yellow'
    Write-Line '  Enable it: pwsh -File scripts/vm-hyperv-enable.ps1   (elevated)'
}

exit $(if ($ready) { 0 } else { 1 })

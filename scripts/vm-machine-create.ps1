<#
.SYNOPSIS
  Create a clean Hyper-V VM from a retail Windows ISO, for testing the README
  install path the way a brand-new user experiences it.

.DESCRIPTION
  Step 2 of the install-test loop:

    scripts/vm-iso-download.ps1        # obtain a Windows 11 ISO
    scripts/vm-host-status.ps1         # read-only report — where does the host stand
    scripts/vm-hyperv-enable.ps1       # one-time host setup
    scripts/vm-machine-create.ps1      # this script
    scripts/vm-checkpoint-create.ps1   # freeze the clean install as 'clean-baseline'
    scripts/readme-steps.sh            # run INSIDE the VM: every README command
    scripts/vm-checkpoint-restore.ps1  # revert to the baseline for the next run
    scripts/vm-machine-delete.ps1      # tear the VM down
    scripts/vm-hyperv-disable.ps1      # turn Hyper-V back off

  Use a plain Windows ISO from https://www.microsoft.com/software-download/windows11
  — NOT Hyper-V Quick Create's "Windows 11 dev environment" image, which ships
  with developer tooling preinstalled and would silently satisfy the very
  prerequisites this exercise exists to test.

  The VM is configured for a realistic run:

    - nested virtualization on, so Docker Desktop works in the guest
    - vTPM + Secure Boot, which Windows 11 setup requires
    - standard (not production) checkpoints, so a revert restores the disk
      byte-for-byte rather than replaying a VSS-consistent snapshot
    - automatic checkpoints off, so your clean baseline is the only one

  Nothing is installed into the guest — that is the point. Once Windows setup
  finishes, take the baseline checkpoint (command printed at the end) and only
  then start following the README.

.PARAMETER IsoPath
  Path to a Windows 11 installation ISO. Required.

.PARAMETER Name
  VM name. Defaults to gh-optivem-install-test.

.PARAMETER MemoryGB
  Startup memory, allocated statically. Docker Desktop in the guest wants room;
  8 GB is the practical floor.

.PARAMETER CpuCount
  Virtual processors.

.PARAMETER DiskGB
  Size of the dynamically-expanding VHDX. It only consumes what the guest writes.

.PARAMETER VhdPath
  Where to put the VHDX. Defaults to the Hyper-V host's configured virtual hard
  disk path, so nothing here is machine-specific.

.PARAMETER SwitchName
  Virtual switch to attach. "Default Switch" is the NAT switch present on
  Windows client SKUs and needs no setup.

.PARAMETER Force
  Replace an existing VM of the same name (deletes it first, disks and all).

.EXAMPLE
  pwsh -File scripts/vm-machine-create.ps1 -IsoPath D:\iso\Win11_24H2_English_x64.iso

.EXAMPLE
  pwsh -File scripts/vm-machine-create.ps1 -IsoPath D:\iso\Win11.iso -MemoryGB 12 -CpuCount 6
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$IsoPath,

    [string]$Name       = 'gh-optivem-install-test',
    [int]   $MemoryGB   = 8,
    [int]   $CpuCount   = 4,
    [int]   $DiskGB     = 80,
    [string]$VhdPath,
    [string]$SwitchName = 'Default Switch',
    [switch]$Force
)

$ErrorActionPreference = 'Stop'

function Test-Elevated {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal $id).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
}

# --- Preconditions -----------------------------------------------------------
if (-not (Test-Elevated)) {
    Write-Error "The Hyper-V cmdlets require an elevated shell. Open PowerShell as Administrator and re-run: pwsh -File `"$PSCommandPath`" -IsoPath `"$IsoPath`""
}

if (-not (Get-Module -ListAvailable -Name Hyper-V)) {
    Write-Error 'The Hyper-V PowerShell module is not present. Run scripts/vm-hyperv-enable.ps1 first (and reboot if it asks you to).'
}

if (-not (Test-Path -LiteralPath $IsoPath -PathType Leaf)) {
    Write-Error @"
ISO not found: $IsoPath

Download a plain Windows 11 ISO from:
  https://www.microsoft.com/software-download/windows11

Do not substitute the Quick Create "Windows 11 dev environment" image — it has
developer tools preinstalled, which defeats the purpose of a clean install test.
"@
}
$IsoPath = (Resolve-Path -LiteralPath $IsoPath).Path

if (-not (Get-VMSwitch -Name $SwitchName -ErrorAction SilentlyContinue)) {
    $available = (Get-VMSwitch | Select-Object -ExpandProperty Name) -join ', '
    Write-Error "Virtual switch '$SwitchName' not found. Available: $available. Pass -SwitchName with one of those, or create a NAT/external switch in Hyper-V Manager."
}

$existing = Get-VM -Name $Name -ErrorAction SilentlyContinue
if ($existing) {
    if (-not $Force) {
        Write-Error "A VM named '$Name' already exists. Delete it with scripts/vm-machine-delete.ps1 -Name '$Name', pass -Name to use a different name, or re-run with -Force to replace it."
    }
    Write-Host "Removing existing VM '$Name' (-Force) ..." -ForegroundColor Yellow
    & (Join-Path $PSScriptRoot 'vm-machine-delete.ps1') -Name $Name -Yes
}

# --- Paths -------------------------------------------------------------------
# Resolved from the Hyper-V host config rather than hardcoded, so this works on
# any machine regardless of where its VM storage lives.
if (-not $VhdPath) {
    $vhdDir  = (Get-VMHost).VirtualHardDiskPath
    $VhdPath = Join-Path $vhdDir "$Name.vhdx"
}
$vhdParent = Split-Path -Parent $VhdPath
if (-not (Test-Path -LiteralPath $vhdParent)) {
    New-Item -ItemType Directory -Path $vhdParent -Force | Out-Null
}
if (Test-Path -LiteralPath $VhdPath) {
    Write-Error "A virtual disk already exists at $VhdPath. Delete it, or pass -VhdPath to write elsewhere."
}

# --- Create ------------------------------------------------------------------
Write-Host ''
Write-Host "Creating VM '$Name'" -ForegroundColor Cyan
Write-Host ("  ISO      : {0}" -f $IsoPath)
Write-Host ("  Disk     : {0} ({1} GB, dynamically expanding)" -f $VhdPath, $DiskGB)
Write-Host ("  Memory   : {0} GB (static)" -f $MemoryGB)
Write-Host ("  CPUs     : {0}" -f $CpuCount)
Write-Host ("  Switch   : {0}" -f $SwitchName)
Write-Host ''

# Generation 2 is required for the UEFI + Secure Boot + vTPM combination that
# Windows 11 setup checks for.
New-VM -Name $Name `
       -Generation 2 `
       -MemoryStartupBytes ($MemoryGB * 1GB) `
       -NewVHDPath $VhdPath `
       -NewVHDSizeBytes ($DiskGB * 1GB) `
       -SwitchName $SwitchName | Out-Null

# Dynamic memory makes the guest's available RAM move under it; for a repeatable
# install test a fixed allocation is one less variable.
Set-VMMemory -VMName $Name -DynamicMemoryEnabled $false

# ExposeVirtualizationExtensions is what makes Docker Desktop possible in the
# guest. MAC spoofing lets the guest's own nested VMs reach the network.
Set-VMProcessor -VMName $Name -Count $CpuCount -ExposeVirtualizationExtensions $true
Set-VMNetworkAdapter -VMName $Name -MacAddressSpoofing On

# Standard checkpoints capture memory and disk exactly as they were; production
# checkpoints ask the guest for a VSS-consistent snapshot instead, which is the
# wrong tool for "put this machine back exactly how it was".
Set-VM -Name $Name -CheckpointType Standard -AutomaticCheckpointsEnabled $false

# Guest Service Interface is off by default; it enables Copy-VMFile, which is how
# you get scripts/readme-steps.sh into the guest without a network share.
Enable-VMIntegrationService -VMName $Name -Name 'Guest Service Interface'

# vTPM. The local key protector is generated per-host; on machines where the
# Host Guardian client is unconfigured this can fail, and Windows 11 setup will
# then refuse to proceed without a registry bypass.
try {
    Set-VMKeyProtector -VMName $Name -NewLocalKeyProtector
    Enable-VMTPM -VMName $Name
    Write-Host '  vTPM enabled.' -ForegroundColor DarkGray
} catch {
    Write-Warning "Could not enable the virtual TPM: $($_.Exception.Message)"
    Write-Warning 'Windows 11 setup will show "This PC can''t run Windows 11". Shift+F10 at that screen, run regedit, and under HKLM\SYSTEM\Setup\LabConfig add DWORD BypassTPMCheck=1 and BypassSecureBootCheck=1.'
}

# Boot the ISO ahead of the (empty) disk for the initial install.
$dvd = Add-VMDvdDrive -VMName $Name -Path $IsoPath -Passthru
Set-VMFirmware -VMName $Name -FirstBootDevice $dvd -EnableSecureBoot On

Write-Host ''
Write-Host "VM '$Name' created." -ForegroundColor Green

# --- Next steps --------------------------------------------------------------
Write-Host ''
Write-Host 'Next' -ForegroundColor Cyan
Write-Host @"
  1. Start it and open the console:

       Start-VM -Name '$Name'
       vmconnect.exe localhost '$Name'

     Watch for "Press any key to boot from CD or DVD" — it waits only a few
     seconds, and missing it drops you to a boot failure. Just reset and retry.

  2. Install Windows. Take the offline/local-account path if offered: a Microsoft
     account can sync settings and apps from your real machine, which is exactly
     the contamination you are trying to avoid. Install nothing else.

  3. Snapshot that clean state — this is what makes the loop repeatable. Decide
     about Windows Update BEFORE this point; whatever you freeze is where every
     later run starts:

       pwsh -File scripts/vm-checkpoint-create.ps1 -Name '$Name'

  4. Copy the README walkthrough in and run it:

       Copy-VMFile -Name '$Name' -SourcePath '$(Join-Path $PSScriptRoot 'readme-steps.sh')' ``
         -DestinationPath 'C:\Users\Public\readme-steps.sh' -CreateFullPath -FileSource Host

     Then in the guest, follow README.md by hand (the honest test), or run
     ``bash readme-steps.sh`` once Git Bash is installed.

  5. Reset for the next attempt — seconds, versus rebuilding from the ISO. The
     checkpoint is not consumed, so this is repeatable indefinitely:

       pwsh -File scripts/vm-checkpoint-restore.ps1 -Name '$Name'

  6. When you are done with the VM entirely:

       pwsh -File scripts/vm-machine-delete.ps1 -Name '$Name'
"@

<#
.SYNOPSIS
  Copy the README walkthrough (scripts/readme-steps.sh) from the host into the
  running test VM, so you can run it inside the guest.

.DESCRIPTION
  Step 4 of the install-test loop:

    scripts/vm-iso-download.ps1        # obtain a Windows 11 ISO
    scripts/vm-host-status.ps1         # read-only report - where does the host stand
    scripts/vm-hyperv-enable.ps1       # one-time host setup
    scripts/vm-machine-create.ps1      # build a clean VM from a Windows ISO
    scripts/vm-checkpoint-create.ps1   # freeze the clean install as 'clean-baseline'
    scripts/vm-steps-copy.ps1          # this script
    scripts/readme-steps.sh            # run INSIDE the VM: every README command
    scripts/vm-checkpoint-restore.ps1  # revert to the baseline for the next run
    scripts/vm-machine-delete.ps1      # tear the VM down
    scripts/vm-hyperv-disable.ps1      # turn Hyper-V back off

  The guest has no network share, no clipboard you can trust for a 300-line
  script, and (by design) no clone of this repo. Copy-VMFile pushes the file over
  the VMBus instead, which is why vm-machine-create.ps1 turns the Guest Service
  Interface on. This script is that one command plus the checks that turn its
  three unhelpful failure modes - VM off, integration service not answering,
  destination already there - into messages that say what to do.

  Re-run it after every host-side edit to readme-steps.sh. The copy is a
  snapshot, not a mount: the guest keeps whatever bytes it received until you
  overwrite them with -Force.

  What lands in the guest is only half the test. readme-steps.sh is the
  mechanical version of README.md; following the README by hand in the guest is
  the honest one, because it also tests the prose.

.PARAMETER Name
  VM to copy into. Defaults to gh-optivem-install-test.

.PARAMETER SourcePath
  File on the host to copy. Defaults to scripts/readme-steps.sh next to this
  script.

.PARAMETER DestinationPath
  Absolute path inside the guest. Defaults to C:\Users\Public\readme-steps.sh,
  which every guest account can read.

.PARAMETER Force
  Overwrite the file if it is already at the destination. Without this, a second
  run fails rather than silently replacing what the guest has.

.PARAMETER Start
  Start the VM first if it is off, and wait for its guest services to answer.

.PARAMETER WaitTimeoutSeconds
  How long to wait for the Guest Service Interface to report OK before giving up.

.EXAMPLE
  pwsh -File scripts/vm-steps-copy.ps1

.EXAMPLE
  pwsh -File scripts/vm-steps-copy.ps1 -Force

.EXAMPLE
  pwsh -File scripts/vm-steps-copy.ps1 -SourcePath .\README.md -DestinationPath 'C:\Users\Public\README.md'
#>
[CmdletBinding()]
param(
    [string]$Name               = 'gh-optivem-install-test',
    [string]$SourcePath         = '',
    [string]$DestinationPath    = 'C:\Users\Public\readme-steps.sh',
    [switch]$Force,
    [switch]$Start,
    [int]   $WaitTimeoutSeconds = 180
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

if (-not $SourcePath) {
    $SourcePath = Join-Path $PSScriptRoot 'readme-steps.sh'
}
if (-not (Test-Path -LiteralPath $SourcePath -PathType Leaf)) {
    Write-Error "No file at '$SourcePath'. Pass -SourcePath with the file you want in the guest."
}
$source = (Resolve-Path -LiteralPath $SourcePath).Path

if (-not [System.IO.Path]::IsPathRooted($DestinationPath)) {
    Write-Error "-DestinationPath must be an absolute path inside the guest (for example C:\Users\Public\readme-steps.sh); got '$DestinationPath'."
}

$vm = Get-VM -Name $Name -ErrorAction SilentlyContinue
if (-not $vm) {
    Write-Error "No VM named '$Name'. Build one first: pwsh -File scripts/vm-machine-create.ps1 -IsoPath <path-to-win11.iso>"
}

# A shell script that reaches the guest with CRLF endings dies on its first line
# with a message about '\r', which costs an hour to diagnose from inside a VM.
# .gitattributes pins *.sh to LF, so this only fires on a mangled checkout.
if ($source -like '*.sh') {
    $bytes = [System.IO.File]::ReadAllBytes($source)
    for ($i = 0; $i -lt $bytes.Length - 1; $i++) {
        if ($bytes[$i] -eq 13 -and $bytes[$i + 1] -eq 10) {
            Write-Error "'$source' has CRLF line endings; bash in the guest will fail on them. Re-checkout the file with LF endings (git config core.autocrlf false, then git checkout -- '$source') and re-run."
        }
    }
}

# --- The VM has to be running --------------------------------------------------
# Copy-VMFile goes over the VMBus to a service running inside Windows, so an off
# VM is not something the host can work around.
if ($vm.State -ne 'Running') {
    if (-not $Start) {
        Write-Error "VM '$Name' is $($vm.State); Copy-VMFile needs it running. Start it and re-run, or pass -Start: Start-VM -Name '$Name'"
    }
    Write-Host "Starting '$Name' ..." -ForegroundColor Cyan
    Start-VM -Name $Name
}

# --- Guest Service Interface ---------------------------------------------------
$svc = Get-VMIntegrationService -VMName $Name -Name 'Guest Service Interface' -ErrorAction SilentlyContinue
if (-not $svc) {
    Write-Error "VM '$Name' has no Guest Service Interface integration service. It is created with the VM; rebuild with pwsh -File scripts/vm-machine-create.ps1."
}
if (-not $svc.Enabled) {
    Write-Host "Enabling the Guest Service Interface on '$Name' ..." -ForegroundColor Cyan
    Enable-VMIntegrationService -VMName $Name -Name 'Guest Service Interface'
}

# Enabled on the host is not the same as answering in the guest: the guest-side
# service only comes up once Windows has finished setup and booted to a desktop.
$deadline = (Get-Date).AddSeconds($WaitTimeoutSeconds)
$svc = Get-VMIntegrationService -VMName $Name -Name 'Guest Service Interface'
while ($svc.PrimaryStatusDescription -ne 'OK' -and (Get-Date) -lt $deadline) {
    Write-Host ("  waiting for guest services ({0}) ..." -f $svc.PrimaryStatusDescription) -ForegroundColor DarkGray
    Start-Sleep -Seconds 5
    $svc = Get-VMIntegrationService -VMName $Name -Name 'Guest Service Interface'
}
if ($svc.PrimaryStatusDescription -ne 'OK') {
    Write-Error "The Guest Service Interface on '$Name' still reports '$($svc.PrimaryStatusDescription)' after $WaitTimeoutSeconds seconds. Open the console (vmconnect.exe localhost '$Name') and check that Windows has finished setup and is sitting at the desktop; the guest-side 'Hyper-V Guest Service Interface' service has to be running. Then re-run, or raise -WaitTimeoutSeconds."
}

# --- Copy ----------------------------------------------------------------------
Write-Host ''
Write-Host ("Copying into '{0}':" -f $Name) -ForegroundColor Cyan
Write-Host ("  host  : {0}" -f $source)
Write-Host ("  guest : {0}" -f $DestinationPath)

$copyArgs = @{
    Name            = $Name
    SourcePath      = $source
    DestinationPath = $DestinationPath
    CreateFullPath  = $true
    FileSource      = 'Host'
}
if ($Force) { $copyArgs['Force'] = $true }

try {
    Copy-VMFile @copyArgs
} catch {
    $msg = $_.Exception.Message
    if ($msg -match 'already exists') {
        Write-Error "'$DestinationPath' already exists in the guest. Re-run with -Force to overwrite it with the host copy."
    }
    Write-Error "Copy-VMFile failed: $msg"
}

Write-Host ''
Write-Host 'Copied.' -ForegroundColor Green

# --- Next steps ----------------------------------------------------------------
# Git Bash sees the guest's C:\ as /c/, so hand over a path that can be pasted
# into the shell the script actually runs under.
$bashPath = $DestinationPath -replace '^([A-Za-z]):', '/$1' -replace '\\', '/'
$bashPath = $bashPath.Substring(0, 2).ToLower() + $bashPath.Substring(2)

Write-Host ''
Write-Host 'Next' -ForegroundColor Cyan
Write-Host @"
  1. Open the guest console:

       vmconnect.exe localhost '$Name'

  2. Inside the guest, either follow README.md by hand - the honest test, because
     it exercises the prose too - or, once Git Bash is installed, run the
     mechanical version:

       bash $bashPath

  3. After a fix on the host, re-run this script with -Force to push the new
     copy in:

       pwsh -File $PSCommandPath -Name '$Name' -Force

  4. Reset the guest to the clean baseline before the next attempt:

       pwsh -File scripts/vm-checkpoint-restore.ps1 -Name '$Name'
"@

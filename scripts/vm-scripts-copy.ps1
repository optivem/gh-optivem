<#
.SYNOPSIS
  Copy the guest-side files (readme-setup.ps1, readme-enable.txt and
  readme-steps.sh) from the host into the running test VM.

.DESCRIPTION
  Step 4 of the install-test loop:

    scripts/vm-iso-download.ps1        # obtain a Windows 11 ISO
    scripts/vm-host-status.ps1         # read-only report - where does the host stand
    scripts/vm-hyperv-enable.ps1       # one-time host setup
    scripts/vm-machine-create.ps1      # build a clean VM from a Windows ISO
    scripts/vm-checkpoint-create.ps1   # freeze the clean install as 'clean-baseline'
    scripts/vm-scripts-copy.ps1        # this script
    scripts/readme-setup.ps1           # run INSIDE the VM: install Git Bash
    scripts/readme-steps.sh            # run INSIDE the VM: every README command
    scripts/vm-logs-copy.ps1           # pull the guest run logs back to the host
    scripts/vm-checkpoint-restore.ps1  # revert to the baseline for the next run
    scripts/vm-machine-delete.ps1      # tear the VM down
    scripts/vm-hyperv-disable.ps1      # turn Hyper-V back off

  The guest has no network share, no clipboard you can trust for a 300-line
  script, and (by design) no clone of this repo. Copy-VMFile pushes files over
  the VMBus instead, which is why vm-machine-create.ps1 turns the Guest Service
  Interface on. This script is that one command per file, plus the checks that
  turn its three unhelpful failure modes - VM off, integration service not
  answering, destination already there - into messages that say what to do.

  Both scripts go, because neither is enough alone: readme-steps.sh is a bash
  script and a clean Windows guest has no bash, so readme-setup.ps1 installs Git
  for Windows first from the shell Windows does ship with.

  readme-enable.txt rides along for the step before even that one: a clean guest
  refuses to run unsigned .ps1 files at all, and the way past it is a command
  you cannot discover from inside the error. Reading it back out with type needs
  no execution policy, which is exactly why the answer is in a .txt.

  Re-run it after every host-side edit: the copy always overwrites, because a
  stale copy in the guest is the one thing this script exists to prevent. The
  copy is a snapshot, not a mount, so the guest keeps whatever bytes it last
  received until the next run.

  What lands in the guest is only half the test. readme-steps.sh is the
  mechanical version of README.md; following the README by hand in the guest is
  the honest one, because it also tests the prose.

.PARAMETER Name
  VM to copy into. Defaults to gh-optivem-install-test.

.PARAMETER SourcePath
  Files on the host to copy. Defaults to readme-setup.ps1, readme-enable.txt
  and readme-steps.sh next to this script.

.PARAMETER DestinationDir
  Directory inside the guest to copy them into, keeping their names. Defaults to
  C:\Users\Public, which every guest account can read.

.PARAMETER Start
  Start the VM first if it is off, and wait for its guest services to answer.

.PARAMETER WaitTimeoutSeconds
  How long to wait for the Guest Service Interface to report OK before giving up.

.EXAMPLE
  pwsh -File scripts/vm-scripts-copy.ps1

.EXAMPLE
  pwsh -File scripts/vm-scripts-copy.ps1 -Start

.EXAMPLE
  pwsh -File scripts/vm-scripts-copy.ps1 -SourcePath .\README.md
#>
[CmdletBinding()]
param(
    [string]  $Name               = 'gh-optivem-install-test',
    [string[]]$SourcePath         = @(),
    [string]  $DestinationDir     = 'C:\Users\Public',
    [switch]  $Start,
    [int]     $WaitTimeoutSeconds = 180
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

if (-not $SourcePath -or $SourcePath.Count -eq 0) {
    # readme-setup.ps1 first: it is the one the operator runs, and the one that
    # makes the other runnable.
    $SourcePath = @(
        (Join-Path $PSScriptRoot 'readme-setup.ps1'),
        (Join-Path $PSScriptRoot 'readme-enable.txt'),
        (Join-Path $PSScriptRoot 'readme-steps.sh')
    )
}

$sources = @()
foreach ($p in $SourcePath) {
    if (-not (Test-Path -LiteralPath $p -PathType Leaf)) {
        Write-Error "No file at '$p'. Pass -SourcePath with the files you want in the guest."
    }
    $sources += (Resolve-Path -LiteralPath $p).Path
}

if (-not [System.IO.Path]::IsPathRooted($DestinationDir)) {
    Write-Error "-DestinationDir must be an absolute path inside the guest (for example C:\Users\Public); got '$DestinationDir'."
}

$vm = Get-VM -Name $Name -ErrorAction SilentlyContinue
if (-not $vm) {
    Write-Error "No VM named '$Name'. Build one first: pwsh -File scripts/vm-machine-create.ps1 -IsoPath <path-to-win11.iso>"
}

# A shell script that reaches the guest with CRLF endings dies on its first line
# with a message about '\r', which costs an hour to diagnose from inside a VM.
# .gitattributes pins *.sh to LF, so this only fires on a mangled checkout.
foreach ($s in $sources) {
    if ($s -notlike '*.sh') { continue }
    $bytes = [System.IO.File]::ReadAllBytes($s)
    for ($i = 0; $i -lt $bytes.Length - 1; $i++) {
        if ($bytes[$i] -eq 13 -and $bytes[$i + 1] -eq 10) {
            Write-Error "'$s' has CRLF line endings; bash in the guest will fail on them. Re-checkout the file with LF endings (git config core.autocrlf false, then git checkout -- '$s') and re-run."
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
Write-Host ("Copying into '{0}' at {1}:" -f $Name, $DestinationDir) -ForegroundColor Cyan

$setupDest = ''
foreach ($s in $sources) {
    $leaf = Split-Path -Leaf $s
    $dest = Join-Path $DestinationDir $leaf
    if ($leaf -eq 'readme-setup.ps1') { $setupDest = $dest }

    Write-Host ("  {0}" -f $leaf) -ForegroundColor DarkGray

    # -Force unconditionally: re-running after a host-side edit is the normal
    # case, and a copy that refuses to replace the stale file would defeat the
    # point. There is nothing in the guest worth preserving - it is a clean room.
    try {
        Copy-VMFile -Name $Name -SourcePath $s -DestinationPath $dest `
            -CreateFullPath -FileSource Host -Force
    } catch {
        $msg = $_.Exception.Message
        # Hyper-V reports the occupied destination as the raw Win32 error - 'The
        # file exists. (0x80070050)' - not as anything containing 'already
        # exists'. Match the code, which is the part that cannot be reworded.
        if ($msg -match '0x80070050' -or $msg -match 'file exists') {
            # Copy-VMFile takes -Force, but the guest-side copy service is what
            # refuses here and it can refuse regardless. Deleting in the guest is
            # then the only way through: the host has no way to remove a guest
            # file over the VMBus.
            Write-Error "'$dest' already exists in the guest and -Force did not displace it - Hyper-V's file-copy service declined to overwrite. Delete it from inside the guest (del $dest) and re-run, or pass -DestinationDir with a directory that is free."
        }
        Write-Error "Copy-VMFile failed for '$leaf': $msg"
    }
}

Write-Host ''
Write-Host ("Copied {0} file(s)." -f $sources.Count) -ForegroundColor Green

# --- Next steps ----------------------------------------------------------------
Write-Host ''
Write-Host 'Next' -ForegroundColor Cyan

if (-not $setupDest) {
    # A custom -SourcePath run; there is no bootstrap to point at.
    Write-Host @"
  Open the guest console and take it from there:

       vmconnect.exe localhost '$Name'
"@
    return
}

Write-Host @"
  1. Open the guest console:

       vmconnect.exe localhost '$Name'

  2. Inside the guest, in PowerShell - not bash, which does not exist there yet:

       powershell -ExecutionPolicy Bypass -File $setupDest

     That installs Git for Windows and prints the command that runs the
     walkthrough. Add -Run to chain straight into it.

     -ExecutionPolicy Bypass is not optional: a clean guest refuses unsigned
     .ps1 files. If you would rather unblock the window once and call the
     script by name, the alternative is in $DestinationDir\readme-enable.txt.

     Both guest-side scripts log to $DestinationDir\logs.

  3. After a fix on the host, push the new copies in - re-running overwrites:

       pwsh -File $PSCommandPath -Name '$Name'

  4. Pull the logs out before resetting - the revert discards them:

       pwsh -File scripts/vm-logs-copy.ps1 -Name '$Name'

  5. Reset the guest to the clean baseline before the next attempt:

       pwsh -File scripts/vm-checkpoint-restore.ps1 -Name '$Name'
"@

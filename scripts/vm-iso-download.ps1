<#
.SYNOPSIS
  Get a Windows 11 ISO for scripts/vm-machine-create.ps1 - automatically if Microsoft
  allows it, by guiding the manual download if not.

.DESCRIPTION
  Step 0 of the install-test loop:

    scripts/vm-iso-download.ps1        # this script - obtain a Windows 11 ISO
    scripts/vm-host-status.ps1         # read-only report - where does the host stand
    scripts/vm-hyperv-enable.ps1       # one-time host setup
    scripts/vm-machine-create.ps1      # build a clean VM from a Windows ISO
    scripts/vm-checkpoint-create.ps1   # freeze the clean install as 'clean-baseline'
    scripts/vm-scripts-copy.ps1        # copy the walkthrough into the guest
    scripts/readme-setup.ps1           # run INSIDE the VM: install Git Bash
    scripts/readme-steps.sh            # run INSIDE the VM: every README command
    scripts/vm-checkpoint-restore.ps1  # revert to the baseline for the next run
    scripts/vm-machine-delete.ps1      # tear the VM down
    scripts/vm-hyperv-disable.ps1      # turn Hyper-V back off

  A caveat you should know before trusting this: Microsoft's ISO download is
  session-gated and actively hostile to automation. Three calls are involved -
  discover the product, list the languages, then issue a download link - and the
  third is screened by a bot filter that answers

      {"Errors":[{"Key":"ErrorSettings.SentinelReject", ...}]}

  for requests it dislikes. VPNs, datacenter ranges and plain bad luck all
  trigger it; Rufus's Fido tool hits exactly the same wall. There is no way
  around it that is worth having in this repo.

  So the script tries, and when the gate says no it does not pretend otherwise:
  it opens the download page, watches your Downloads folder for the ISO to
  finish arriving, verifies what showed up, and prints the vm-machine-create.ps1 command
  with the real path filled in. Either way you end up in the same place.

.PARAMETER OutDir
  Where the ISO should end up. Defaults to your Downloads folder.

.PARAMETER Language
  ISO language as Microsoft names it, e.g. 'English', 'English International',
  'German'. Defaults to English.

.PARAMETER Open
  Skip the automated attempt and go straight to the browser.

.PARAMETER Verify
  Verify an ISO you already have and print the vm-machine-create.ps1 command for it.
  Does nothing else.

.PARAMETER NoWait
  Do not watch for the download to finish; exit once the browser is open.

.PARAMETER TimeoutMinutes
  How long to watch for the ISO to appear. Default 60.

.EXAMPLE
  pwsh -File scripts/vm-iso-download.ps1

.EXAMPLE
  pwsh -File scripts/vm-iso-download.ps1 -Verify "$HOME\Downloads\Win11.iso"
#>
[CmdletBinding()]
param(
    [string]$OutDir,
    [string]$Language = 'English',
    [switch]$Open,
    [string]$Verify,
    [switch]$NoWait,
    [int]   $TimeoutMinutes = 60
)

$ErrorActionPreference = 'Stop'

$DownloadPage  = 'https://www.microsoft.com/en-us/software-download/windows11'
$ConnectorBase = 'https://www.microsoft.com/software-download-connector/api'
$SentinelUrl   = 'https://vlscppe.microsoft.com/fp/tags?org_id=y6jn8c31&session_id={0}'
# Constant in Microsoft's own page JavaScript, not a credential.
$Profile       = '606624d44113'
$UserAgent     = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0'

if (-not $OutDir) {
    # Downloads is not guaranteed to be under $HOME - it can be redirected to
    # OneDrive or another drive - so ask the shell where it actually is.
    try {
        $OutDir = (New-Object -ComObject Shell.Application).NameSpace('shell:Downloads').Self.Path
    } catch {
        $OutDir = Join-Path $HOME 'Downloads'
    }
}

# --- ISO verification --------------------------------------------------------
# Reads the ISO 9660 Primary Volume Descriptor directly: 2048-byte sector at
# offset 0x8000, magic 'CD001' at byte 1, volume identifier at bytes 40..71.
# Mounting would tell us more but needs elevation; this needs nothing.
function Test-WindowsIso([string]$path) {
    $result = [ordered]@{ Path = $path; Valid = $false; SizeGB = 0; VolumeId = ''; Reason = '' }

    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        $result.Reason = 'file not found'
        return [pscustomobject]$result
    }

    $file = Get-Item -LiteralPath $path
    $result.SizeGB = [math]::Round($file.Length / 1GB, 2)

    if ($file.Length -lt 3GB) {
        $result.Reason = "only $($result.SizeGB) GB - a Windows 11 ISO is roughly 5-7 GB, so this looks truncated or is not an installation image"
        return [pscustomobject]$result
    }

    try {
        $stream = [IO.File]::OpenRead($path)
        try {
            $buffer = New-Object byte[] 2048
            [void]$stream.Seek(0x8000, [IO.SeekOrigin]::Begin)
            [void]$stream.Read($buffer, 0, 2048)
        } finally {
            $stream.Dispose()
        }
    } catch {
        $result.Reason = "could not read the volume descriptor: $($_.Exception.Message)"
        return [pscustomobject]$result
    }

    $magic = [Text.Encoding]::ASCII.GetString($buffer, 1, 5)
    if ($magic -ne 'CD001') {
        $result.Reason = 'not an ISO 9660 image (volume descriptor magic missing)'
        return [pscustomobject]$result
    }

    $result.VolumeId = ([Text.Encoding]::ASCII.GetString($buffer, 40, 32)).Trim()

    # Retail Windows media labels look like CCCOMA_X64FRE_EN-US_DV9; evaluation
    # and MSDN images vary, so treat an unfamiliar label as a warning only.
    if ($result.VolumeId -notmatch 'X64|X86|ARM64|WIN|CCCOMA|CES_') {
        $result.Reason = "volume label '$($result.VolumeId)' does not look like Windows installation media - check you downloaded the ISO and not something else"
        return [pscustomobject]$result
    }

    $result.Valid = $true
    return [pscustomobject]$result
}

function Show-IsoResult($iso) {
    if ($iso.Valid) {
        Write-Host ''
        Write-Host 'ISO looks good' -ForegroundColor Green
        Write-Host ("  Path      : {0}" -f $iso.Path)
        Write-Host ("  Size      : {0} GB" -f $iso.SizeGB)
        Write-Host ("  Volume ID : {0}" -f $iso.VolumeId)
        Write-Host ''
        Write-Host 'Create the test VM with (elevated PowerShell):' -ForegroundColor Cyan
        Write-Host ("  pwsh -File scripts/vm-machine-create.ps1 -IsoPath `"{0}`"" -f $iso.Path)
        return $true
    }

    Write-Host ''
    Write-Host 'ISO did not verify' -ForegroundColor Red
    Write-Host ("  Path   : {0}" -f $iso.Path)
    Write-Host ("  Reason : {0}" -f $iso.Reason)
    return $false
}

# --- -Verify: check an existing file and stop --------------------------------
if ($Verify) {
    $resolved = (Resolve-Path -LiteralPath $Verify -ErrorAction SilentlyContinue).Path
    if (-not $resolved) { $resolved = $Verify }
    $iso = Test-WindowsIso $resolved
    exit $(if (Show-IsoResult $iso) { 0 } else { 1 })
}

if (-not (Test-Path -LiteralPath $OutDir)) {
    New-Item -ItemType Directory -Path $OutDir -Force | Out-Null
}

# --- Already have one? -------------------------------------------------------
$existing = @(Get-ChildItem -LiteralPath $OutDir -Filter '*.iso' -File -ErrorAction SilentlyContinue |
    Where-Object { $_.Length -gt 3GB } | Sort-Object LastWriteTime -Descending)

if ($existing.Count -gt 0) {
    Write-Host ''
    Write-Host ("Found {0} ISO(s) already in {1}:" -f $existing.Count, $OutDir) -ForegroundColor Cyan
    foreach ($f in $existing) {
        Write-Host ("  {0}  ({1:N1} GB, {2:yyyy-MM-dd})" -f $f.Name, ($f.Length / 1GB), $f.LastWriteTime)
    }
    $iso = Test-WindowsIso $existing[0].FullName
    if ($iso.Valid) {
        Write-Host ''
        Write-Host 'Using the newest one - no download needed.' -ForegroundColor Green
        [void](Show-IsoResult $iso)
        Write-Host ''
        Write-Host 'Re-run with -Open if you want a different or fresher build.' -ForegroundColor DarkGray
        exit 0
    }
}

# --- Automated attempt -------------------------------------------------------
$autoLink = $null

if (-not $Open) {
    Write-Host ''
    Write-Host 'Asking Microsoft for a download link ...' -ForegroundColor Cyan

    try {
        $sessionId = [guid]::NewGuid().Guid
        $session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
        $session.UserAgent = $UserAgent
        $headers = @{
            'Accept'          = 'application/json, text/plain, */*'
            'Accept-Language' = 'en-US,en;q=0.9'
            'Referer'         = $DownloadPage
        }

        # 1. The product page carries the current release's edition id, so this
        #    keeps working across Windows releases without a pinned constant.
        $page = Invoke-WebRequest -Uri $DownloadPage -WebSession $session -UseBasicParsing -TimeoutSec 30
        $editionId = ([regex]::Matches($page.Content, 'option value="(\d{3,5})"') |
            ForEach-Object { $_.Groups[1].Value } | Select-Object -First 1)
        if (-not $editionId) { throw 'could not find a product edition id on the download page' }
        Write-Host ("  Product edition : {0}" -f $editionId) -ForegroundColor DarkGray

        # 2. Register the session with the bot filter before using it.
        Invoke-WebRequest -Uri ($SentinelUrl -f $sessionId) -WebSession $session -UseBasicParsing -TimeoutSec 30 | Out-Null
        Start-Sleep -Seconds 3

        # 3. Languages / SKUs.
        $skuUrl = "$ConnectorBase/getskuinformationbyproductedition?profile=$Profile&ProductEditionId=$editionId&SKU=undefined&friendlyFileName=undefined&Locale=en-US&sessionID=$sessionId"
        $skuJson = (Invoke-WebRequest -Uri $skuUrl -WebSession $session -Headers $headers -UseBasicParsing -TimeoutSec 30).Content | ConvertFrom-Json
        if ($skuJson.Errors) { throw ($skuJson.Errors | ForEach-Object { $_.Value }) -join '; ' }

        $sku = $skuJson.Skus | Where-Object { $_.Language -eq $Language } | Select-Object -First 1
        if (-not $sku) {
            $available = ($skuJson.Skus | Select-Object -ExpandProperty Language | Sort-Object) -join ', '
            throw "language '$Language' not offered. Available: $available"
        }
        Write-Host ("  Edition         : {0} ({1})" -f $sku.ProductDisplayName, $sku.Language) -ForegroundColor DarkGray

        # 4. The gated call. This is the one that usually says no.
        $linkUrl = "$ConnectorBase/GetProductDownloadLinksBySku?profile=$Profile&productEditionId=undefined&SKU=$($sku.Id)&friendlyFileName=undefined&Locale=en-US&sessionID=$sessionId"
        $linkJson = (Invoke-WebRequest -Uri $linkUrl -WebSession $session -Headers $headers -UseBasicParsing -TimeoutSec 30).Content | ConvertFrom-Json

        if ($linkJson.Errors) {
            $keys = ($linkJson.Errors | ForEach-Object { $_.Key }) -join ', '
            if ($keys -match 'Sentinel') {
                throw 'Microsoft''s bot filter rejected the request (SentinelReject). This is expected on VPNs, corporate networks and datacenter IPs, and is not something the script can work around.'
            }
            throw (($linkJson.Errors | ForEach-Object { "$($_.Key): $($_.Value)" }) -join '; ')
        }

        $autoLink = ($linkJson.ProductDownloadOptions | Where-Object { $_.Uri } | Select-Object -First 1).Uri
        if (-not $autoLink) { throw 'response contained no download URI' }
    } catch {
        Write-Host ''
        Write-Host ("Automated download unavailable: {0}" -f $_.Exception.Message) -ForegroundColor Yellow
        Write-Host 'Falling back to the browser - same ISO, one extra click.' -ForegroundColor Yellow
    }
}

# --- Download ----------------------------------------------------------------
$startTime = Get-Date

if ($autoLink) {
    $fileName = ([uri]$autoLink).Segments[-1]
    if ($fileName -notmatch '\.iso$') { $fileName = "Windows11_$Language.iso" }
    $target = Join-Path $OutDir $fileName

    Write-Host ''
    Write-Host ("Downloading to {0}" -f $target) -ForegroundColor Cyan
    Write-Host '  ~6 GB - this takes a while.' -ForegroundColor DarkGray

    # BITS where available: it shows real progress and survives interruptions.
    # Invoke-WebRequest buffers aggressively and reports nothing useful.
    try {
        Start-BitsTransfer -Source $autoLink -Destination $target -Description 'Windows 11 ISO' -ErrorAction Stop
    } catch {
        Write-Host ("  BITS unavailable ({0}) - falling back to a direct download." -f $_.Exception.Message) -ForegroundColor DarkGray
        Invoke-WebRequest -Uri $autoLink -OutFile $target -UseBasicParsing
    }

    exit $(if (Show-IsoResult (Test-WindowsIso $target)) { 0 } else { 1 })
}

# --- Browser fallback --------------------------------------------------------
Write-Host ''
Write-Host 'Opening the Microsoft download page.' -ForegroundColor Cyan
Write-Host @"

On that page:

  1. Scroll to "Download Windows 11 Disk Image (ISO) for x64 devices"
  2. Choose "Windows 11 (multi-edition ISO for x64 devices)" -> Download now
  3. Pick your language -> Confirm
  4. Click "64-bit Download"

Save it to:
  $OutDir

You do not need a product key. During Windows setup choose "I don't have a
product key" - an unactivated Windows runs fine for install testing.
"@

Start-Process $DownloadPage

if ($NoWait) {
    Write-Host ''
    Write-Host 'Once it has downloaded, verify it with:' -ForegroundColor Cyan
    Write-Host ("  pwsh -File scripts/vm-iso-download.ps1 -Verify `"{0}\<filename>.iso`"" -f $OutDir)
    exit 0
}

# --- Watch for the download to land ------------------------------------------
Write-Host ''
Write-Host ("Watching {0} for a new ISO (Ctrl+C to stop) ..." -f $OutDir) -ForegroundColor Cyan

$deadline  = $startTime.AddMinutes($TimeoutMinutes)
$lastSize  = -1
$stableFor = 0

while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds 15

    # Browsers write .iso.crdownload / .iso.part first, so a plain *.iso newer
    # than this script's start is the signal that a download has begun.
    $candidate = Get-ChildItem -LiteralPath $OutDir -Filter '*.iso' -File -ErrorAction SilentlyContinue |
        Where-Object { $_.LastWriteTime -ge $startTime } |
        Sort-Object LastWriteTime -Descending | Select-Object -First 1

    if (-not $candidate) {
        $partial = @(Get-ChildItem -LiteralPath $OutDir -File -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -match '\.(crdownload|part|tmp|download)$' -and $_.LastWriteTime -ge $startTime })
        if ($partial) {
            $mb = [math]::Round(($partial | Measure-Object -Property Length -Sum).Sum / 1MB)
            Write-Host ("  downloading ... {0:N0} MB" -f $mb) -ForegroundColor DarkGray
        }
        continue
    }

    # Size stable across two polls means the write has finished.
    if ($candidate.Length -eq $lastSize) {
        $stableFor++
    } else {
        $stableFor = 0
        Write-Host ("  {0} ... {1:N1} GB" -f $candidate.Name, ($candidate.Length / 1GB)) -ForegroundColor DarkGray
    }
    $lastSize = $candidate.Length

    if ($stableFor -ge 2 -and $candidate.Length -gt 3GB) {
        exit $(if (Show-IsoResult (Test-WindowsIso $candidate.FullName)) { 0 } else { 1 })
    }
}

Write-Host ''
Write-Host ("Gave up watching after {0} minutes." -f $TimeoutMinutes) -ForegroundColor Yellow
Write-Host 'When the download finishes, verify it with:' -ForegroundColor Cyan
Write-Host ("  pwsh -File scripts/vm-iso-download.ps1 -Verify `"{0}\<filename>.iso`"" -f $OutDir)
exit 1

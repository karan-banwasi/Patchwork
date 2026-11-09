# Install script for Patchwork (pw)
[CmdletBinding()]
param(
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "Programs\Patchwork\bin"),
    [string]$Repo = "karan-banwasi/Patchwork",
    [switch]$FunctionsOnly,
    [switch]$SkipAttestation,
    [switch]$RequireAttestation
)

# Force TLS 1.2 / TLS 1.3 for GitHub compatibility on Windows PowerShell
try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]'Tls12, Tls13'
} catch {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
}

function Get-ExpectedChecksum {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)]
        [string]$Content,
        [Parameter(Mandatory = $true)]
        [string]$AssetName,
        [Parameter(Mandatory = $true)]
        [string]$WingetArch
    )

    # 1. Primary: SHA256SUMS format (<64-hex>  <filename>)
    $shaSumsPattern = '(?m)^([A-Fa-f0-9]{64})\s+\*?(?:\.[\/\\])?' + [regex]::Escape($AssetName) + '\s*$'
    $matchSha = [regex]::Match($Content, $shaSumsPattern)
    if ($matchSha.Success) {
        return $matchSha.Groups[1].Value.ToUpper()
    }

    # 2. Fallback: Winget installer YAML format (Architecture: <arch> ... InstallerSha256: <hash>)
    $yamlPattern = "(?ms)Architecture:\s*" + [regex]::Escape($WingetArch) + "\b.*?InstallerSha256:\s*([A-Fa-f0-9]{64})"
    $matchYaml = [regex]::Match($Content, $yamlPattern)
    if ($matchYaml.Success) {
        return $matchYaml.Groups[1].Value.ToUpper()
    }

    return $null
}

function Test-AttestationProvenance {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)]
        [string]$BinaryPath,
        [Parameter(Mandatory = $true)]
        [string]$Repo,
        [string]$SignerWorkflow,
        [switch]$SkipAttestation,
        [switch]$RequireAttestation,
        [bool]$HasGh = [bool](Get-Command gh -ErrorAction SilentlyContinue),
        [scriptblock]$GhRunner = {
            param($RunnerArgs)
            $prevEap = $ErrorActionPreference
            $ErrorActionPreference = 'Continue'
            try {
                $out = & gh @RunnerArgs 2>&1
                $code = if ($null -ne $global:LASTEXITCODE) { $global:LASTEXITCODE } elseif ($null -ne $LASTEXITCODE) { $LASTEXITCODE } else { 0 }
                return [PSCustomObject]@{
                    Output = ($out | Out-String).Trim()
                    ExitCode = $code
                }
            } finally {
                $ErrorActionPreference = $prevEap
            }
        }
    )

    if ($SkipAttestation -and $RequireAttestation) {
        throw "Cannot specify both -SkipAttestation and -RequireAttestation."
    }

    if ($SkipAttestation) {
        Write-Warning "Skipping build provenance attestation verification (-SkipAttestation)."
        return $true
    }

    if (-not $HasGh) {
        if ($RequireAttestation) {
            throw "Attestation verification required (-RequireAttestation), but GitHub CLI ('gh') is not installed or available on PATH."
        }
        Write-Verbose "GitHub CLI ('gh') not found; skipping attestation check."
        return $true
    }

    $verifyArgs = @("attestation", "verify", $BinaryPath, "--repo", $Repo)
    if ($SignerWorkflow) {
        $verifyArgs += @("--signer-workflow", $SignerWorkflow)
    }

    Write-Host "Verifying GitHub build provenance attestation..." -ForegroundColor Cyan
    $result = & $GhRunner $verifyArgs

    $runnerExitCode = 0
    $detail = ""

    if ($result -is [PSCustomObject] -and ($null -ne $result.PSObject.Properties['ExitCode'])) {
        $runnerExitCode = [int]$result.ExitCode
        $detail = if ($null -ne $result.PSObject.Properties['Output']) { [string]$result.Output } else { "" }
    } elseif ($result -is [int]) {
        $runnerExitCode = [int]$result
        $detail = ""
    } elseif ($result -is [System.Collections.IEnumerable] -and -not ($result -is [string]) -and ($result.Count -gt 0) -and ($result[-1] -is [int])) {
        $runnerExitCode = [int]$result[-1]
        $detail = ($result[0..($result.Count - 2)] | Out-String).Trim()
    } else {
        $runnerExitCode = if ($null -ne $global:LASTEXITCODE) { $global:LASTEXITCODE } else { $LASTEXITCODE }
        $detail = ($result | Out-String).Trim()
    }

    if ($runnerExitCode -ne 0) {
        throw "SECURITY WARNING: GitHub build provenance attestation verification failed.`nError: $detail`nRefusing to install unverified binary. (Pass -SkipAttestation to bypass if you trust this binary.)"
    }

    Write-Host "Verified GitHub build provenance attestation." -ForegroundColor Green
    return $true
}

# If dot-sourced or called with -FunctionsOnly, return early after defining functions
if ($FunctionsOnly -or $MyInvocation.InvocationName -eq '.') {
    return
}

$ErrorActionPreference = "Stop"

# Early parameter validation and preflight checks before any network or file operations
if ($SkipAttestation -and $RequireAttestation) {
    Write-Error "Cannot specify both -SkipAttestation and -RequireAttestation."
    exit 1
}

$hasGh = [bool](Get-Command gh -ErrorAction SilentlyContinue)
if ($RequireAttestation -and -not $hasGh) {
    Write-Error "Attestation verification required (-RequireAttestation), but GitHub CLI ('gh') is not installed or available on PATH."
    exit 1
}

$repo = $Repo
$installDir = $InstallDir
$exePath = Join-Path $installDir "pw.exe"
$shaSumsName = "SHA256SUMS"
$legacyManifestName = "KaranBanwasi.Patchwork.installer.yaml"

# Detect System Architecture (amd64 vs arm64)
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_ARCHITEW6432 -eq "ARM64") { "arm64" } else { "amd64" }
$assetName = if ($arch -eq "arm64") { "pw-arm64.exe" } else { "pw.exe" }
$wingetArch = if ($arch -eq "arm64") { "arm64" } else { "x64" }

# Warn (do not delete) if a developer build exists in ~\go\bin\pw.exe
$goBinExe = Join-Path $env:USERPROFILE "go\bin\pw.exe"
if (Test-Path -Path $goBinExe) {
    Write-Warning "$goBinExe may shadow the installed pw.exe on PATH; remove it if it is an old dev build."
}

# Ensure target directory exists without removing any existing installation prior to verification
if (-not (Test-Path -Path $installDir)) {
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
}

$tmpId = [guid]::NewGuid().ToString("N")
$tmpExe = Join-Path $env:TEMP "pw-$tmpId.exe"
$tmpManifest = Join-Path $env:TEMP "pw-$tmpId.manifest"

Write-Host "Installing Patchwork (pw) for $arch..." -ForegroundColor Cyan

$downloaded = $false

# 1. Try downloading latest stable release via GitHub CLI
if ($hasGh) {
    try {
        Write-Host "Attempting download of latest release via GitHub CLI..." -ForegroundColor Cyan
        # Try downloading binary + SHA256SUMS first
        & gh release download -R $repo -p $assetName -p $shaSumsName -D $env:TEMP --clobber 2>$null
        $ghAssetPath = Join-Path $env:TEMP $assetName
        $ghShaPath = Join-Path $env:TEMP $shaSumsName
        if ($LASTEXITCODE -eq 0 -and (Test-Path -Path $ghAssetPath) -and (Test-Path -Path $ghShaPath)) {
            Move-Item -Path $ghAssetPath -Destination $tmpExe -Force
            Move-Item -Path $ghShaPath -Destination $tmpManifest -Force
            $downloaded = $true
        } else {
            # Fallback: try downloading binary + legacy Winget manifest
            & gh release download -R $repo -p $assetName -p $legacyManifestName -D $env:TEMP --clobber 2>$null
            $ghLegacyPath = Join-Path $env:TEMP $legacyManifestName
            if ($LASTEXITCODE -eq 0 -and (Test-Path -Path $ghAssetPath) -and (Test-Path -Path $ghLegacyPath)) {
                Move-Item -Path $ghAssetPath -Destination $tmpExe -Force
                Move-Item -Path $ghLegacyPath -Destination $tmpManifest -Force
                $downloaded = $true
            }
        }
    } catch {
        Write-Verbose "GitHub CLI download did not succeed ($($_.Exception.Message)); falling back to REST API."
    } finally {
        # Clean up any leftover un-moved assets in TEMP
        Remove-Item -Path (Join-Path $env:TEMP $assetName), (Join-Path $env:TEMP $shaSumsName), (Join-Path $env:TEMP $legacyManifestName) -Force -ErrorAction SilentlyContinue
    }
}

# 2. Try REST download of latest stable release (/releases/latest excludes drafts and prereleases)
if (-not $downloaded) {
    $token = if ($env:GH_TOKEN) { $env:GH_TOKEN } elseif ($env:GITHUB_TOKEN) { $env:GITHUB_TOKEN } else { "" }
    $headers = @{ "User-Agent" = "PatchworkInstaller" }
    if ($token) {
        $headers["Authorization"] = "Bearer $token"
    }

    try {
        $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers $headers -UseBasicParsing
        if ($rel.draft -or $rel.prerelease) {
            throw "Release $($rel.tag_name) is a draft or prerelease."
        }
        $exeAsset = $rel.assets | Where-Object { $_.name -eq $assetName } | Select-Object -First 1
        if (-not $exeAsset) {
            throw "Release $($rel.tag_name) does not contain asset '$assetName'."
        }

        $shaSumsAsset = $rel.assets | Where-Object { $_.name -eq $shaSumsName } | Select-Object -First 1
        $legacyManAsset = $rel.assets | Where-Object { $_.name -eq $legacyManifestName } | Select-Object -First 1

        $manAsset = if ($shaSumsAsset) { $shaSumsAsset } else { $legacyManAsset }
        if (-not $manAsset) {
            throw "Release $($rel.tag_name) does not contain a checksum manifest ('$shaSumsName' or '$legacyManifestName')."
        }

        Write-Host "Downloading release $($rel.tag_name) from GitHub..." -ForegroundColor Cyan

        if ($token -and $exeAsset.url -and $manAsset.url) {
            $apiHeaders = @{
                "User-Agent"    = "PatchworkInstaller"
                "Authorization" = "Bearer $token"
                "Accept"        = "application/octet-stream"
            }
            Invoke-WebRequest -Uri $exeAsset.url -OutFile $tmpExe -Headers $apiHeaders -UseBasicParsing
            Invoke-WebRequest -Uri $manAsset.url -OutFile $tmpManifest -Headers $apiHeaders -UseBasicParsing
        } else {
            Invoke-WebRequest -Uri $exeAsset.browser_download_url -OutFile $tmpExe -Headers $headers -UseBasicParsing
            Invoke-WebRequest -Uri $manAsset.browser_download_url -OutFile $tmpManifest -Headers $headers -UseBasicParsing
        }
        $downloaded = $true
    } catch {
        if (Test-Path -Path $tmpExe) { Remove-Item -Path $tmpExe -Force -ErrorAction SilentlyContinue }
        if (Test-Path -Path $tmpManifest) { Remove-Item -Path $tmpManifest -Force -ErrorAction SilentlyContinue }
        $exMsg = $_.Exception.Message
        Write-Error "Failed to download Patchwork ($exMsg)."
        exit 1
    }
}

# Verify SHA-256 checksum against the release manifest before replacing any existing binary
try {
    $manifestContent = Get-Content -Path $tmpManifest -Raw
    $expectedHash = Get-ExpectedChecksum -Content $manifestContent -AssetName $assetName -WingetArch $wingetArch
    if (-not $expectedHash) {
        throw "Could not find expected SHA256 checksum for '$assetName' in manifest."
    }
    $actualHash = (Get-FileHash -Path $tmpExe -Algorithm SHA256).Hash.ToUpper()

    if ($actualHash -ne $expectedHash) {
        throw "SHA256 verification failed for $assetName (expected $expectedHash, got $actualHash)."
    }
    Write-Host "Verified SHA256 checksum ($($actualHash.Substring(0, 12))...)." -ForegroundColor Green

    $signerWorkflow = "$repo/.github/workflows/release.yml"
    Test-AttestationProvenance -BinaryPath $tmpExe -Repo $repo -SignerWorkflow $signerWorkflow `
        -SkipAttestation:$SkipAttestation -RequireAttestation:$RequireAttestation -HasGh:$hasGh

    Move-Item -Path $tmpExe -Destination $exePath -Force
    Remove-Item -Path $tmpManifest -Force -ErrorAction SilentlyContinue
    Write-Host "Successfully installed pw.exe to $installDir" -ForegroundColor Green
} catch {
    if (Test-Path -Path $tmpExe) { Remove-Item -Path $tmpExe -Force -ErrorAction SilentlyContinue }
    if (Test-Path -Path $tmpManifest) { Remove-Item -Path $tmpManifest -Force -ErrorAction SilentlyContinue }
    Write-Error $_.Exception.Message
    exit 1
}

# Update User PATH if needed
try {
    $userPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
} catch {
    $userPath = $null
}
$cleanUserPath = if ($userPath) { $userPath.TrimEnd(';') } else { "" }
if (($cleanUserPath -split ';') -notcontains $installDir) {
    Write-Host "Adding $installDir to User PATH..." -ForegroundColor Yellow
    $newUserPath = if ([string]::IsNullOrWhitespace($cleanUserPath)) { $installDir } else { "$cleanUserPath;$installDir" }
    
    $pathUpdated = $false
    try {
        [Environment]::SetEnvironmentVariable("Path", $newUserPath, [EnvironmentVariableTarget]::User)
        $pathUpdated = $true
    } catch {
        Write-Warning "Could not update User PATH in registry: $($_.Exception.Message). Please add $installDir to PATH manually."
    }

    $env:PATH = if ([string]::IsNullOrWhitespace($env:PATH)) { $installDir } else { "$($env:PATH.TrimEnd(';'));$installDir" }

    if ($pathUpdated) {
        # Broadcast WM_SETTINGCHANGE so running terminal windows update environment without restarting
        try {
            if (-not ('Win32.NativeMethods' -as [type])) {
                Add-Type -Namespace Win32 -Name NativeMethods -MemberDefinition @"
                    [DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)]
                    public static extern IntPtr SendMessageTimeout(
                        IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam,
                        uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);
"@
            }
            $HWND_BROADCAST = [IntPtr]0xffff
            $WM_SETTINGCHANGE = 0x001A
            $SMTO_ABORTIFHUNG = 0x0002
            $result = [UIntPtr]::Zero
            [Win32.NativeMethods]::SendMessageTimeout($HWND_BROADCAST, $WM_SETTINGCHANGE, [UIntPtr]::Zero, "Environment", $SMTO_ABORTIFHUNG, 5000, [ref]$result) | Out-Null
        } catch {
            # Ignore notification failures if P/Invoke is restricted
        }

        Write-Host "PATH updated successfully." -ForegroundColor Green
    }
}

Write-Host "`nPatchwork has been installed successfully!" -ForegroundColor Green

if (Test-Path -Path $exePath) {
    try {
        & $exePath version
    } catch {
        # Fallback if execution is restricted
    }
}

Write-Host "Run 'pw --help' or 'pw check' in a new terminal window to get started." -ForegroundColor Cyan

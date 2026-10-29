# Install script for Patchwork (pw)
$ErrorActionPreference = "Stop"

# Force TLS 1.2 / TLS 1.3 for GitHub compatibility on Windows PowerShell
try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]'Tls12, Tls13'
} catch {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
}

$repo = "karan-banwasi/Patchwork"
$installDir = Join-Path $env:LOCALAPPDATA "Programs\Patchwork\bin"
$exePath = Join-Path $installDir "pw.exe"
$manifestName = "KaranBanwasi.Patchwork.installer.yaml"

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
$tmpManifest = Join-Path $env:TEMP "pw-$tmpId.installer.yaml"

Write-Host "Installing Patchwork (pw) for $arch..." -ForegroundColor Cyan

$downloaded = $false
$hasGh = [bool](Get-Command gh -ErrorAction SilentlyContinue)

# 1. Try downloading latest stable release via GitHub CLI
if ($hasGh) {
    try {
        Write-Host "Attempting download of latest release via GitHub CLI..." -ForegroundColor Cyan
        & gh release download -R $repo -p $assetName -p $manifestName -D $env:TEMP --clobber
        $ghAssetPath = Join-Path $env:TEMP $assetName
        $ghManifestPath = Join-Path $env:TEMP $manifestName
        if ($LASTEXITCODE -eq 0 -and (Test-Path -Path $ghAssetPath) -and (Test-Path -Path $ghManifestPath)) {
            Move-Item -Path $ghAssetPath -Destination $tmpExe -Force
            Move-Item -Path $ghManifestPath -Destination $tmpManifest -Force
            $downloaded = $true
        }
    } catch {
        Write-Verbose "GitHub CLI download did not succeed ($($_.Exception.Message)); falling back to REST API."
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
        $manAsset = $rel.assets | Where-Object { $_.name -eq $manifestName } | Select-Object -First 1
        if (-not $exeAsset) {
            throw "Release $($rel.tag_name) does not contain asset '$assetName'."
        }
        if (-not $manAsset) {
            throw "Release $($rel.tag_name) does not contain checksum manifest '$manifestName'."
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

# Verify SHA-256 checksum against the release installer manifest before replacing any existing binary
try {
    $yamlContent = Get-Content -Path $tmpManifest -Raw
    $pattern = "(?ms)Architecture:\s*$wingetArch\b.*?InstallerSha256:\s*([A-Fa-f0-9]{64})"
    $match = [regex]::Match($yamlContent, $pattern)
    if (-not $match.Success) {
        throw "Could not find InstallerSha256 for architecture '$wingetArch' in $manifestName."
    }
    $expectedHash = $match.Groups[1].Value.ToUpper()
    $actualHash = (Get-FileHash -Path $tmpExe -Algorithm SHA256).Hash.ToUpper()

    if (-not $expectedHash -or $actualHash -ne $expectedHash) {
        throw "SHA256 verification failed for $assetName (expected $expectedHash, got $actualHash)."
    }
    Write-Host "Verified SHA256 checksum ($($actualHash.Substring(0, 12))...)." -ForegroundColor Green

    if ($hasGh) {
        & gh attestation verify $tmpExe --repo $repo 2>$null
        if ($LASTEXITCODE -eq 0) {
            Write-Host "Verified GitHub build provenance attestation." -ForegroundColor Green
        }
    }

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
$userPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
$cleanUserPath = if ($userPath) { $userPath.TrimEnd(';') } else { "" }
if (($cleanUserPath -split ';') -notcontains $installDir) {
    Write-Host "Adding $installDir to User PATH..." -ForegroundColor Yellow
    $newUserPath = if ([string]::IsNullOrWhitespace($cleanUserPath)) { $installDir } else { "$cleanUserPath;$installDir" }
    [Environment]::SetEnvironmentVariable("Path", $newUserPath, [EnvironmentVariableTarget]::User)
    $env:PATH = if ([string]::IsNullOrWhitespace($env:PATH)) { $installDir } else { "$($env:PATH.TrimEnd(';'));$installDir" }

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

Write-Host "`nPatchwork has been installed successfully!" -ForegroundColor Green

if (Test-Path -Path $exePath) {
    try {
        & $exePath version
    } catch {
        # Fallback if execution is restricted
    }
}

Write-Host "Run 'pw --help' or 'pw check' in a new terminal window to get started." -ForegroundColor Cyan

# Install script for Patchwork (pw)
$ErrorActionPreference = "Stop"

# Force TLS 1.2 / TLS 1.3 for GitHub compatibility on Windows PowerShell
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13

$repo = "karan-banwasi/Patchwork"
$installDir = Join-Path $env:LOCALAPPDATA "Programs\Patchwork\bin"
$exePath = Join-Path $installDir "pw.exe"

# Create target directory if it doesn't exist
if (-not (Test-Path -Path $installDir)) {
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
}

Write-Host "Installing Patchwork (pw)..." -ForegroundColor Cyan

# 1. Try downloading via GitHub CLI (supports authenticated private repositories out-of-the-box)
$downloaded = $false
if (Get-Command gh -ErrorAction SilentlyContinue) {
    try {
        Write-Host "Attempting download via GitHub CLI (authenticated)..." -ForegroundColor Cyan
        & gh release download -R $repo -p "pw.exe" -O $exePath --clobber 2>$null
        if ($LASTEXITCODE -eq 0 -and (Test-Path -Path $exePath)) {
            Write-Host "Successfully downloaded pw.exe via GitHub CLI" -ForegroundColor Green
            $downloaded = $true
        }
    } catch {
        # Fall through to REST API download
    }
}

# 2. Try REST download (supports public repos & token-authenticated private repos)
if (-not $downloaded) {
    $token = if ($env:GH_TOKEN) { $env:GH_TOKEN } elseif ($env:GITHUB_TOKEN) { $env:GITHUB_TOKEN } else { "" }
    $headers = @{ "User-Agent" = "PatchworkInstaller" }
    if ($token) {
        $headers["Authorization"] = "Bearer $token"
    }

    $downloadUrl = "https://github.com/$repo/releases/latest/download/pw.exe"
    $assetApiUrl = ""
    $releaseTag = ""

    try {
        $releases = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases" -Headers $headers -UseBasicParsing
        foreach ($rel in $releases) {
            if ($rel.draft) { continue }
            $asset = $rel.assets | Where-Object { $_.name -eq "pw.exe" } | Select-Object -First 1
            if ($asset) {
                $downloadUrl = $asset.browser_download_url
                $assetApiUrl = $asset.url
                $releaseTag = $rel.tag_name
                break
            }
        }
    } catch {
        # Fall back to direct latest download URL
    }

    $tagMsg = if ($releaseTag) { " $releaseTag" } else { "" }
    Write-Host "Downloading release$tagMsg from GitHub..."

    try {
        if ($token -and $assetApiUrl) {
            # Private repository asset download via GitHub REST API
            $apiHeaders = @{
                "User-Agent"    = "PatchworkInstaller"
                "Authorization" = "Bearer $token"
                "Accept"        = "application/octet-stream"
            }
            Invoke-WebRequest -Uri $assetApiUrl -OutFile $exePath -Headers $apiHeaders -UseBasicParsing
        } else {
            # Public repository download
            Invoke-WebRequest -Uri $downloadUrl -OutFile $exePath -Headers $headers -UseBasicParsing
        }
        Write-Host "Successfully downloaded pw.exe to $installDir" -ForegroundColor Green
        $downloaded = $true
    } catch {
        $exMsg = $_.Exception.Message
        Write-Error "Failed to download pw.exe ($exMsg).`nFor private repositories, ensure 'gh' CLI is logged in (gh auth login) or GH_TOKEN is set in your environment."
        exit 1
    }
}

# Update User PATH if needed
$userPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
$cleanUserPath = if ($userPath) { $userPath.TrimEnd(';') } else { "" }
if (($cleanUserPath -split ';') -notcontains $installDir) {
    Write-Host "Adding $installDir to User PATH..." -ForegroundColor Yellow
    $newUserPath = if ([string]::IsNullOrWhitespace($cleanUserPath)) { $installDir } else { "$cleanUserPath;$installDir" }
    [Environment]::SetEnvironmentVariable("Path", $newUserPath, [EnvironmentVariableTarget]::User)
    $env:PATH = if ([string]::IsNullOrWhitespace($env:PATH)) { $installDir } else { "$($env:PATH.TrimEnd(';'));$installDir" }
    Write-Host "PATH updated successfully." -ForegroundColor Green
}

Write-Host "`nPatchwork has been installed successfully!" -ForegroundColor Green
Write-Host "Run 'pw --help' or 'pw check' in a new terminal window to get started." -ForegroundColor Cyan

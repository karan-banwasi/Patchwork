# Install script for Patchwork (pw)
$ErrorActionPreference = "Stop"

$repo = "karan-banwasi/Patchwork"
$installDir = Join-Path $env:LOCALAPPDATA "Programs\Patchwork\bin"
$exePath = Join-Path $installDir "pw.exe"
# Resolve latest download URL (finds most recent release containing pw.exe)
$downloadUrl = "https://github.com/$repo/releases/latest/download/pw.exe"
$releaseTag = ""

$headers = @{ "User-Agent" = "PatchworkInstaller" }

try {
    $releases = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases" -Headers $headers -UseBasicParsing
    foreach ($rel in $releases) {
        if ($rel.draft) { continue }
        $asset = $rel.assets | Where-Object { $_.name -eq "pw.exe" } | Select-Object -First 1
        if ($asset -and $asset.browser_download_url) {
            $downloadUrl = $asset.browser_download_url
            $releaseTag = $rel.tag_name
            break
        }
    }
} catch {
    # Fall back to direct latest download URL if GitHub API query fails
}

Write-Host "Installing Patchwork (pw)..." -ForegroundColor Cyan

# Create target directory if it doesn't exist
if (-not (Test-Path -Path $installDir)) {
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
}

# Download binary
$tagMsg = if ($releaseTag) { " $releaseTag" } else { "" }
Write-Host "Downloading release$tagMsg from GitHub ($downloadUrl)..."
try {
    Invoke-WebRequest -Uri $downloadUrl -OutFile $exePath -Headers $headers -UseBasicParsing
    Write-Host "Successfully downloaded pw.exe to $installDir" -ForegroundColor Green
} catch {
    Write-Error "Failed to download pw.exe. Make sure a valid release with pw.exe has been published on GitHub."
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
    Write-Host "PATH updated successfully." -ForegroundColor Green
}

Write-Host "`nPatchwork has been installed successfully!" -ForegroundColor Green
Write-Host "Run 'pw --help' or 'pw check' in a new terminal window to get started." -ForegroundColor Cyan

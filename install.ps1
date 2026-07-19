# Install script for Patchwork (pw)
$ErrorActionPreference = "Stop"

$repo = "karan-banwasi/Patchwork"
$installDir = Join-Path $env:LOCALAPPDATA "Programs\Patchwork\bin"
$exePath = Join-Path $installDir "pw.exe"
$downloadUrl = "https://github.com/$repo/releases/latest/download/pw.exe"

Write-Host "Installing Patchwork (pw)..." -ForegroundColor Cyan

# Create target directory if it doesn't exist
if (-not (Test-Path -Path $installDir)) {
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
}

# Download binary
Write-Host "Downloading latest release from GitHub ($downloadUrl)..."
try {
    Invoke-WebRequest -Uri $downloadUrl -OutFile $exePath -UseBasicParsing
    Write-Host "Successfully downloaded pw.exe to $installDir" -ForegroundColor Green
} catch {
    Write-Error "Failed to download pw.exe. Make sure a release has been published on GitHub."
    exit 1
}

# Update User PATH if needed
$userPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
if ($userPath -split ';' -notcontains $installDir) {
    Write-Host "Adding $installDir to User PATH..." -ForegroundColor Yellow
    $newUserPath = if ([string]::IsNullOrWhitespace($userPath)) { $installDir } else { "$userPath;$installDir" }
    [Environment]::SetEnvironmentVariable("Path", $newUserPath, [EnvironmentVariableTarget]::User)
    $env:PATH = "$env:PATH;$installDir"
    Write-Host "PATH updated successfully." -ForegroundColor Green
}

Write-Host "`nPatchwork has been installed successfully!" -ForegroundColor Green
Write-Host "Run 'pw --help' or 'pw check' in a new terminal window to get started." -ForegroundColor Cyan

<#
.SYNOPSIS
    Submits the Patchwork Winget manifest to microsoft/winget-pkgs using sparse git checkout + GitHub CLI (gh),
    avoiding the GitHub REST Contents API 429 rate-limit ("gitmon refuses to schedule us") in wingetcreate.
.PARAMETER Version
    Optional version to submit (e.g. "0.0.22"). If omitted, detects the latest folder under winget/manifests/k/KaranBanwasi/Patchwork/.
#>
param(
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$pkgId = "KaranBanwasi.Patchwork"
$pkgRoot = Join-Path $PSScriptRoot "manifests\k\KaranBanwasi\Patchwork"

if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
    Write-Error "GitHub CLI ('gh') is required. Install it via 'winget install GitHub.cli' and run 'gh auth login'."
    exit 1
}

if (-not $Version) {
    if (-not (Test-Path $pkgRoot)) {
        & (Join-Path $PSScriptRoot "generate-manifest.ps1")
    }
    $latestDir = Get-ChildItem -Path $pkgRoot -Directory | Sort-Object Name -Descending | Select-Object -First 1
    if (-not $latestDir) {
        Write-Error "No generated manifest version found under $pkgRoot"
        exit 1
    }
    $Version = $latestDir.Name
}

$verClean = $Version.TrimStart("v")
$sourceManifestDir = Join-Path $pkgRoot $verClean
if (-not (Test-Path $sourceManifestDir)) {
    Write-Host "Generating manifests for version $verClean..." -ForegroundColor Cyan
    & (Join-Path $PSScriptRoot "generate-manifest.ps1") -Version $verClean
}

Write-Host "Validating manifest for $pkgId version $verClean..." -ForegroundColor Cyan
& winget validate --manifest $sourceManifestDir
if ($LASTEXITCODE -ne 0) {
    Write-Error "winget validate failed."
    exit 1
}

$ghUser = (& gh api user --jq .login).Trim()
if (-not $ghUser) {
    Write-Error "Not authenticated with GitHub CLI. Run 'gh auth login' first."
    exit 1
}

Write-Host "Ensuring fork of microsoft/winget-pkgs exists under $ghUser..." -ForegroundColor Cyan
& gh repo fork microsoft/winget-pkgs --clone=false

$workDir = Join-Path ([System.IO.Path]::GetTempPath()) "winget-pkgs-sparse-$([Guid]::NewGuid().ToString('N').Substring(0,8))"
New-Item -ItemType Directory -Force -Path $workDir | Out-Null

try {
    Push-Location $workDir

    Write-Host "Cloning partial/sparse checkout of $ghUser/winget-pkgs..." -ForegroundColor Cyan
    & git clone --filter=blob:none --no-checkout --depth 1 "https://github.com/$ghUser/winget-pkgs.git" .
    & git remote add upstream "https://github.com/microsoft/winget-pkgs.git" 2>$null
    & git fetch upstream master --depth 1

    & git sparse-checkout init --cone
    & git sparse-checkout set "manifests/k/KaranBanwasi/Patchwork"
    & git checkout -B "patchwork-$verClean" FETCH_HEAD

    $destDir = Join-Path $workDir "manifests\k\KaranBanwasi\Patchwork\$verClean"
    New-Item -ItemType Directory -Force -Path $destDir | Out-Null
    Copy-Item -Path (Join-Path $sourceManifestDir "*.yaml") -Destination $destDir -Force

    & git add "manifests/k/KaranBanwasi/Patchwork/$verClean"
    & git commit -m "New package: $pkgId version $verClean"
    & git push -u origin "patchwork-$verClean" --force

    $existingPr = (& gh pr list --repo microsoft/winget-pkgs --head "${ghUser}:patchwork-$verClean" --json url --jq ".[0].url")
    if ($existingPr) {
        Write-Host "Updated existing Pull Request: $existingPr" -ForegroundColor Green
    } else {
        Write-Host "Creating Pull Request to microsoft/winget-pkgs..." -ForegroundColor Cyan
        & gh pr create `
            --repo microsoft/winget-pkgs `
            --head "${ghUser}:patchwork-$verClean" `
            --base master `
            --title "New package: $pkgId version $verClean" `
            --body "###Checklist`n- [x] Have you signed the [Contributor License Agreement](https://cla.opensource.microsoft.com/)?`n- [x] Have you validated the manifest locally with ``winget validate``?"
    }
} finally {
    Pop-Location
    Remove-Item -Path $workDir -Recurse -Force -ErrorAction SilentlyContinue
}

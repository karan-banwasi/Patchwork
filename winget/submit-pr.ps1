<#
.SYNOPSIS
    Submits the Patchwork Winget manifest to microsoft/winget-pkgs using sparse git checkout + GitHub CLI (gh),
    avoiding the GitHub REST Contents API 429 rate-limit ("gitmon refuses to schedule us") in wingetcreate.
.PARAMETER Version
    Optional version to submit (e.g. "0.2.2"). If omitted, detects the latest folder under winget/manifests/k/KaranBanwasi/Patchwork/.
.PARAMETER Branch
    Optional branch name to target. If omitted, detects any open PR for this package or defaults to "patchwork-$verClean".
.PARAMETER PrNumber
    Optional PR number in microsoft/winget-pkgs to update directly.
#>
param(
    [string]$Version = "",
    [string]$Branch = "",
    [string]$PrNumber = ""
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

$targetBranch = $Branch
$existingPrNumber = $PrNumber
$existingPrUrl = ""

if ($existingPrNumber) {
    $prData = (& gh pr view $existingPrNumber --repo microsoft/winget-pkgs --json headRefName,url | ConvertFrom-Json)
    if ($prData) {
        if (-not $targetBranch) {
            $targetBranch = $prData.headRefName
        }
        $existingPrUrl = $prData.url
    }
} else {
    Write-Host "Checking for existing open PRs for $pkgId by $ghUser..." -ForegroundColor Cyan
    $prList = (& gh pr list --repo microsoft/winget-pkgs --author $ghUser --search "$pkgId in:title" --json number,headRefName,url | ConvertFrom-Json)
    if ($prList -and $prList.Count -gt 0) {
        $existingPrNumber = $prList[0].number
        $existingPrUrl = $prList[0].url
        if (-not $targetBranch) {
            $targetBranch = $prList[0].headRefName
        }
        Write-Host "Found existing open PR: #$existingPrNumber ($existingPrUrl) on branch '$targetBranch'" -ForegroundColor Yellow
    }
}

if (-not $targetBranch) {
    $targetBranch = "patchwork-$verClean"
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
    & git checkout -B $targetBranch FETCH_HEAD

    $destDir = Join-Path $workDir "manifests\k\KaranBanwasi\Patchwork\$verClean"
    New-Item -ItemType Directory -Force -Path $destDir | Out-Null
    Copy-Item -Path (Join-Path $sourceManifestDir "*.yaml") -Destination $destDir -Force

    & git add "manifests/k/KaranBanwasi/Patchwork"
    & git commit -m "New package: $pkgId version $verClean"
    & git push -u origin $targetBranch --force

    if ($existingPrNumber) {
        Write-Host "Updating title for PR #$existingPrNumber..." -ForegroundColor Cyan
        & gh pr edit $existingPrNumber --repo microsoft/winget-pkgs --title "New package: $pkgId version $verClean"
        Write-Host "Updated existing Pull Request: $existingPrUrl" -ForegroundColor Green
    } else {
        Write-Host "Creating Pull Request to microsoft/winget-pkgs..." -ForegroundColor Cyan
        $newPr = & gh pr create `
            --repo microsoft/winget-pkgs `
            --head "${ghUser}:${targetBranch}" `
            --base master `
            --title "New package: $pkgId version $verClean" `
            --body "###Checklist`n- [x] Have you signed the [Contributor License Agreement](https://cla.opensource.microsoft.com/)?`n- [x] Have you validated the manifest locally with ``winget validate``?"
        Write-Host "Created Pull Request: $newPr" -ForegroundColor Green
    }
} finally {
    Pop-Location
    Remove-Item -Path $workDir -Recurse -Force -ErrorAction SilentlyContinue
}

<#
.SYNOPSIS
    Generates and validates the Windows Package Manager (winget-pkgs) manifest files for Patchwork.
.DESCRIPTION
    Downloads the latest (or specified) release assets from GitHub (pw.exe and pw-arm64.exe),
    computes their SHA256 hashes, writes the multi-file Winget v1.6.0 manifests under
    winget/manifests/k/KaranBanwasi/Patchwork/<version>/, and runs `winget validate`.
.PARAMETER Version
    Optional version string (e.g. "0.0.22" or "v0.0.22"). If omitted, queries the latest GitHub release.
#>
param(
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$repo = "karan-banwasi/Patchwork"
$pkgId = "KaranBanwasi.Patchwork"

if (-not $Version) {
    Write-Host "Fetching latest release from GitHub ($repo)..." -ForegroundColor Cyan
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -UseBasicParsing
    $Version = $rel.tag_name
}

$tag = if ($Version.StartsWith("v")) { $Version } else { "v$Version" }
$verClean = $Version.TrimStart("v")

$urlX64 = "https://github.com/$repo/releases/download/$tag/pw.exe"
$urlArm64 = "https://github.com/$repo/releases/download/$tag/pw-arm64.exe"

$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "patchwork-winget-$verClean"
New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null

$x64File = Join-Path $tmpDir "pw.exe"
$arm64File = Join-Path $tmpDir "pw-arm64.exe"

Write-Host "Downloading $tag assets to calculate SHA256..." -ForegroundColor Cyan
Invoke-WebRequest -Uri $urlX64 -OutFile $x64File -UseBasicParsing
Invoke-WebRequest -Uri $urlArm64 -OutFile $arm64File -UseBasicParsing

$shaX64 = (Get-FileHash -Path $x64File -Algorithm SHA256).Hash.ToUpper()
$shaArm64 = (Get-FileHash -Path $arm64File -Algorithm SHA256).Hash.ToUpper()

Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue

Write-Host "  x64 SHA256:   $shaX64" -ForegroundColor Green
Write-Host "  arm64 SHA256: $shaArm64" -ForegroundColor Green

$manifestDir = Join-Path $PSScriptRoot "manifests\k\KaranBanwasi\Patchwork\$verClean"
New-Item -ItemType Directory -Force -Path $manifestDir | Out-Null

$versionYaml = @"
# Created with Patchwork Winget Manifest Generator
# yaml-language-server: `$schema=https://aka.ms/winget-manifest.version.1.6.0.schema.json

PackageIdentifier: $pkgId
PackageVersion: $verClean
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.6.0
"@

$localeYaml = @"
# Created with Patchwork Winget Manifest Generator
# yaml-language-server: `$schema=https://aka.ms/winget-manifest.defaultLocale.1.6.0.schema.json

PackageIdentifier: $pkgId
PackageVersion: $verClean
PackageLocale: en-US
Publisher: Karan Banwasi
PublisherUrl: https://github.com/karan-banwasi
PublisherSupportUrl: https://github.com/karan-banwasi/Patchwork/issues
Author: Karan Banwasi
PackageName: Patchwork
PackageUrl: https://github.com/karan-banwasi/Patchwork
License: MIT
LicenseUrl: https://github.com/karan-banwasi/Patchwork/blob/main/LICENSE
Copyright: Copyright (c) Karan Banwasi
ShortDescription: A fast Windows CLI utility to check and manage package updates across Winget, Scoop, Chocolatey, npm, and .NET tools.
Description: Patchwork (pw) is a native Windows CLI tool that concurrently checks and interactively upgrades outdated packages across Windows Package Manager (winget), Scoop, Chocolatey, npm global packages, and .NET global tools.
Moniker: patchwork
Tags:
  - cli
  - package-manager
  - updater
  - winget
  - scoop
  - chocolatey
  - patchwork
ReleaseNotesUrl: https://github.com/karan-banwasi/Patchwork/releases/tag/$tag
ManifestType: defaultLocale
ManifestVersion: 1.6.0
"@

$installerYaml = @"
# Created with Patchwork Winget Manifest Generator
# yaml-language-server: `$schema=https://aka.ms/winget-manifest.installer.1.6.0.schema.json

PackageIdentifier: $pkgId
PackageVersion: $verClean
InstallerType: portable
Commands:
  - pw
Installers:
  - Architecture: x64
    InstallerUrl: $urlX64
    InstallerSha256: $shaX64
  - Architecture: arm64
    InstallerUrl: $urlArm64
    InstallerSha256: $shaArm64
ManifestType: installer
ManifestVersion: 1.6.0
"@

Set-Content -Path (Join-Path $manifestDir "$pkgId.yaml") -Value $versionYaml -Encoding UTF8
Set-Content -Path (Join-Path $manifestDir "$pkgId.locale.en-US.yaml") -Value $localeYaml -Encoding UTF8
Set-Content -Path (Join-Path $manifestDir "$pkgId.installer.yaml") -Value $installerYaml -Encoding UTF8

# Also update template manifests in winget/ root
Set-Content -Path (Join-Path $PSScriptRoot "$pkgId.yaml") -Value $versionYaml -Encoding UTF8
Set-Content -Path (Join-Path $PSScriptRoot "$pkgId.locale.en-US.yaml") -Value $localeYaml -Encoding UTF8
Set-Content -Path (Join-Path $PSScriptRoot "$pkgId.installer.yaml") -Value $installerYaml -Encoding UTF8

Write-Host "`nGenerated manifests in: $manifestDir" -ForegroundColor Cyan

if (Get-Command winget -ErrorAction SilentlyContinue) {
    Write-Host "Validating manifests with 'winget validate'..." -ForegroundColor Cyan
    & winget validate --manifest $manifestDir
}

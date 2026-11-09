# Unit tests for install.ps1 Get-ExpectedChecksum logic
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$installScript = Join-Path (Split-Path -Parent $scriptDir) "install.ps1"

# Load functions from install.ps1
. $installScript -FunctionsOnly

$passCount = 0
$failCount = 0

function Assert-Equal {
    param(
        [string]$TestName,
        [string]$Actual,
        [string]$Expected
    )
    if ($Actual -eq $Expected) {
        Write-Host "  [PASS] $TestName" -ForegroundColor Green
        $script:passCount++
    } else {
        Write-Host "  [FAIL] ${TestName}: expected '$Expected', got '$Actual'" -ForegroundColor Red
        $script:failCount++
    }
}

Write-Host "Running install.ps1 unit tests..." -ForegroundColor Cyan

# Test 1: Standard SHA256SUMS parsing for pw.exe (x64)
$sums1 = @"
3a8f4c2e01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805  pw.exe
5b9a1d3fe3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b  pw-arm64.exe
"@
$hash1 = Get-ExpectedChecksum -Content $sums1 -AssetName "pw.exe" -WingetArch "x64"
Assert-Equal "Standard SHA256SUMS (pw.exe)" $hash1 "3A8F4C2E01BA4719C80B6FE911B091A7C05124B64EEECE964E09C058EF8F9805"

# Test 2: Standard SHA256SUMS parsing for pw-arm64.exe (arm64)
$hash2 = Get-ExpectedChecksum -Content $sums1 -AssetName "pw-arm64.exe" -WingetArch "arm64"
Assert-Equal "Standard SHA256SUMS (pw-arm64.exe)" $hash2 "5B9A1D3FE3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B"

# Test 3: Formatting variations (single space, multiple spaces, tabs)
$sums3 = "3a8f4c2e01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805`tpw.exe`r`n"
$hash3 = Get-ExpectedChecksum -Content $sums3 -AssetName "pw.exe" -WingetArch "x64"
Assert-Equal "Tab-separated SHA256SUMS" $hash3 "3A8F4C2E01BA4719C80B6FE911B091A7C05124B64EEECE964E09C058EF8F9805"

# Test 4: Binary mode asterisk (*pw.exe)
$sums4 = "3a8f4c2e01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805 *pw.exe`n"
$hash4 = Get-ExpectedChecksum -Content $sums4 -AssetName "pw.exe" -WingetArch "x64"
Assert-Equal "Binary mode asterisk (*pw.exe)" $hash4 "3A8F4C2E01BA4719C80B6FE911B091A7C05124B64EEECE964E09C058EF8F9805"

# Test 5: Relative path prefix (./pw.exe and .\pw.exe)
$sums5a = "3a8f4c2e01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805  ./pw.exe`n"
$hash5a = Get-ExpectedChecksum -Content $sums5a -AssetName "pw.exe" -WingetArch "x64"
Assert-Equal "Relative path prefix (./pw.exe)" $hash5a "3A8F4C2E01BA4719C80B6FE911B091A7C05124B64EEECE964E09C058EF8F9805"

$sums5b = "3a8f4c2e01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805  .\pw.exe`n"
$hash5b = Get-ExpectedChecksum -Content $sums5b -AssetName "pw.exe" -WingetArch "x64"
Assert-Equal "Relative path prefix (.\pw.exe)" $hash5b "3A8F4C2E01BA4719C80B6FE911B091A7C05124B64EEECE964E09C058EF8F9805"

# Test 6: CRLF vs LF line endings
$sums6 = "3a8f4c2e01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805  pw.exe`r`n5b9a1d3fe3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b  pw-arm64.exe`r`n"
$hash6 = Get-ExpectedChecksum -Content $sums6 -AssetName "pw-arm64.exe" -WingetArch "arm64"
Assert-Equal "CRLF line endings" $hash6 "5B9A1D3FE3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B"

# Test 7: Fallback to Winget installer YAML for x64
$yaml7 = @"
PackageIdentifier: KaranBanwasi.Patchwork
PackageVersion: 0.0.22
InstallerType: portable
Installers:
  - Architecture: x64
    InstallerUrl: https://github.com/karan-banwasi/Patchwork/releases/download/v0.0.22/pw.exe
    InstallerSha256: AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899
  - Architecture: arm64
    InstallerUrl: https://github.com/karan-banwasi/Patchwork/releases/download/v0.0.22/pw-arm64.exe
    InstallerSha256: 11223344556677889900AABBCCDDEEFF11223344556677889900AABBCCDDEEFF
"@
$hash7 = Get-ExpectedChecksum -Content $yaml7 -AssetName "pw.exe" -WingetArch "x64"
Assert-Equal "Winget YAML fallback (x64)" $hash7 "AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899"

# Test 8: Fallback to Winget installer YAML for arm64
$hash8 = Get-ExpectedChecksum -Content $yaml7 -AssetName "pw-arm64.exe" -WingetArch "arm64"
Assert-Equal "Winget YAML fallback (arm64)" $hash8 "11223344556677889900AABBCCDDEEFF11223344556677889900AABBCCDDEEFF"

# Test 9: Fail-closed on missing target asset in manifest -> returns $null
$sums9 = "3a8f4c2e01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805  other-file.exe`n"
$hash9 = Get-ExpectedChecksum -Content $sums9 -AssetName "pw.exe" -WingetArch "x64"
Assert-Equal "Missing target asset returns null" ([string]$hash9) ""

# Test 10: Fail-closed on corrupted / unparseable content -> returns $null
$corrupt = "This is not a valid SHA256SUMS file or winget manifest"
$hash10 = Get-ExpectedChecksum -Content $corrupt -AssetName "pw.exe" -WingetArch "x64"
Assert-Equal "Corrupted manifest returns null" ([string]$hash10) ""

$resultColor = if ($failCount -eq 0) { 'Green' } else { 'Red' }
Write-Host "`nTest Results: $passCount passed, $failCount failed." -ForegroundColor $resultColor
if ($failCount -ne 0) {
    exit 1
}
exit 0

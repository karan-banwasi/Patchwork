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

function Clean-Stderr {
    param([string]$Raw)
    return (($Raw -replace '\x1b\[[0-9;]*m', '') -replace '(?m)^\s*\|\s*', ' ' -replace '\s+', ' ').Trim()
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

# Test 11: Test-AttestationProvenance -SkipAttestation returns true
$res11 = Test-AttestationProvenance -BinaryPath "dummy.exe" -Repo "karan-banwasi/Patchwork" -SkipAttestation
Assert-Equal "Test-AttestationProvenance -SkipAttestation returns true" "$res11" "True"

# Test 12: Test-AttestationProvenance -RequireAttestation without gh throws
$threw12 = $false
try {
    Test-AttestationProvenance -BinaryPath "dummy.exe" -Repo "karan-banwasi/Patchwork" -RequireAttestation -HasGh:$false
} catch {
    $threw12 = $true
}
Assert-Equal "Test-AttestationProvenance -RequireAttestation without gh throws" "$threw12" "True"

# Test 13: Test-AttestationProvenance verification failure throws security warning (mock runner)
$threw13 = $false
$errMsg13 = ""
$global:LASTEXITCODE = 0
try {
    Test-AttestationProvenance -BinaryPath "dummy.exe" -Repo "karan-banwasi/Patchwork" `
        -SignerWorkflow "karan-banwasi/Patchwork/.github/workflows/release.yml" `
        -HasGh:$true `
        -GhRunner {
            param($RunnerArgs)
            return [PSCustomObject]@{
                Output = "simulated verification error: signature invalid"
                ExitCode = 1
            }
        }
} catch {
    $threw13 = $true
    $errMsg13 = $_.Exception.Message
}
Assert-Equal "Test-AttestationProvenance failure throws" "$threw13" "True"
$hasSecurityWarning = $errMsg13 -like "*SECURITY WARNING*"
Assert-Equal "Test-AttestationProvenance failure includes SECURITY WARNING" "$hasSecurityWarning" "True"

# Test 14: Test-AttestationProvenance verification success returns true
$capturedArgs14 = @()
$global:LASTEXITCODE = 1 # Prove that stale dirty exit code does not pollute successful runner
$res14 = Test-AttestationProvenance -BinaryPath "dummy.exe" -Repo "karan-banwasi/Patchwork" `
    -SignerWorkflow "karan-banwasi/Patchwork/.github/workflows/release.yml" `
    -HasGh:$true `
    -GhRunner {
        param($RunnerArgs)
        $script:capturedArgs14 = $RunnerArgs
        return [PSCustomObject]@{
            Output = "Verified OK"
            ExitCode = 0
        }
    }
Assert-Equal "Test-AttestationProvenance success returns true" "$res14" "True"
$hasSignerWorkflow = ($script:capturedArgs14 -contains "--signer-workflow") -and ($script:capturedArgs14 -contains "karan-banwasi/Patchwork/.github/workflows/release.yml")
Assert-Equal "Test-AttestationProvenance passes --signer-workflow" "$hasSignerWorkflow" "True"

# Test 15: Test-AttestationProvenance with both -SkipAttestation and -RequireAttestation throws
$threw15 = $false
try {
    Test-AttestationProvenance -BinaryPath "dummy.exe" -Repo "karan-banwasi/Patchwork" -SkipAttestation -RequireAttestation
} catch {
    $threw15 = $true
}
Assert-Equal "Test-AttestationProvenance mutually exclusive flags throw" "$threw15" "True"

# Test 16: Test-AttestationProvenance default runner fails on unverified binary (real gh execution)
$hasGh = [bool](Get-Command gh -ErrorAction SilentlyContinue)
if ($hasGh) {
    $dummyPath = [System.IO.Path]::GetTempFileName()
    "unverified mock binary payload" | Set-Content -Path $dummyPath
    $threw16 = $false
    $errMsg16 = ""
    $global:LASTEXITCODE = 0
    try {
        Test-AttestationProvenance -BinaryPath $dummyPath -Repo "karan-banwasi/Patchwork" `
            -SignerWorkflow "karan-banwasi/Patchwork/.github/workflows/release.yml"
    } catch {
        $threw16 = $true
        $errMsg16 = $_.Exception.Message
    } finally {
        Remove-Item -Path $dummyPath -Force -ErrorAction SilentlyContinue
    }
    Assert-Equal "Test-AttestationProvenance default runner throws on unverified binary" "$threw16" "True"
    $hasSecurityWarning16 = $errMsg16 -like "*SECURITY WARNING*"
    Assert-Equal "Test-AttestationProvenance default runner includes SECURITY WARNING" "$hasSecurityWarning16" "True"
}

# Test 17: Test-AttestationProvenance handles simple integer exit code from runner
$threw17 = $false
$errMsg17 = ""
$global:LASTEXITCODE = 0
try {
    Test-AttestationProvenance -BinaryPath "dummy.exe" -Repo "karan-banwasi/Patchwork" `
        -SignerWorkflow "karan-banwasi/Patchwork/.github/workflows/release.yml" `
        -HasGh:$true `
        -GhRunner {
            param($RunnerArgs)
            return 1
        }
} catch {
    $threw17 = $true
    $errMsg17 = $_.Exception.Message
}
Assert-Equal "Test-AttestationProvenance integer exit code failure throws" "$threw17" "True"
$hasSecurityWarning17 = $errMsg17 -like "*SECURITY WARNING*"
Assert-Equal "Test-AttestationProvenance integer exit code includes SECURITY WARNING" "$hasSecurityWarning17" "True"

# Test 18: Standalone script early rejection on mutually exclusive flags (-SkipAttestation and -RequireAttestation)
$currentShell = (Get-Process -Id $PID).Path
$pinfo18 = New-Object System.Diagnostics.ProcessStartInfo
$pinfo18.FileName = $currentShell
$pinfo18.Arguments = "-NoProfile -ExecutionPolicy Bypass -File `"$installScript`" -SkipAttestation -RequireAttestation"
$pinfo18.RedirectStandardOutput = $true
$pinfo18.RedirectStandardError = $true
$pinfo18.UseShellExecute = $false

$proc18 = [System.Diagnostics.Process]::Start($pinfo18)
$stdout18 = $proc18.StandardOutput.ReadToEnd()
$stderr18 = $proc18.StandardError.ReadToEnd()
$proc18.WaitForExit()

Assert-Equal "Standalone script exits 1 on mutually exclusive flags" "$($proc18.ExitCode)" "1"
$cleanStderr18 = Clean-Stderr $stderr18
$hasMutexMsg18 = $cleanStderr18 -like "*Cannot specify both -SkipAttestation and -RequireAttestation*"
Assert-Equal "Standalone script reports mutual exclusivity error" "$hasMutexMsg18" "True"
$noNetwork18 = ($stdout18 -notlike "*Attempting download*") -and ($stdout18 -notlike "*Downloading release*")
Assert-Equal "Standalone script rejects mutually exclusive flags before network requests" "$noNetwork18" "True"

# Test 19: Standalone script early rejection on -RequireAttestation when gh is absent from PATH
$cleanPath = "C:\Windows\System32;C:\Windows;C:\Windows\System32\WindowsPowerShell\v1.0;C:\Program Files\PowerShell\7"
$pinfo19 = New-Object System.Diagnostics.ProcessStartInfo
$pinfo19.FileName = $currentShell
$pinfo19.Arguments = "-NoProfile -ExecutionPolicy Bypass -File `"$installScript`" -RequireAttestation"
$pinfo19.RedirectStandardOutput = $true
$pinfo19.RedirectStandardError = $true
$pinfo19.UseShellExecute = $false
$pinfo19.EnvironmentVariables["PATH"] = $cleanPath

$proc19 = [System.Diagnostics.Process]::Start($pinfo19)
$stdout19 = $proc19.StandardOutput.ReadToEnd()
$stderr19 = $proc19.StandardError.ReadToEnd()
$proc19.WaitForExit()

Assert-Equal "Standalone script exits 1 on -RequireAttestation without gh" "$($proc19.ExitCode)" "1"
$cleanStderr19 = Clean-Stderr $stderr19
$hasNoGhMsg19 = $cleanStderr19 -like "*GitHub CLI ('gh') is not installed or available on PATH*"
Assert-Equal "Standalone script reports missing gh error" "$hasNoGhMsg19" "True"
$noNetwork19 = ($stdout19 -notlike "*Attempting download*") -and ($stdout19 -notlike "*Downloading release*")
Assert-Equal "Standalone script rejects missing gh before network requests" "$noNetwork19" "True"

$resultColor = if ($failCount -eq 0) { 'Green' } else { 'Red' }
Write-Host "`nTest Results: $passCount passed, $failCount failed." -ForegroundColor $resultColor
if ($failCount -ne 0) {
    exit 1
}
exit 0

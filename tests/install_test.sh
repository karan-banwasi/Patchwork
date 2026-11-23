#!/bin/sh
# Unit tests for install.sh
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_SH="$SCRIPT_DIR/../install.sh"

FAILED=0
PASSED=0

assert_eq() {
    expected="$1"
    actual="$2"
    desc="$3"
    if [ "$expected" = "$actual" ]; then
        printf "  PASS: %s\n" "$desc"
        PASSED=$((PASSED + 1))
    else
        printf "  FAIL: %s (expected '%s', got '%s')\n" "$desc" "$expected" "$actual"
        FAILED=$((FAILED + 1))
    fi
}

assert_success() {
    desc="$1"
    shift
    if "$@"; then
        printf "  PASS: %s\n" "$desc"
        PASSED=$((PASSED + 1))
    else
        printf "  FAIL: %s (command failed: %s)\n" "$desc" "$*"
        FAILED=$((FAILED + 1))
    fi
}

assert_failure() {
    desc="$1"
    shift
    if "$@"; then
        printf "  FAIL: %s (expected command to fail: %s)\n" "$desc" "$*"
        FAILED=$((FAILED + 1))
    else
        printf "  PASS: %s\n" "$desc"
        PASSED=$((PASSED + 1))
    fi
}

echo "=== Running install.sh test suite ==="

# Source install.sh in functions-only mode
export PW_FUNCTIONS_ONLY=1
# shellcheck disable=SC1090
. "$INSTALL_SH"

# Test 1: detect_os
printf "\n--- Testing detect_os ---\n"
assert_eq "darwin" "$(detect_os Darwin)" "Darwin maps to darwin"
assert_eq "linux" "$(detect_os Linux)" "Linux maps to linux"
assert_failure "FreeBSD should fail" detect_os FreeBSD
assert_failure "Windows_NT should fail" detect_os Windows_NT

# Test 2: detect_arch
printf "\n--- Testing detect_arch ---\n"
assert_eq "amd64" "$(detect_arch x86_64)" "x86_64 maps to amd64"
assert_eq "amd64" "$(detect_arch amd64)" "amd64 maps to amd64"
assert_eq "arm64" "$(detect_arch arm64)" "arm64 maps to arm64"
assert_eq "arm64" "$(detect_arch aarch64)" "aarch64 maps to arm64"
assert_failure "i386 should fail" detect_arch i386
assert_failure "mips64 should fail" detect_arch mips64

# Test 3: get_expected_checksum
printf "\n--- Testing get_expected_checksum ---\n"
TEST_TMP="$(mktemp -d 2>/dev/null || mktemp -d -t 'test-sums')"
TEST_SUMS="$TEST_TMP/SHA256SUMS"
cat <<'EOF' > "$TEST_SUMS"
8fad8b305d4bd6cc1220e087b63fdbd5ab7224ceae01c1d8324be529a7d1329a  pw.exe
9b9d04a32d85ae42dc65d30cb3f9e6f6d8393d4e21323898093bac016509b74a  pw-arm64.exe
03C31CCC2409700B0D1B4C16F004A13F1358A898C8D66A40C5452892540BD5A6 *pw-darwin-arm64
8a369ccc4f3dcc97474375689d25ecd4f48a201be4eba7bad5d04d4b77996d76  ./pw-darwin-amd64
2b70100032d019cb4b8f528294c665644f1c8a8499aad3f796c15b20ffb2b7a0  pw-linux-amd64
dc2312898379b6e4c0e52966da2cf4aaeb1fcf52a1bfa14afa8b112f77a8f875  pw-linux-arm64
EOF

assert_eq "8a369ccc4f3dcc97474375689d25ecd4f48a201be4eba7bad5d04d4b77996d76" \
    "$(get_expected_checksum "$TEST_SUMS" "pw-darwin-amd64")" \
    "Extract checksum with ./ prefix"

assert_eq "03c31ccc2409700b0d1b4c16f004a13f1358a898c8d66a40c5452892540bd5a6" \
    "$(get_expected_checksum "$TEST_SUMS" "pw-darwin-arm64")" \
    "Extract checksum with * prefix and lowercase normalized"

assert_eq "2b70100032d019cb4b8f528294c665644f1c8a8499aad3f796c15b20ffb2b7a0" \
    "$(get_expected_checksum "$TEST_SUMS" "pw-linux-amd64")" \
    "Extract checksum with standard format"

assert_eq "" \
    "$(get_expected_checksum "$TEST_SUMS" "nonexistent-asset")" \
    "Nonexistent asset returns empty string"

# Test 4: verify_checksum
printf "\n--- Testing verify_checksum ---\n"
DUMMY_FILE="$TEST_TMP/test_file.bin"
printf "hello patchwork" > "$DUMMY_FILE"
# echo -n "hello patchwork" sha256 is:
# dcfbb323ae924c5bbfa775d5e2363b71ca0ae004a8b417eef9e3e78a2cbbe62c
CALC_SHA="$(calculate_sha256 "$DUMMY_FILE")"
assert_success "verify_checksum succeeds with matching hash" \
    verify_checksum "$DUMMY_FILE" "$CALC_SHA"

UPPER_SHA="$(printf "%s" "$CALC_SHA" | tr '[:lower:]' '[:upper:]')"
assert_success "verify_checksum is case-insensitive" \
    verify_checksum "$DUMMY_FILE" "$UPPER_SHA"

assert_failure "verify_checksum fails on wrong hash" \
    verify_checksum "$DUMMY_FILE" "0000000000000000000000000000000000000000000000000000000000000000"

# Test 5: verify_attestation
printf "\n--- Testing verify_attestation ---\n"
assert_success "verify_attestation succeeds when skip_attest=1" \
    verify_attestation "$DUMMY_FILE" "karan-banwasi/Patchwork" 1 0 0

assert_success "verify_attestation succeeds when gh missing and require_attest=0" \
    verify_attestation "$DUMMY_FILE" "karan-banwasi/Patchwork" 0 0 0

assert_failure "verify_attestation fails when gh missing and require_attest=1" \
    verify_attestation "$DUMMY_FILE" "karan-banwasi/Patchwork" 0 1 0

# Test 6: CLI Argument Parsing
printf "\n--- Testing CLI flags ---\n"
unset PW_FUNCTIONS_ONLY

assert_success "--help displays usage and returns 0" \
    sh "$INSTALL_SH" --help

assert_failure "--skip-attestation and --require-attestation conflict" \
    sh "$INSTALL_SH" --skip-attestation --require-attestation

assert_failure "Unknown argument fails" \
    sh "$INSTALL_SH" --invalid-flag

# Cleanup
rm -rf "$TEST_TMP"

printf "\n=== Test Results: %d passed, %d failed ===\n" "$PASSED" "$FAILED"
if [ "$FAILED" -ne 0 ]; then
    exit 1
fi
exit 0

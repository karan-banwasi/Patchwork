#!/bin/sh
# Install script for Patchwork (pw) on macOS and Linux
set -eu

REPO="${PW_REPO:-karan-banwasi/Patchwork}"
INSTALL_DIR="${INSTALL_DIR:-}"
SKIP_ATTESTATION=0
REQUIRE_ATTESTATION=0
FUNCTIONS_ONLY="${PW_FUNCTIONS_ONLY:-0}"

# Print formatted messages
log_info() { printf "\033[36m%s\033[0m\n" "$*"; }
log_success() { printf "\033[32m%s\033[0m\n" "$*"; }
log_warning() { printf "\033[33m%s\033[0m\n" "$*"; }
log_error() { printf "\033[31m%s\033[0m\n" "$*" >&2; }

usage() {
    exit_code="${1:-0}"
    cat <<EOF
Usage: install.sh [options]

Options:
    -d, --install-dir <dir>    Directory to install pw into
    -r, --repo <repo>          GitHub repository (default: karan-banwasi/Patchwork)
    --skip-attestation         Skip GitHub build provenance attestation verification
    --require-attestation      Require GitHub build provenance attestation (fails if gh is missing)
    --functions-only           Load functions only without executing installation
    -h, --help                 Display this help message
EOF
    exit "$exit_code"
}

detect_os() {
    raw_os="${1:-$(uname -s)}"
    case "$raw_os" in
        Darwin) echo "darwin" ;;
        Linux)  echo "linux" ;;
        *)
            echo "unsupported"
            return 1
            ;;
    esac
}

detect_arch() {
    raw_arch="${1:-$(uname -m)}"
    case "$raw_arch" in
        x86_64|amd64)   echo "amd64" ;;
        arm64|aarch64)  echo "arm64" ;;
        *)
            echo "unsupported"
            return 1
            ;;
    esac
}

get_expected_checksum() {
    sums_file="$1"
    asset_name="$2"
    awk -v asset="$asset_name" '
        $2 == asset || $2 == "*" asset || $2 == "./" asset || $2 == "*./" asset {
            print tolower($1);
            found=1;
            exit;
        }
        $NF ~ "(^|[*/])" asset "$" {
            print tolower($1);
            found=1;
            exit;
        }
    ' "$sums_file"
}

calculate_sha256() {
    target_file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$target_file" | awk '{print tolower($1)}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$target_file" | awk '{print tolower($1)}'
    elif command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 "$target_file" | awk '{print tolower($NF)}'
    else
        return 1
    fi
}

verify_checksum() {
    target_file="$1"
    expected_sha="$2"
    actual_sha="$(calculate_sha256 "$target_file")"
    if [ -z "$actual_sha" ]; then
        log_error "No SHA-256 hashing tool found (requires sha256sum, shasum, or openssl)."
        return 2
    fi

    exp_lower=$(printf "%s" "$expected_sha" | tr '[:upper:]' '[:lower:]')
    act_lower=$(printf "%s" "$actual_sha" | tr '[:upper:]' '[:lower:]')

    if [ "$exp_lower" != "$act_lower" ]; then
        return 1
    fi
    return 0
}

verify_attestation() {
    file_path="$1"
    repo_name="$2"
    skip_attest="${3:-0}"
    require_attest="${4:-0}"
    has_gh="${5:-0}"

    if [ "$skip_attest" -eq 1 ]; then
        log_warning "Skipping build provenance attestation verification (--skip-attestation)."
        return 0
    fi

    if [ "$has_gh" -eq 0 ]; then
        if [ "$require_attest" -eq 1 ]; then
            log_error "Attestation verification required (--require-attestation), but GitHub CLI ('gh') is not installed or available on PATH."
            return 1
        fi
        log_info "GitHub CLI ('gh') not found; skipping provenance attestation verification."
        return 0
    fi

    log_info "Verifying GitHub build provenance attestation..."
    signer_workflow="$repo_name/.github/workflows/release.yml"
    if ! gh attestation verify "$file_path" --repo "$repo_name" --signer-workflow "$signer_workflow" >/dev/null 2>&1; then
        log_error "SECURITY WARNING: GitHub build provenance attestation verification failed."
        log_error "Refusing to install unverified binary. (Pass --skip-attestation to bypass if you trust this binary.)"
        return 1
    fi
    log_success "Verified GitHub build provenance attestation."
    return 0
}

# Parse options
while [ $# -gt 0 ]; do
    case "$1" in
        -d|--install-dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        -r|--repo)
            REPO="$2"
            shift 2
            ;;
        --skip-attestation)
            SKIP_ATTESTATION=1
            shift
            ;;
        --require-attestation)
            REQUIRE_ATTESTATION=1
            shift
            ;;
        --functions-only)
            FUNCTIONS_ONLY=1
            shift
            ;;
        -h|--help)
            usage 0
            ;;
        *)
            log_error "Unknown argument: $1"
            usage 1
            ;;
    esac
done

if [ "$FUNCTIONS_ONLY" -eq 1 ]; then
    return 0 2>/dev/null || exit 0
fi

if [ "$SKIP_ATTESTATION" -eq 1 ] && [ "$REQUIRE_ATTESTATION" -eq 1 ]; then
    log_error "Cannot specify both --skip-attestation and --require-attestation."
    exit 1
fi

# Detect Operating System
if ! OS=$(detect_os); then
    log_error "Unsupported operating system: $(uname -s). Patchwork install.sh supports macOS and Linux."
    exit 1
fi

# Detect Architecture
if ! ARCH=$(detect_arch); then
    log_error "Unsupported architecture: $(uname -m). Supported architectures are amd64 and arm64."
    exit 1
fi

ASSET_NAME="pw-${OS}-${ARCH}"
SHA_SUMS_NAME="SHA256SUMS"

# Determine Install Directory
if [ -z "$INSTALL_DIR" ]; then
    if [ "$(id -u)" -eq 0 ]; then
        INSTALL_DIR="/usr/local/bin"
    elif [ -w "/usr/local/bin" ]; then
        INSTALL_DIR="/usr/local/bin"
    else
        INSTALL_DIR="$HOME/.local/bin"
    fi
fi

TARGET_BIN="$INSTALL_DIR/pw"

# Setup temporary directory and cleanup trap
TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t 'pw-install')"
cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

log_info "Installing Patchwork (pw) for ${OS}/${ARCH} into ${INSTALL_DIR}..."

DOWNLOADED=0
HAS_GH=0
if command -v gh >/dev/null 2>&1; then
    HAS_GH=1
fi

if [ "$HAS_GH" -eq 1 ]; then
    log_info "Attempting download of latest release via GitHub CLI..."
    if gh release download -R "$REPO" -p "$ASSET_NAME" -p "$SHA_SUMS_NAME" -D "$TMP_DIR" --clobber >/dev/null 2>&1; then
        if [ -f "$TMP_DIR/$ASSET_NAME" ] && [ -f "$TMP_DIR/$SHA_SUMS_NAME" ]; then
            DOWNLOADED=1
        fi
    fi
fi

if [ "$DOWNLOADED" -eq 0 ]; then
    TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
    AUTH_HEADER=""
    if [ -n "$TOKEN" ]; then
        AUTH_HEADER="Authorization: Bearer $TOKEN"
    fi

    log_info "Downloading latest release metadata from GitHub..."
    API_URL="https://api.github.com/repos/$REPO/releases/latest"

    if command -v curl >/dev/null 2>&1; then
        REL_JSON=$(curl -fsSL ${AUTH_HEADER:+-H "$AUTH_HEADER"} "$API_URL")
    elif command -v wget >/dev/null 2>&1; then
        REL_JSON=$(wget -qO- ${AUTH_HEADER:+--header="$AUTH_HEADER"} "$API_URL")
    else
        log_error "Neither curl nor wget is available. Please install one of them."
        exit 1
    fi

    # Extract browser download URLs
    BIN_URL=$(printf "%s\n" "$REL_JSON" | grep -F "browser_download_url" | grep -F "$ASSET_NAME\"" | head -n 1 | cut -d '"' -f 4 || true)
    SHA_URL=$(printf "%s\n" "$REL_JSON" | grep -F "browser_download_url" | grep -F "$SHA_SUMS_NAME\"" | head -n 1 | cut -d '"' -f 4 || true)

    if [ -z "$BIN_URL" ] || [ -z "$SHA_URL" ]; then
        log_error "Could not find assets '$ASSET_NAME' and '$SHA_SUMS_NAME' in latest release."
        exit 1
    fi

    log_info "Downloading $ASSET_NAME and $SHA_SUMS_NAME..."
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$BIN_URL" -o "$TMP_DIR/$ASSET_NAME"
        curl -fsSL "$SHA_URL" -o "$TMP_DIR/$SHA_SUMS_NAME"
    else
        wget -qO "$TMP_DIR/$ASSET_NAME" "$BIN_URL"
        wget -qO "$TMP_DIR/$SHA_SUMS_NAME" "$SHA_URL"
    fi
fi

# Verify SHA-256 Checksum
log_info "Verifying SHA-256 checksum..."
EXPECTED_SHA=$(get_expected_checksum "$TMP_DIR/$SHA_SUMS_NAME" "$ASSET_NAME")
if [ -z "$EXPECTED_SHA" ]; then
    log_error "Could not find expected SHA256 checksum for '$ASSET_NAME' in $SHA_SUMS_NAME."
    exit 1
fi

if ! verify_checksum "$TMP_DIR/$ASSET_NAME" "$EXPECTED_SHA"; then
    ACTUAL_SHA=$(calculate_sha256 "$TMP_DIR/$ASSET_NAME" || echo "unknown")
    log_error "SHA-256 checksum verification failed for $ASSET_NAME!"
    log_error "  Expected: $EXPECTED_SHA"
    log_error "  Actual:   $ACTUAL_SHA"
    exit 1
fi
log_success "Verified SHA-256 checksum ($EXPECTED_SHA)."

# Verify Attestation
if ! verify_attestation "$TMP_DIR/$ASSET_NAME" "$REPO" "$SKIP_ATTESTATION" "$REQUIRE_ATTESTATION" "$HAS_GH"; then
    exit 1
fi

# Install binary atomically
mkdir -p "$INSTALL_DIR"
chmod 755 "$TMP_DIR/$ASSET_NAME"
mv -f "$TMP_DIR/$ASSET_NAME" "$TARGET_BIN"
log_success "Successfully installed pw to $TARGET_BIN"

# Check PATH
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        log_warning "\nNOTE: $INSTALL_DIR is not currently in your PATH."
        log_warning "To use 'pw', add the following line to your shell configuration file (~/.bashrc, ~/.zshrc, or ~/.profile):"
        printf "\n    export PATH=\"%s:\$PATH\"\n\n" "$INSTALL_DIR"
        ;;
esac

# Smoke test installed binary
if [ -x "$TARGET_BIN" ]; then
    "$TARGET_BIN" version || true
fi

log_success "\nPatchwork has been installed successfully!"
log_info "Run 'pw --help' or 'pw check' to get started."

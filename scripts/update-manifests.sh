#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: update-manifests.sh --version <ver> --sha-x64 <sha256> --sha-arm64 <sha256> --tarball-sha <sha256> [options]

Required:
  --version <ver>       Package version (e.g. 0.3.1)
  --sha-x64 <sha256>    64-bit Windows binary SHA-256 hex digest
  --sha-arm64 <sha256>  ARM64 Windows binary SHA-256 hex digest
  --tarball-sha <sha>   Source tarball SHA-256 hex digest

Optional:
  --tag <tag>           Release git tag (default: v<version>)
  --repo <owner/repo>   GitHub repository slug (default: karan-banwasi/Patchwork)
  --root <dir>          Repository root containing Formula/, bucket/, winget/ (default: .)
  -h, --help            Show this help message
EOF
}

VERSION=""
TAG=""
SHA_X64=""
SHA_ARM64=""
TARBALL_SHA=""
REPO="karan-banwasi/Patchwork"
ROOT="."

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || { echo "Error: --version requires a value." >&2; exit 1; }
      VERSION="$2"
      shift 2
      ;;
    --tag)
      [ "$#" -ge 2 ] || { echo "Error: --tag requires a value." >&2; exit 1; }
      TAG="$2"
      shift 2
      ;;
    --sha-x64)
      [ "$#" -ge 2 ] || { echo "Error: --sha-x64 requires a value." >&2; exit 1; }
      SHA_X64="$2"
      shift 2
      ;;
    --sha-arm64)
      [ "$#" -ge 2 ] || { echo "Error: --sha-arm64 requires a value." >&2; exit 1; }
      SHA_ARM64="$2"
      shift 2
      ;;
    --tarball-sha)
      [ "$#" -ge 2 ] || { echo "Error: --tarball-sha requires a value." >&2; exit 1; }
      TARBALL_SHA="$2"
      shift 2
      ;;
    --repo)
      [ "$#" -ge 2 ] || { echo "Error: --repo requires a value." >&2; exit 1; }
      REPO="$2"
      shift 2
      ;;
    --root)
      [ "$#" -ge 2 ] || { echo "Error: --root requires a value." >&2; exit 1; }
      ROOT="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Error: Unknown argument '$1'." >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [ -z "$VERSION" ]; then
  echo "Error: --version is required." >&2
  exit 1
fi
VERSION="${VERSION#v}"
ver_re='^[0-9]+(\.[0-9]+)*([+-][0-9A-Za-z.-]+)?$'
if [[ ! "$VERSION" =~ $ver_re ]]; then
  echo "Error: Invalid --version '$VERSION'." >&2
  exit 1
fi

if [ -z "$TAG" ]; then
  TAG="v${VERSION}"
fi
tag_re='^[A-Za-z0-9._+-]+$'
if [[ ! "$TAG" =~ $tag_re ]]; then
  echo "Error: Invalid --tag '$TAG'." >&2
  exit 1
fi

repo_re='^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$'
if [[ ! "$REPO" =~ $repo_re ]]; then
  echo "Error: Invalid --repo '$REPO' (expected owner/repo)." >&2
  exit 1
fi

validate_sha256() {
  local name="$1"
  local val="$2"
  local sha_re='^[0-9a-fA-F]{64}$'
  if [ -z "$val" ]; then
    echo "Error: $name is required." >&2
    exit 1
  fi
  if [[ ! "$val" =~ $sha_re ]]; then
    echo "Error: Invalid $name '$val' (expected 64 hex characters)." >&2
    exit 1
  fi
}

validate_sha256 "--sha-x64" "$SHA_X64"
validate_sha256 "--sha-arm64" "$SHA_ARM64"
validate_sha256 "--tarball-sha" "$TARBALL_SHA"

if [ -z "$ROOT" ]; then
  echo "Error: --root must not be empty." >&2
  exit 1
fi
if [ "$ROOT" != "/" ]; then
  ROOT="${ROOT%/}"
fi
if [ ! -d "$ROOT" ]; then
  echo "Error: Root directory '$ROOT' does not exist." >&2
  exit 1
fi
if [ "$ROOT" = "/" ]; then
  PREFIX=""
else
  PREFIX="$ROOT"
fi

FORMULA_FILE="${PREFIX}/Formula/pw.rb"
SCOOP_FILE="${PREFIX}/bucket/pw.json"
WINGET_VER_FILE="${PREFIX}/winget/KaranBanwasi.Patchwork.yaml"
WINGET_LOCALE_FILE="${PREFIX}/winget/KaranBanwasi.Patchwork.locale.en-US.yaml"
WINGET_INST_FILE="${PREFIX}/winget/KaranBanwasi.Patchwork.installer.yaml"

for target in \
  "$FORMULA_FILE" \
  "$SCOOP_FILE" \
  "$WINGET_VER_FILE" \
  "$WINGET_LOCALE_FILE" \
  "$WINGET_INST_FILE"; do
  if [ ! -f "$target" ]; then
    echo "Error: Required manifest file '$target' does not exist." >&2
    exit 1
  fi
done

TMP_FILES=()
cleanup() {
  local f
  for f in "${TMP_FILES[@]+"${TMP_FILES[@]}"}"; do
    rm -f "$f"
  done
}
trap cleanup EXIT

make_tmp() {
  local target="$1"
  local tmp
  tmp="$(mktemp "${target}.tmp.XXXXXX")"
  TMP_FILES+=("$tmp")
  printf '%s' "$tmp"
}

commit_tmp() {
  local tmp="$1"
  local target="$2"
  chmod 0644 "$tmp"
  mv -f "$tmp" "$target"
}

TARBALL_URL="https://github.com/${REPO}/archive/refs/tags/${TAG}.tar.gz"
TARBALL_SHA_LOWER="$(printf '%s' "$TARBALL_SHA" | tr '[:upper:]' '[:lower:]')"
URL_X64="https://github.com/${REPO}/releases/download/${TAG}/pw.exe"
URL_ARM64="https://github.com/${REPO}/releases/download/${TAG}/pw-arm64.exe"
SHA_X64_LOWER="$(printf '%s' "$SHA_X64" | tr '[:upper:]' '[:lower:]')"
SHA_ARM64_LOWER="$(printf '%s' "$SHA_ARM64" | tr '[:upper:]' '[:lower:]')"
SHA_X64_UPPER="$(printf '%s' "$SHA_X64" | tr '[:lower:]' '[:upper:]')"
SHA_ARM64_UPPER="$(printf '%s' "$SHA_ARM64" | tr '[:lower:]' '[:upper:]')"
RELEASE_NOTES_URL="https://github.com/${REPO}/releases/tag/${TAG}"

# 1. Update Homebrew Formula (Formula/pw.rb)
formula_tmp="$(make_tmp "$FORMULA_FILE")"
tr -d '\r' < "$FORMULA_FILE" | awk -v url="$TARBALL_URL" -v sha="$TARBALL_SHA_LOWER" '
  {
    if (!found_url && $0 ~ /^[[:space:]]*url[[:space:]]+"/) {
      match($0, /^[[:space:]]*/)
      print substr($0, 1, RLENGTH) "url \"" url "\""
      found_url = 1
      next
    }
    if (!found_sha && $0 ~ /^[[:space:]]*sha256[[:space:]]+"/) {
      match($0, /^[[:space:]]*/)
      print substr($0, 1, RLENGTH) "sha256 \"" sha "\""
      found_sha = 1
      next
    }
    print
  }
  END {
    if (!found_url || !found_sha) {
      exit 2
    }
  }
' > "$formula_tmp" || {
  echo "Error: Failed to update url/sha256 in '$FORMULA_FILE'." >&2
  exit 1
}
commit_tmp "$formula_tmp" "$FORMULA_FILE"

# 2. Update Scoop bucket manifest (bucket/pw.json)
scoop_tmp="$(make_tmp "$SCOOP_FILE")"
if command -v jq >/dev/null 2>&1; then
  jq --indent 4 \
    --arg ver "$VERSION" \
    --arg url64 "$URL_X64" \
    --arg hash64 "$SHA_X64_LOWER" \
    --arg urlarm "$URL_ARM64" \
    --arg hasharm "$SHA_ARM64_LOWER" \
    '.version = $ver | .architecture["64bit"].url = $url64 | .architecture["64bit"].hash = $hash64 | .architecture.arm64.url = $urlarm | .architecture.arm64.hash = $hasharm' \
    < "$SCOOP_FILE" | tr -d '\r' > "$scoop_tmp"
elif command -v node >/dev/null 2>&1 && node -e 'process.exit(0)' >/dev/null 2>&1; then
  node -e '
    const fs = require("fs");
    const [ver, url64, hash64, urlArm, hashArm] = process.argv.slice(1);
    const data = JSON.parse(fs.readFileSync(0, "utf8").replace(/^\uFEFF/, ""));
    if (!data.architecture || !data.architecture["64bit"] || !data.architecture.arm64) {
      throw new Error("missing architecture keys in Scoop manifest");
    }
    data.version = ver;
    data.architecture["64bit"].url = url64;
    data.architecture["64bit"].hash = hash64;
    data.architecture.arm64.url = urlArm;
    data.architecture.arm64.hash = hashArm;
    process.stdout.write(JSON.stringify(data, null, 4) + "\n");
  ' "$VERSION" "$URL_X64" "$SHA_X64_LOWER" "$URL_ARM64" "$SHA_ARM64_LOWER" < "$SCOOP_FILE" | tr -d '\r' > "$scoop_tmp"
elif command -v python3 >/dev/null 2>&1 && python3 -c 'import json' >/dev/null 2>&1; then
  python3 -c '
import json, sys
ver, url64, hash64, url_arm, hash_arm = sys.argv[1:6]
data = json.load(sys.stdin)
data["version"] = ver
data["architecture"]["64bit"]["url"] = url64
data["architecture"]["64bit"]["hash"] = hash64
data["architecture"]["arm64"]["url"] = url_arm
data["architecture"]["arm64"]["hash"] = hash_arm
sys.stdout.buffer.write((json.dumps(data, indent=4) + "\n").encode("utf-8"))
' "$VERSION" "$URL_X64" "$SHA_X64_LOWER" "$URL_ARM64" "$SHA_ARM64_LOWER" < "$SCOOP_FILE" | tr -d '\r' > "$scoop_tmp"
else
  echo "Error: A working JSON processor ('jq', 'node', or 'python3') is required to update '$SCOOP_FILE'." >&2
  exit 1
fi
commit_tmp "$scoop_tmp" "$SCOOP_FILE"

# 3. Update Winget version manifest (winget/KaranBanwasi.Patchwork.yaml)
winget_ver_tmp="$(make_tmp "$WINGET_VER_FILE")"
tr -d '\r' < "$WINGET_VER_FILE" | awk -v ver="$VERSION" '
  {
    if (!found_ver && $0 ~ /^PackageVersion:[[:space:]]*/) {
      print "PackageVersion: " ver
      found_ver = 1
      next
    }
    print
  }
  END {
    if (!found_ver) {
      exit 2
    }
  }
' > "$winget_ver_tmp" || {
  echo "Error: Failed to update PackageVersion in '$WINGET_VER_FILE'." >&2
  exit 1
}
commit_tmp "$winget_ver_tmp" "$WINGET_VER_FILE"

# 4. Update Winget defaultLocale manifest (winget/KaranBanwasi.Patchwork.locale.en-US.yaml)
winget_locale_tmp="$(make_tmp "$WINGET_LOCALE_FILE")"
tr -d '\r' < "$WINGET_LOCALE_FILE" | awk -v ver="$VERSION" -v notes="$RELEASE_NOTES_URL" '
  {
    if (!found_ver && $0 ~ /^PackageVersion:[[:space:]]*/) {
      print "PackageVersion: " ver
      found_ver = 1
      next
    }
    if (!found_notes && $0 ~ /^ReleaseNotesUrl:[[:space:]]*/) {
      print "ReleaseNotesUrl: " notes
      found_notes = 1
      next
    }
    print
  }
  END {
    if (!found_ver || !found_notes) {
      exit 2
    }
  }
' > "$winget_locale_tmp" || {
  echo "Error: Failed to update PackageVersion/ReleaseNotesUrl in '$WINGET_LOCALE_FILE'." >&2
  exit 1
}
commit_tmp "$winget_locale_tmp" "$WINGET_LOCALE_FILE"

# 5. Update Winget installer manifest (winget/KaranBanwasi.Patchwork.installer.yaml)
winget_inst_tmp="$(make_tmp "$WINGET_INST_FILE")"
tr -d '\r' < "$WINGET_INST_FILE" | awk \
  -v ver="$VERSION" \
  -v url_x64="$URL_X64" \
  -v sha_x64="$SHA_X64_UPPER" \
  -v url_arm64="$URL_ARM64" \
  -v sha_arm64="$SHA_ARM64_UPPER" '
  {
    if (!found_ver && $0 ~ /^PackageVersion:[[:space:]]*/) {
      print "PackageVersion: " ver
      found_ver = 1
      next
    }
    if ($0 ~ /^[[:space:]]*-[[:space:]]*Architecture:[[:space:]]*/) {
      if ($0 ~ /Architecture:[[:space:]]*x64([[:space:]]|$)/) {
        arch = "x64"
      } else if ($0 ~ /Architecture:[[:space:]]*arm64([[:space:]]|$)/) {
        arch = "arm64"
      } else {
        arch = ""
      }
      print
      next
    }
    if ($0 ~ /^[[:space:]]*InstallerUrl:[[:space:]]*/) {
      match($0, /^[[:space:]]*/)
      indent = substr($0, 1, RLENGTH)
      if (arch == "x64" && !found_x64_url) {
        print indent "InstallerUrl: " url_x64
        found_x64_url = 1
        next
      }
      if (arch == "arm64" && !found_arm64_url) {
        print indent "InstallerUrl: " url_arm64
        found_arm64_url = 1
        next
      }
    }
    if ($0 ~ /^[[:space:]]*InstallerSha256:[[:space:]]*/) {
      match($0, /^[[:space:]]*/)
      indent = substr($0, 1, RLENGTH)
      if (arch == "x64" && !found_x64_sha) {
        print indent "InstallerSha256: " sha_x64
        found_x64_sha = 1
        next
      }
      if (arch == "arm64" && !found_arm64_sha) {
        print indent "InstallerSha256: " sha_arm64
        found_arm64_sha = 1
        next
      }
    }
    print
  }
  END {
    if (!found_ver || !found_x64_url || !found_x64_sha || !found_arm64_url || !found_arm64_sha) {
      exit 2
    }
  }
' > "$winget_inst_tmp" || {
  echo "Error: Failed to update installer fields in '$WINGET_INST_FILE'." >&2
  exit 1
}
commit_tmp "$winget_inst_tmp" "$WINGET_INST_FILE"

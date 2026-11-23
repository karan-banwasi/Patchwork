#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "Usage: $0 <windows-binary> [<windows-binary>...]" >&2
  exit 1
fi

for target in "$@"; do
  if [ ! -f "$target" ]; then
    echo "Error: Target binary '$target' does not exist." >&2
    exit 1
  fi
done

AZURE_TENANT_ID="${AZURE_TENANT_ID:-}"
AZURE_CLIENT_ID="${AZURE_CLIENT_ID:-}"
AZURE_CLIENT_SECRET="${AZURE_CLIENT_SECRET:-}"
AZURE_ACCESS_TOKEN="${AZURE_ACCESS_TOKEN:-}"
AZURE_TRUSTED_SIGNING_ENDPOINT="${AZURE_TRUSTED_SIGNING_ENDPOINT:-https://eus.codesigning.azure.net/}"
AZURE_TRUSTED_SIGNING_ACCOUNT="${AZURE_TRUSTED_SIGNING_ACCOUNT:-}"
AZURE_TRUSTED_SIGNING_PROFILE="${AZURE_TRUSTED_SIGNING_PROFILE:-}"
REQUIRE_WINDOWS_SIGNING="${REQUIRE_WINDOWS_SIGNING:-false}"
JSIGN_VERSION="${JSIGN_VERSION:-7.6}"
JSIGN_SHA256="${JSIGN_SHA256:-6cf58aa080e7ff82656f320cff8d4a3bff30408df8766806dcba3740dd839fa5}"
JSIGN_CMD="${JSIGN_CMD:-}"

is_truthy() {
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
    true|1|yes) return 0 ;;
    *) return 1 ;;
  esac
}

if [ -z "$AZURE_CLIENT_ID" ] && [ -z "$AZURE_ACCESS_TOKEN" ]; then
  if is_truthy "$REQUIRE_WINDOWS_SIGNING"; then
    echo "Error: Windows Authenticode signing is required (REQUIRE_WINDOWS_SIGNING=$REQUIRE_WINDOWS_SIGNING), but AZURE_CLIENT_ID is not set." >&2
    exit 1
  fi
  echo "::notice::Azure Trusted Signing not configured (AZURE_CLIENT_ID not set); skipping Windows Authenticode signing."
  exit 0
fi

missing_vars=""
if [ -z "$AZURE_TRUSTED_SIGNING_ACCOUNT" ]; then
  missing_vars="${missing_vars} AZURE_TRUSTED_SIGNING_ACCOUNT"
fi
if [ -z "$AZURE_TRUSTED_SIGNING_PROFILE" ]; then
  missing_vars="${missing_vars} AZURE_TRUSTED_SIGNING_PROFILE"
fi
if [ -z "$AZURE_ACCESS_TOKEN" ] && [ -z "$AZURE_TENANT_ID" ]; then
  missing_vars="${missing_vars} AZURE_TENANT_ID"
fi
if [ -n "$missing_vars" ]; then
  echo "Error: Incomplete Azure Trusted Signing configuration; missing:${missing_vars}." >&2
  exit 1
fi

ENDPOINT_HOST="${AZURE_TRUSTED_SIGNING_ENDPOINT#https://}"
ENDPOINT_HOST="${ENDPOINT_HOST#http://}"
ENDPOINT_HOST="${ENDPOINT_HOST%%/*}"
if [ -z "$ENDPOINT_HOST" ]; then
  echo "Error: Invalid AZURE_TRUSTED_SIGNING_ENDPOINT '$AZURE_TRUSTED_SIGNING_ENDPOINT'." >&2
  exit 1
fi

json_get() {
  local key="$1"
  local payload="$2"
  if command -v jq >/dev/null 2>&1; then
    printf '%s' "$payload" | jq -r --arg k "$key" '.[$k] // empty'
    return
  fi
  if command -v python3 >/dev/null 2>&1; then
    printf '%s' "$payload" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get(sys.argv[1], '') or '')" "$key"
    return
  fi
  echo "Error: Either 'jq' or 'python3' is required to parse OAuth/OIDC JSON responses." >&2
  exit 1
}

verify_sha256() {
  local file_path="$1"
  local expected="$2"
  local actual=""
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$file_path" | awk '{print tolower($1)}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$file_path" | awk '{print tolower($1)}')"
  else
    echo "Error: Neither 'sha256sum' nor 'shasum' is available to verify jsign download." >&2
    exit 1
  fi
  local expected_lower
  expected_lower="$(printf '%s' "$expected" | tr '[:upper:]' '[:lower:]')"
  if [ "$actual" != "$expected_lower" ]; then
    echo "Error: SHA-256 mismatch for '$file_path' (expected $expected_lower, got $actual)." >&2
    exit 1
  fi
}

ACCESS_TOKEN="$AZURE_ACCESS_TOKEN"
if [ -z "$ACCESS_TOKEN" ]; then
  TOKEN_URL="https://login.microsoftonline.com/${AZURE_TENANT_ID}/oauth2/v2.0/token"
  if [ -n "$AZURE_CLIENT_SECRET" ]; then
    TOKEN_RESPONSE="$(curl -fsSL --show-error -X POST "$TOKEN_URL" \
      --data-urlencode "grant_type=client_credentials" \
      --data-urlencode "client_id=${AZURE_CLIENT_ID}" \
      --data-urlencode "client_secret=${AZURE_CLIENT_SECRET}" \
      --data-urlencode "scope=https://codesigning.azure.net/.default" 2>&1)" || {
      echo "Error: Failed to obtain Azure Entra ID token via client secret." >&2
      exit 1
    }
  elif [ -n "${ACTIONS_ID_TOKEN_REQUEST_URL:-}" ] && [ -n "${ACTIONS_ID_TOKEN_REQUEST_TOKEN:-}" ]; then
    case "$ACTIONS_ID_TOKEN_REQUEST_URL" in
      *\?*) OIDC_URL="${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=api://AzureADTokenExchange" ;;
      *)    OIDC_URL="${ACTIONS_ID_TOKEN_REQUEST_URL}?audience=api://AzureADTokenExchange" ;;
    esac
    OIDC_RESPONSE="$(curl -fsSL --show-error \
      -H "Authorization: bearer ${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" \
      "$OIDC_URL" 2>&1)" || {
      echo "Error: Failed to fetch GitHub Actions OIDC token (ensure workflow job has 'id-token: write')." >&2
      exit 1
    }
    OIDC_TOKEN="$(json_get "value" "$OIDC_RESPONSE")"
    if [ -z "$OIDC_TOKEN" ]; then
      echo "Error: GitHub OIDC response did not include a token value." >&2
      exit 1
    fi
    TOKEN_RESPONSE="$(curl -fsSL --show-error -X POST "$TOKEN_URL" \
      --data-urlencode "grant_type=client_credentials" \
      --data-urlencode "client_id=${AZURE_CLIENT_ID}" \
      --data-urlencode "client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer" \
      --data-urlencode "client_assertion=${OIDC_TOKEN}" \
      --data-urlencode "scope=https://codesigning.azure.net/.default" 2>&1)" || {
      echo "Error: Failed to exchange GitHub OIDC JWT for Azure Trusted Signing token." >&2
      exit 1
    }
  else
    echo "Error: Authentication credentials missing. Provide GitHub OIDC (id-token: write), AZURE_CLIENT_SECRET, or AZURE_ACCESS_TOKEN." >&2
    exit 1
  fi

  ACCESS_TOKEN="$(json_get "access_token" "$TOKEN_RESPONSE")"
  if [ -z "$ACCESS_TOKEN" ]; then
    ERR_DESC="$(json_get "error_description" "$TOKEN_RESPONSE")"
    echo "Error: Azure Entra ID did not return an access_token${ERR_DESC:+: $ERR_DESC}." >&2
    exit 1
  fi
fi

set -- \
  --storetype TRUSTEDSIGNING \
  --keystore "$ENDPOINT_HOST" \
  --storepass "$ACCESS_TOKEN" \
  --alias "${AZURE_TRUSTED_SIGNING_ACCOUNT}/${AZURE_TRUSTED_SIGNING_PROFILE}" \
  --alg SHA-256 \
  --tsaurl "http://timestamp.acs.microsoft.com" \
  --tsmode RFC3161 \
  "$@"

if [ -n "$JSIGN_CMD" ]; then
  echo "Signing Windows binaries using custom JSIGN_CMD (account: ${AZURE_TRUSTED_SIGNING_ACCOUNT}, profile: ${AZURE_TRUSTED_SIGNING_PROFILE})..."
  "$JSIGN_CMD" "$@"
  exit 0
fi

if ! command -v java >/dev/null 2>&1; then
  echo "Error: 'java' runtime is required to execute jsign on this runner." >&2
  exit 1
fi

JSIGN_CACHE_DIR="${RUNNER_TEMP:-/tmp}"
mkdir -p "$JSIGN_CACHE_DIR"
JSIGN_JAR="${JSIGN_CACHE_DIR}/jsign-${JSIGN_VERSION}.jar"
JSIGN_URL="https://github.com/ebourg/jsign/releases/download/${JSIGN_VERSION}/jsign-${JSIGN_VERSION}.jar"

if [ ! -f "$JSIGN_JAR" ]; then
  echo "Downloading jsign ${JSIGN_VERSION}..."
  curl -fsSL "$JSIGN_URL" -o "${JSIGN_JAR}.tmp"
  verify_sha256 "${JSIGN_JAR}.tmp" "$JSIGN_SHA256"
  mv -f "${JSIGN_JAR}.tmp" "$JSIGN_JAR"
else
  verify_sha256 "$JSIGN_JAR" "$JSIGN_SHA256"
fi

echo "Authenticode-signing Windows binaries via Azure Trusted Signing (${ENDPOINT_HOST}, ${AZURE_TRUSTED_SIGNING_ACCOUNT}/${AZURE_TRUSTED_SIGNING_PROFILE})..."
java -jar "$JSIGN_JAR" "$@"

#!/usr/bin/env bash
# Prompts for release secrets and stores them as GitHub secrets with gh. Every
# value travels through stdin, never the argument list, so a secret never
# appears in ps output or shell history. Nothing is written to disk.
set -euo pipefail

if ! command -v gh >/dev/null 2>&1; then
  echo "gh is not installed: brew install gh" >&2
  exit 1
fi

if ! gh auth status >/dev/null 2>&1; then
  echo "gh is not authenticated: run gh auth login first" >&2
  exit 1
fi

prompt_secret() {
  local name="$1" label="$2" value
  printf '%s: ' "$label"
  IFS= read -rs value
  printf '\n'
  if [ -z "$value" ]; then
    echo "skipping $name (empty)"
    return 0
  fi
  printf '%s' "$value" | gh secret set "$name"
  echo "set $name"
  unset value
}

prompt_secret NPM_TOKEN "npm access token (empty to skip)"
prompt_secret SPARKLE_PRIVATE_ED_KEY "Sparkle EdDSA private key (empty to skip)"
prompt_secret APPLE_SIGN_P12_BASE64 "Developer ID .p12 as base64 (empty to skip)"
prompt_secret APPLE_SIGN_P12_PASSWORD ".p12 password (empty to skip)"
prompt_secret APPLE_ID "Apple ID email (empty to skip)"
prompt_secret APPLE_TEAM_ID "Apple Team ID (empty to skip)"
prompt_secret APPLE_APP_SPECIFIC_PASSWORD "app-specific password (empty to skip)"

echo "done; verify with: gh secret list"

#!/usr/bin/env bash
# Prompts for release secrets and stores them as GitHub secrets with gh. Every
# value travels through stdin, never the argument list, so a secret never
# appears in ps output or shell history. Keys that exist locally (Sparkle
# keypair, Developer ID .p12, Team ID) can be generated at runtime: the script
# asks before reading them, and generated files stay under /Volumes/WS/Secret.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=sparkle-keys.sh
. "$root/scripts/sparkle-keys.sh"

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

ask_yes() {
  local answer
  printf '%s [y/N]: ' "$1"
  IFS= read -rs answer
  printf '\n'
  [ "$answer" = "y" ] || [ "$answer" = "Y" ]
}

secret_dir="/Volumes/WS/Secret/macos"

generate_sparkle() {
  local public private
  public="$(sparkle_public_key)"
  private="$(sparkle_private_key)"
  printf '%s' "$public" | gh secret set SPARKLE_PUBLIC_ED_KEY
  echo "set SPARKLE_PUBLIC_ED_KEY"
  printf '%s' "$private" | gh secret set SPARKLE_PRIVATE_ED_KEY
  echo "set SPARKLE_PRIVATE_ED_KEY"
  mkdir -p "$secret_dir" && chmod 700 "$secret_dir"
  printf '%s' "$public" > "$secret_dir/sparkle-public.key"
  printf '%s' "$private" > "$secret_dir/sparkle-private.key"
  chmod 600 "$secret_dir/sparkle-private.key"
  echo "wrote $secret_dir/sparkle-public.key and sparkle-private.key"
}

# export_p12 exports the Developer ID identity as .p12, base64-encodes it for
# the CI secret, and derives the Team ID from the certificate itself.
export_p12() {
  local password p12 b64 team
  password="$(openssl rand -base64 24)"
  p12="$secret_dir/relo-signing.p12"
  b64="$secret_dir/relo-signing.p12.b64"
  mkdir -p "$secret_dir" && chmod 700 "$secret_dir"
  security export -k "${KEYCHAIN:-$HOME/Library/Keychains/login.keychain-db}" \
    -t identities -f pkcs12 -o "$p12" -P "$password"
  chmod 600 "$p12"
  base64 -i "$p12" -o "$b64"
  chmod 600 "$b64"
  printf '%s' "$password" > "$secret_dir/p12-password.txt"
  chmod 600 "$secret_dir/p12-password.txt"
  gh secret set APPLE_SIGN_P12_BASE64 < "$b64"
  echo "set APPLE_SIGN_P12_BASE64"
  printf '%s' "$password" | gh secret set APPLE_SIGN_P12_PASSWORD
  echo "set APPLE_SIGN_P12_PASSWORD"
  team="$(security find-certificate -c "Developer ID Application" -p 2>/dev/null \
    | openssl x509 -noout -subject -nameopt multiline 2>/dev/null \
    | awk -F'= ' '/organizationalUnitName/ {print $2; exit}')"
  if [ -n "$team" ]; then
    printf '%s' "$team" | gh secret set APPLE_TEAM_ID
    echo "set APPLE_TEAM_ID ($team)"
    printf '%s' "$team" > "$secret_dir/apple-team-id.txt"
  else
    echo "could not read the Team ID; set APPLE_TEAM_ID by hand" >&2
  fi
}

if ask_yes "generate the Sparkle keypair at runtime and set it as secrets?"; then
  generate_sparkle
else
  prompt_secret SPARKLE_PRIVATE_ED_KEY "Sparkle EdDSA private key (empty to skip)"
fi

if ask_yes "export the Developer ID .p12 at runtime and set it as secrets?"; then
  export_p12
else
  prompt_secret APPLE_SIGN_P12_BASE64 "Developer ID .p12 as base64 (empty to skip)"
  prompt_secret APPLE_SIGN_P12_PASSWORD ".p12 password (empty to skip)"
  prompt_secret APPLE_TEAM_ID "Apple Team ID (empty to skip)"
fi

prompt_secret NPM_TOKEN "npm access token (empty to skip)"
prompt_secret APPLE_ID "Apple ID email (empty to skip)"
prompt_secret APPLE_APP_SPECIFIC_PASSWORD "app-specific password (empty to skip)"

echo "done; verify with: gh secret list"

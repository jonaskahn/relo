#!/usr/bin/env bash
# Stores release secrets with gh. Every value travels through stdin, never the
# argument list, so a secret never appears in ps output or shell history. Names
# are auto-detected from .github/workflows (secrets.X / vars.X, minus automatic
# tokens like GITHUB_TOKEN); every name is always prompted, empty input skips
# (so existing values are kept unless overwritten). Pass NAMEs to override detection ("FOO" = secret,
# "VAR:FOO" / "vars.FOO" = variable); --list prints the names without asking.
# Keys that exist locally (Sparkle keypair, Developer ID .p12, Team ID) can be generated at runtime; the script
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

list_only=0
only_names=""
for arg in "$@"; do
  case "$arg" in
    --all) ;; # accepted for compatibility; every name is always prompted now
    --list) list_only=1 ;;
    -h|--help)
      echo "usage: update-secrets.sh [--all] [--list] [NAME...]" >&2
      echo "  no NAMEs: auto-detect secrets.X / vars.X from .github/workflows" >&2
      echo "  NAMEs: only prompt for those names (VAR:NAME forces a variable)" >&2
      exit 0 ;;
    --*) echo "usage: update-secrets.sh [--all] [--list] [NAME...]" >&2; exit 2 ;;
    *) only_names="$only_names $arg" ;;
  esac
done
# NAMEs override auto-detection: "FOO" = secret, "VAR:FOO" = variable.
only_names="$(printf '%s' "$only_names" | xargs 2>/dev/null || printf '%s' "$only_names")"

# Auto-detect every secrets.NAME / vars.NAME referenced by workflows, minus
# automatic tokens (e.g. GITHUB_TOKEN) that never need `gh secret set`.
detect_names() {
  grep -rhoE '(secrets|vars)\.[A-Za-z0-9_]+' "$root/.github/workflows" 2>/dev/null \
    | sort -u | grep -v 'GITHUB_TOKEN' || true
}

if [ -n "$only_names" ]; then
  detected="$only_names"
else
  detected="$(detect_names)"
fi

if [ "$list_only" -eq 1 ]; then
  printf '%s\n' $detected
  exit 0
fi

existing="$(gh secret list 2>/dev/null | awk '{print $1}')"
existing_vars="$(gh variable list 2>/dev/null | awk '{print $1}')"

# already_set prints " (already set)" when the name exists, else "".
already_set() {
  printf '%s\n' "$existing" | grep -qxF "$1" && printf ' (already set)' || true
}

already_set_var() {
  printf '%s\n' "$existing_vars" | grep -qxF "$1" && printf ' (already set)' || true
}

# read_tty reads one line into the named variable from the terminal, so prompts
# still work when stdin is piped. Falls back to stdin with no tty attached.
read_tty() {
  if [ -r /dev/tty ]; then
    IFS= read -rs "$1" < /dev/tty
  else
    IFS= read -rs "$1"
  fi
}

prompt_secret() {
  local name="$1" label="$2" value
  printf '%s: ' "$label" > /dev/tty 2>/dev/null || printf '%s: ' "$label"
  read_tty value
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
  printf '%s [y/N]: ' "$1" > /dev/tty 2>/dev/null || printf '%s [y/N]: ' "$1"
  read_tty answer
  printf '\n'
  [ "$answer" = "y" ] || [ "$answer" = "Y" ]
}

prompt_var() {
  local name="$1" label="$2" value
  printf '%s: ' "$label" > /dev/tty 2>/dev/null || printf '%s: ' "$label"
  read_tty value
  printf '\n'
  if [ -z "$value" ]; then
    echo "skipping $name (empty)"
    return 0
  fi
  printf '%s' "$value" | gh variable set "$name"
  echo "set $name variable"
  unset value
}

secret_dir="/Volumes/WS/Secret/macos"

generate_sparkle() {
  local public private
  public="$(sparkle_public_key)"
  private="$(sparkle_private_key)"
  printf '%s' "$public" | gh variable set SPARKLE_PUBLIC_ED_KEY
  echo "set SPARKLE_PUBLIC_ED_KEY variable"
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

if [ -z "$only_names" ]; then
if ask_yes "generate the Sparkle keypair at runtime and set it as secrets?"; then
  generate_sparkle
else
  prompt_secret SPARKLE_PRIVATE_ED_KEY "Sparkle EdDSA private key$(already_set SPARKLE_PRIVATE_ED_KEY) (empty to skip)"
  prompt_var SPARKLE_PUBLIC_ED_KEY "Sparkle EdDSA public key$(already_set_var SPARKLE_PUBLIC_ED_KEY) (empty to skip)"
fi

if ask_yes "export the Developer ID .p12 at runtime and set it as secrets?"; then
  export_p12
else
  prompt_secret APPLE_SIGN_P12_BASE64 "Developer ID .p12 as base64$(already_set APPLE_SIGN_P12_BASE64) (empty to skip)"
  prompt_secret APPLE_SIGN_P12_PASSWORD ".p12 password$(already_set APPLE_SIGN_P12_PASSWORD) (empty to skip)"
  prompt_secret APPLE_TEAM_ID "Apple Team ID$(already_set APPLE_TEAM_ID) (empty to skip)"
fi
fi

prompt_detected() {
  local entry kind name label
  for entry in $detected; do
    case "$entry" in
      secrets.*) kind=secret; name="${entry#secrets.}" ;;
      vars.*) kind=var; name="${entry#vars.}" ;;
      VAR:*) kind=var; name="${entry#VAR:}" ;;
      *) kind=secret; name="$entry" ;;
    esac
    # Sparkle / Apple signing have runtime generators above; the generic
    # loop only handles the rest so each name is asked for exactly once.
    case "$name" in
      SPARKLE_PRIVATE_ED_KEY|SPARKLE_PUBLIC_ED_KEY|APPLE_SIGN_P12_BASE64|APPLE_SIGN_P12_PASSWORD|APPLE_TEAM_ID) continue ;;
    esac
    label="$name (empty to skip)"
    case "$name" in
      HOMEBREW_TAP_TOKEN) label="Homebrew tap token, PAT with contents:write on homebrew-apps (empty to skip)" ;;
      APPLE_APP_SPECIFIC_PASSWORD) label="app-specific password (empty to skip)" ;;
      APPLE_ID) label="Apple ID email (empty to skip)" ;;
      NPM_TOKEN) label="npm access token (empty to skip)" ;;
    esac
    if [ "$kind" = "var" ]; then
      prompt_var "$name" "$label$(already_set_var "$name")"
    else
      prompt_secret "$name" "$label$(already_set "$name")"
    fi
  done
}

prompt_detected

echo "done; verify with: gh secret list"

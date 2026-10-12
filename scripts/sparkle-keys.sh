#!/usr/bin/env bash
# Sparkle EdDSA key helpers, sourced by package-macos.sh and update-secrets.sh.
#
# The keypair lives in the login keychain: generate_keys reuses the existing
# key, so ensuring is idempotent and never rotates a published key. Helpers
# print only the requested value on stdout so callers can capture it.
set -euo pipefail

sparkle_version="2.9.4"
sparkle_sha256="ce89daf967db1e1893ed3ebd67575ed82d3902563e3191ca92aaec9164fbdef9"

# ensure_sparkle downloads the pinned Sparkle tools when missing and prints
# the directory holding Sparkle.framework and bin/. Outside dist/ on purpose:
# `make package-clean` wipes dist, and a fresh 15 MB download on every run is
# what made packaging look like it hung.
ensure_sparkle() {
  local cache scratch archive found
  cache="${RELO_CACHE:-$HOME/Library/Caches/relo}/sparkle-$sparkle_version"
  if [ -d "$cache/Sparkle.framework" ]; then
    printf '%s' "$cache"
    return 0
  fi
  scratch="$(mktemp -d)"
  trap 'rm -rf "$scratch"' RETURN
  # --retry covers a stalled connection; --progress-bar keeps the 15 MB
  # transfer visible even when make captures stderr, so it never looks like
  # a hung build.
  archive="$scratch/sparkle-$sparkle_version.tar.xz"
  curl -fSL --retry 3 --progress-bar -o "$archive" \
    "https://github.com/sparkle-project/Sparkle/releases/download/${sparkle_version}/Sparkle-${sparkle_version}.tar.xz"
  echo "${sparkle_sha256}  $archive" | shasum -a 256 -c -
  mkdir -p "$cache"
  tar -xf "$archive" -C "$cache"
  if [ ! -d "$cache/Sparkle.framework" ]; then
    found="$(find "$cache" -maxdepth 3 -name Sparkle.framework -type d | head -n 1)"
    if [ -n "$found" ]; then
      ln -s "$(cd "$(dirname "$found")" && pwd)/$(basename "$found")" "$cache/Sparkle.framework"
    fi
  fi
  printf '%s' "$cache"
}

# sparkle_public_key ensures the keypair exists and prints the public key.
sparkle_public_key() {
  local key
  key="$("$(ensure_sparkle)/bin/generate_keys" 2>/dev/null | grep -o '<string>[^<]*</string>' | sed 's/<[^>]*>//g')"
  if [ -z "$key" ]; then
    echo "could not read the Sparkle public key from generate_keys" >&2
    return 1
  fi
  printf '%s' "$key"
}

# sparkle_private_key prints the private key stored in the login keychain.
sparkle_private_key() {
  security find-generic-password -s "https://sparkle-project.org" -a "ed25519" -w
}

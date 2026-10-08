#!/usr/bin/env bash
# Builds the Windows Relo-Setup installer for every architecture: one
# relo.exe with the daemon and the tray, beside the relo.cmd shim the command
# line runs. The same binary is a release asset that build-all writes and npm
# installs.
#
# ARCH names one architecture ("amd64 arm64" builds both). VERSION is the
# release version without a leading v.
#
# Requires makensis on PATH (brew install makensis, or the NSIS package on
# Linux).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
arches="${ARCH:-amd64 arm64}"
version="${VERSION:-0.1.0}"
version="${version#v}"

if ! command -v makensis >/dev/null 2>&1; then
  echo "makensis is not installed: brew install makensis (or apt install nsis)" >&2
  exit 1
fi

# The binary is a build input, not a release artifact, so it lives outside
# dist/ and never outlasts this run. One build ships: relo.exe is a
# GUI-subsystem binary, so a shortcut or a login start never flashes a
# console, and the relo.cmd shim beside it is what a terminal runs.
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
mkdir -p "$root/dist"

for arch in $arches; do
  binary="$scratch/relo-windows-$arch.exe"
  echo "building the $arch relo.exe"
  (cd "$root/daemon" && GOOS=windows GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -H windowsgui -X github.com/jonaskahn/relo/internal/cli.version=$version" \
    -o "$binary" ./cmd/relo)

  outfile="$root/dist/Relo-Setup-$version-$arch.exe"
  echo "building $outfile"
  makensis -V2 \
    -DVERSION="$version" \
    -DBINARY="$binary" \
    -DSHIMCMD="$root/packaging/windows/relo.cmd" \
    -DSHIMSH="$root/packaging/windows/relo" \
    -DICON="$root/packaging/windows/icon.ico" \
    -DOUTFILE="$outfile" \
    "$root/packaging/windows/installer.nsi"
  ls -la "$outfile"
done

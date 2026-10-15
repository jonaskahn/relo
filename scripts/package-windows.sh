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

# One build ships: relo.exe is a GUI-subsystem binary, so a shortcut or a
# login start never flashes a console, and the relo.cmd shim beside it is
# what a terminal runs. A binary already built for this exact version is
# reused from dist/.pkgcache; the version in the path means a version change
# always misses. Wipe dist/ (make package-clean) to force a rebuild.
cache="$root/dist/.pkgcache"
mkdir -p "$cache"

# makensis is a native Windows program and MSYS bash leaves -DKEY=value
# alone, so a Unix-style /tmp or /d path never resolves there. Convert every
# path makensis reads or writes to a Windows path first.
native_path() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -m "$1"
  else
    printf '%s' "$1"
  fi
}

for arch in $arches; do
  binary="$cache/relo-windows-$arch-$version.exe"
  if [ -x "$binary" ]; then
    echo "reusing the $arch relo.exe for $version"
  else
    echo "building the $arch relo.exe"
    (cd "$root/daemon" && GOOS=windows GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -H windowsgui -X github.com/jonaskahn/relo/internal/cli.version=$version" \
      -o "$binary" ./cmd/relo)
  fi

  ls -la "$binary"
  outfile="$root/dist/Relo-Setup-$version-$arch.exe"
  echo "building $outfile"
  makensis -V2 \
    -DVERSION="$version" \
    -DBINARY="$(native_path "$binary")" \
    -DSHIMCMD="$(native_path "$root/packaging/windows/relo.cmd")" \
    -DSHIMSH="$(native_path "$root/packaging/windows/relo")" \
    -DICON="$(native_path "$root/packaging/windows/icon.ico")" \
    -DOUTFILE="$(native_path "$outfile")" \
    "$(native_path "$root/packaging/windows/installer.nsi")"
  ls -la "$outfile"
done

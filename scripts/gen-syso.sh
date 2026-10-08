#!/usr/bin/env bash
# Generates the Windows resource object that puts the Relo icon on every
# relo.exe. The file name carries windows_<arch>, so go build links it into
# Windows builds of that architecture and skips it everywhere else. The
# output directory is an argument so the drift check can generate into a
# staging tree instead of a developer's working copy.
#
# The generator is pinned: a new rsrc can reshape the .rsrc section, and the
# artwork drift check has to compare like with like.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
rsrc_version="v0.10.2"
ico="$root/packaging/windows/icon.ico"
out_dir="${1:-$root/daemon/cmd/relo}"

mkdir -p "$out_dir"
for arch in amd64 arm64; do
  out="$out_dir/relo_windows_$arch.syso"
  echo "generating ${out#"$root"/}"
  go run "github.com/akavel/rsrc@$rsrc_version" \
    -ico "$ico" \
    -arch "$arch" \
    -o "$out"
done

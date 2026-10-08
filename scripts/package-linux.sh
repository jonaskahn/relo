#!/usr/bin/env bash
# Builds the Relo-Setup .deb and .rpm packages for every architecture: the
# daemon plus the tray, installed as /usr/bin/relo with its launcher and
# icons. The same binary is a release asset that build-all writes and npm
# installs.
#
# ARCH names one architecture ("amd64 arm64" builds both). VERSION is the
# release version without a leading v.
#
# Requires nfpm on PATH:
#   go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
arches="${ARCH:-amd64 arm64}"
version="${VERSION:-0.1.0}"
version="${version#v}"
case "$version" in
  [0-9]*) ;;
  *) version="0.0.0-$version" ;;
esac

if ! command -v nfpm >/dev/null 2>&1; then
  # `go install` writes to GOPATH/bin, which is not always on PATH.
  gopath_bin="$(go env GOPATH)/bin"
  if [ -x "$gopath_bin/nfpm" ]; then
    PATH="$gopath_bin:$PATH"
    export PATH
  else
    echo "nfpm is not installed: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest" >&2
    exit 1
  fi
fi

mkdir -p "$root/dist"

# The binary the package ships is a build input, not a release artifact, so
# it lives outside dist/ and never outlasts this run.
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

cd "$root"

for arch in $arches; do
  binary="$scratch/relo-linux-$arch"
  echo "building the $arch binary"
  (cd "$root/daemon" && GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/jonaskahn/relo/internal/cli.version=$version" \
    -o "$binary" ./cmd/relo)

  for format in deb rpm; do
    # The target is an exact path, so the artifact carries the product name
    # and architecture instead of nfpm's own naming.
    ARCH="$arch" VERSION="$version" BINARY="$binary" \
      nfpm package --config packaging/linux/nfpm.yaml --packager "$format" \
      --target "dist/Relo-Setup-$version-$arch.$format"
  done
done
ls -la "$root/dist/Relo-Setup-"*.{deb,rpm}

#!/usr/bin/env bash
# Builds the binaries the installers ship, without building any installer.
# Outputs land in dist/.pkgcache, where package-*.sh reuses them when the
# version matches, so a later `make package-*` skips its Go builds.
#
# OS and ARCH narrow the matrix ("darwin amd64" builds both arches of one OS);
# by default every OS and arch is built. VERSION is the release version,
# optionally with a leading v.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
oses="${OS:-linux darwin windows}"
arches="${ARCH:-amd64 arm64}"
version="${VERSION:-0.1.1}"
version="${version#v}"

cache="$root/dist/.pkgcache"
mkdir -p "$cache"

# The internal linker's -macos flag exists only on newer Go; older linkers
# lack the flag and already target the floor (same probe as package-macos.sh).
macos_min="${MACOS_MIN_VERSION:-12.0}"
macos_ldflag=""
if go tool link -h 2>&1 | grep -q -- "-macos"; then
  macos_ldflag=" -macos=$macos_min"
fi

for os in $oses; do
  # package-linux.sh maps a non-numeric version to 0.0.0-<version>; the other
  # scripts only strip a leading v. Match each consumer so a binary built
  # here is always a cache hit there.
  os_version="$version"
  if [ "$os" = "linux" ]; then
    case "$os_version" in
      [0-9]*) ;;
      *) os_version="0.0.0-$os_version" ;;
    esac
  fi
  case "$os" in
    linux) ldflags="-s -w"; ext="" ;;
    darwin) ldflags="-s -w$macos_ldflag"; ext="" ;;
    windows) ldflags="-s -w -H windowsgui"; ext=".exe" ;;
    *) echo "package-binary: unknown OS '$os'" >&2; exit 2 ;;
  esac
  for arch in $arches; do
    out="$cache/relo-$os-$arch-$os_version$ext"
    if [ -x "$out" ]; then
      echo "reusing $os/$arch for $os_version"
      continue
    fi
    echo "building $os/$arch"
    (cd "$root/daemon" && GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
      -ldflags "$ldflags -X github.com/jonaskahn/relo/internal/cli.version=$os_version" \
      -o "$out" ./cmd/relo)
  done
done
ls -la "$cache"

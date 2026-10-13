#!/usr/bin/env bash
# Builds Relo.app and zips it twice: the universal bundle and the smaller
# arm64-only bundle for Apple silicon. Runs on macOS and uses the system's
# lipo, sips, and iconutil.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
version="${VERSION:-0.1.0}"
version="${version#v}"
# The bundle states two versions and neither may hold a prerelease: the short
# version is what Apple reads, and the build number is what an upgrade compares,
# so a beta takes BUILD and leaves its short version at the release it descends
# from. The zip name and the binary still carry the full version, so the asset
# name and `relo version` name the exact build.
short="${version%%-*}"
build="${BUILD:-$version}"
[[ "$build" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  echo "the bundle's build version must be three integers (got '$build'); pass BUILD=0.1.<run> for a prerelease" >&2
  exit 1
}
# Sparkle reads the appcast, and the appcast only ever names stable releases. A
# beta would see a stable build number below its own and never update, so it is
# built not to check at all and is replaced by hand instead.
auto_checks=true
[ "$build" != "$version" ] && auto_checks=false
# The Makefile owns the floor; the default only serves a direct run.
macos_min="${MACOS_MIN_VERSION:-12.0}"

# Signing resolves before the long build rather than at the signing step.
# APPLE_SIGN_IDENTITY names the certificate explicitly: CI always sets it to
# the imported Developer ID and never relies on detection. Locally it is
# usually unset, and then a single Developer ID in the keychain is used; with
# none the build falls back to ad-hoc signing, which needs no Apple account
# and still seals every binary so the app can be copied to another Mac.
if [ "$#" -gt 0 ]; then
  echo "unknown argument: $1 (signing is automatic; set APPLE_SIGN_IDENTITY to override)" >&2
  exit 2
fi

if [ "$(uname -s)" != "Darwin" ]; then
  echo "building the macOS app requires macOS" >&2
  exit 1
fi

identity="${APPLE_SIGN_IDENTITY:-}"
if [ -z "$identity" ]; then
  count="$(security find-identity -v -p codesigning 2>/dev/null | grep -c "Developer ID Application" || true)"
  if [ "$count" -eq 1 ]; then
    identity="Developer ID Application"
  elif [ "$count" -eq 0 ]; then
    identity="-"
  else
    echo "$count Developer ID identities found; set APPLE_SIGN_IDENTITY to pick one" >&2
    security find-identity -v -p codesigning >&2
    exit 1
  fi
fi

# Fail before the long build rather than at the signing step, once the whole
# bundle exists and only the identity is unusable. codesign rejects a name
# that matches more than one certificate, so anything but a single match is
# an error here too.
if [ "$identity" != "-" ]; then
  matches="$(security find-identity -v -p codesigning | grep -cF "$identity" || true)"
  if [ "$matches" -ne 1 ]; then
    echo "$matches codesigning identities match \"$identity\"; pass the 40-character hash" >&2
    security find-identity -v -p codesigning >&2
    exit 1
  fi
  echo "signing with \"$identity\""
else
  echo "no Developer ID found; signing ad-hoc"
fi

# Notarization resolves up front too: Apple scans the signed app, and the
# stapled ticket is what silences Gatekeeper on another Mac. Credentials come
# from the environment in CI; locally a missing one is prompted below, and an
# empty answer skips notarization instead of failing the build. Ad-hoc builds
# cannot be notarized. NOTARIZE=0 skips it outright, and
# NOTARY_KEYCHAIN_PROFILE replaces the three values with a stored profile.
notarize="0"
apple_id="${APPLE_ID:-}"
apple_team="${APPLE_TEAM_ID:-}"
apple_password="${APPLE_APP_SPECIFIC_PASSWORD:-}"
if [ "$identity" = "-" ]; then
  echo "ad-hoc build; skipping notarization"
elif [ "${NOTARIZE:-1}" = "0" ]; then
  echo "NOTARIZE=0; skipping notarization"
elif [ -n "${NOTARY_KEYCHAIN_PROFILE:-}" ]; then
  notarize="1"
  echo "notarizing with keychain profile \"$NOTARY_KEYCHAIN_PROFILE\""
else
  if [ -z "$apple_team" ]; then
    if [[ "$identity" =~ ^[A-Fa-f0-9]{40}$ ]]; then
      cert_pem="$(security find-certificate -Z "$identity" -p 2>/dev/null || true)"
    else
      cert_pem="$(security find-certificate -c "$identity" -p 2>/dev/null || true)"
    fi
    if [ -n "${cert_pem:-}" ]; then
      apple_team="$(printf '%s' "$cert_pem" | openssl x509 -noout -subject -nameopt multiline 2>/dev/null | awk -F'= ' '/organizationalUnitName/ {print $2; exit}')"
      [ -n "$apple_team" ] && echo "Team ID $apple_team from the signing certificate"
    fi
    unset cert_pem
  fi
  if { [ -z "$apple_id" ] || [ -z "$apple_team" ] || [ -z "$apple_password" ]; } && [ -t 0 ]; then
    if [ -z "$apple_id" ]; then printf 'Apple ID email: '; IFS= read -r apple_id; fi
    if [ -z "$apple_team" ]; then printf 'Apple Team ID: '; IFS= read -r apple_team; fi
    if [ -z "$apple_password" ]; then printf 'app-specific password: '; IFS= read -rs apple_password; printf '\n'; fi
  fi
  if [ -n "$apple_id" ] && [ -n "$apple_team" ] && [ -n "$apple_password" ]; then
    notarize="1"
    echo "notarizing as $apple_id"
  else
    echo "skipping notarization; set APPLE_ID, APPLE_TEAM_ID, and APPLE_APP_SPECIFIC_PASSWORD to enable it" >&2
  fi
fi

app="$root/dist/Relo.app"
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$root/dist"

# The per-architecture binaries are lipo inputs, not release artifacts: the
# zips carry the finished app, so the loose binaries live outside dist/ and
# never outlast this run.
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

# The internal linker's -macos flag exists only on newer Go (1.27+ writes
# minos 13.0 by default); older linkers lack it and already target the floor.
macos_ldflag=""
if go tool link -h 2>&1 | grep -q -- "-macos"; then
  macos_ldflag=" -macos=$macos_min"
fi

for arch in arm64 amd64; do
  echo "building the $arch binary"
  (cd "$root/daemon" && GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w$macos_ldflag -X github.com/jonaskahn/relo/internal/cli.version=$version" \
    -o "$scratch/relo-darwin-$arch" ./cmd/relo)
done
lipo -create -output "$app/Contents/MacOS/Relo" \
  "$scratch/relo-darwin-arm64" "$scratch/relo-darwin-amd64"
chmod +x "$app/Contents/MacOS/Relo"

# shellcheck source=sparkle-keys.sh
. "$root/scripts/sparkle-keys.sh"
# Outside dist/ on purpose: `make package-clean` wipes dist, and a fresh
# 15 MB download on every run is what made packaging look like it hung.
sparkle_root="$(ensure_sparkle)"
mkdir -p "$app/Contents/Frameworks"
rm -rf "$app/Contents/Frameworks/Sparkle.framework"
cp -R "$sparkle_root/Sparkle.framework" "$app/Contents/Frameworks/Sparkle.framework"

# LaunchServices starts CFBundleExecutable. The Swift host starts Sparkle
# and then the Go desktop app, so an update can replace Relo.app. The
# per-arch slivers stay in the scratch directory so the bundle only ever
# holds the finished universal launcher.
for arch in arm64 x86_64; do
  echo "building ReloLauncher $arch"
  swiftc -O -target "${arch}-apple-macos${macos_min}" \
    -F "$app/Contents/Frameworks" \
    -framework Sparkle \
    -Xlinker -rpath -Xlinker @executable_path/../Frameworks \
    -o "$scratch/ReloLauncher-$arch" \
    "$root/packaging/darwin/ReloLauncher.swift"
done
lipo -create -output "$app/Contents/MacOS/ReloLauncher" \
  "$scratch/ReloLauncher-arm64" "$scratch/ReloLauncher-x86_64"
chmod +x "$app/Contents/MacOS/ReloLauncher"

iconset="$(mktemp -d)/Relo.iconset"
mkdir -p "$iconset"
sips -z 16 16 "$root/packaging/darwin/app-icon.png" --out "$iconset/icon_16x16.png" >/dev/null
sips -z 32 32 "$root/packaging/darwin/app-icon.png" --out "$iconset/icon_16x16@2x.png" >/dev/null
sips -z 32 32 "$root/packaging/darwin/app-icon.png" --out "$iconset/icon_32x32.png" >/dev/null
sips -z 64 64 "$root/packaging/darwin/app-icon.png" --out "$iconset/icon_32x32@2x.png" >/dev/null
sips -z 128 128 "$root/packaging/darwin/app-icon.png" --out "$iconset/icon_128x128.png" >/dev/null
sips -z 256 256 "$root/packaging/darwin/app-icon.png" --out "$iconset/icon_128x128@2x.png" >/dev/null
sips -z 256 256 "$root/packaging/darwin/app-icon.png" --out "$iconset/icon_256x256.png" >/dev/null
sips -z 512 512 "$root/packaging/darwin/app-icon.png" --out "$iconset/icon_256x256@2x.png" >/dev/null
sips -z 512 512 "$root/packaging/darwin/app-icon.png" --out "$iconset/icon_512x512.png" >/dev/null
sips -z 1024 1024 "$root/packaging/darwin/app-icon.png" --out "$iconset/icon_512x512@2x.png" >/dev/null
iconutil -c icns "$iconset" -o "$app/Contents/Resources/icon.icns"

# SPARKLE_PUBLIC_ED_KEY pins the update feed to this publisher; generate the
# keypair at build time when unset so a local run never needs a stored secret.
public_key="${SPARKLE_PUBLIC_ED_KEY:-}"
if [ -z "$public_key" ]; then
  public_key="$(sparkle_public_key)"
  echo "generated the Sparkle keypair in the login keychain"
fi
sed -e "s/__VERSION__/$short/g" -e "s/__BUILD__/$build/g" \
  -e "s/__SU_AUTO_CHECKS__/$auto_checks/g" \
  -e "s/__SU_PUBLIC_ED_KEY__/$public_key/g" \
  -e "s/__MACOS_MIN_VERSION__/$macos_min/g" \
  "$root/packaging/darwin/Info.plist" > "$app/Contents/Info.plist"

# Without a signature a copy that reaches another Mac has no seal for
# Gatekeeper to evaluate, so the app silently never opens. Ad-hoc signing
# needs no Apple account; the first launch elsewhere still takes the
# quarantine step in CONTRIBUTING.md. A Developer ID identity plus
# notarization is the upgrade path. Nested code is signed before the code
# that contains it, so --deep is not needed. Developer ID builds seal with
# the hardened runtime and a secure timestamp: Apple returns Invalid without
# both, which is the rejection above. Ad-hoc keeps a plain seal so offline
# builds still work.
sign() {
  if [ "$identity" = "-" ]; then
    codesign --force --sign "$identity" "$@"
  else
    codesign --force --options runtime --timestamp --sign "$identity" "$@"
  fi
}

# The bundle's own code is sealed last; the arm64 copy below swaps two
# executables and has to repeat just this part.
sign_outer() {
  sign --identifier com.relo.tray.relo "$1/Contents/MacOS/Relo"
  sign "$1/Contents/MacOS/ReloLauncher"
  sign "$1"
  codesign --verify --deep --strict "$1"
}

sparkle_b="$app/Contents/Frameworks/Sparkle.framework/Versions/B"
sign "$sparkle_b/Autoupdate"
sign "$sparkle_b/Updater.app"
for xpc in "$sparkle_b"/XPCServices/*.xpc; do sign "$xpc"; done
sign "$app/Contents/Frameworks/Sparkle.framework"
sign_outer "$app"

# A notarized app carries Apple's scan ticket; without it Gatekeeper warns on
# another Mac even when the signature is valid. When enabled above, each zip
# is submitted, stapled back into the .app, and re-zipped so the published
# artifact is exactly the stapled bundle. The arm64 zip reuses the same
# ticket: its executables are rebuilt, so it re-staples from Apple's record.
notarize_zip() {
  local zip="$1" app_dir="$2" ticket
  if [ "$notarize" != "1" ]; then
    return 0
  fi
  echo "submitting $zip for notarization"
  if [ -n "${NOTARY_KEYCHAIN_PROFILE:-}" ]; then
    xcrun notarytool submit "$zip" --keychain-profile "$NOTARY_KEYCHAIN_PROFILE" --wait
  else
    xcrun notarytool submit "$zip" --apple-id "$apple_id" --team-id "$apple_team" --password "$apple_password" --wait
  fi
  ticket="$(mktemp -d)/ticketed.app"
  (cd "$app_dir" && ditto -x -k "$zip" "$ticket")
  xcrun stapler staple "$ticket/Relo.app"
  xcrun stapler validate "$ticket/Relo.app"
  (cd "$ticket" && ditto -c -k --keepParent Relo.app "$zip")
  rm -rf "$ticket"
  echo "stapled and re-zipped $zip"
}

(cd "$root/dist" && ditto -c -k --keepParent Relo.app "Relo-$version-universal.zip")
ls -la "$root/dist/Relo-$version-universal.zip"
notarize_zip "$root/dist/Relo-$version-universal.zip" "$root/dist"

# The arm64-only app is the smaller download for Apple silicon. It is
# assembled from a copy, so the universal app left in dist/ is exactly what
# the first zip holds: only the two executables differ.
mkdir -p "$scratch/arm64"
cp -R "$app" "$scratch/arm64/Relo.app"
  cp "$scratch/relo-darwin-arm64" "$scratch/arm64/Relo.app/Contents/MacOS/Relo"
cp "$scratch/ReloLauncher-arm64" "$scratch/arm64/Relo.app/Contents/MacOS/ReloLauncher"
chmod +x "$scratch/arm64/Relo.app/Contents/MacOS/Relo" "$scratch/arm64/Relo.app/Contents/MacOS/ReloLauncher"
sign_outer "$scratch/arm64/Relo.app"
(cd "$root/dist" && ditto -c -k --keepParent "$scratch/arm64/Relo.app" "Relo-$version-arm64.zip")
ls -la "$root/dist/Relo-$version-arm64.zip"
notarize_zip "$root/dist/Relo-$version-arm64.zip" "$scratch/arm64"

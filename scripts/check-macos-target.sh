#!/usr/bin/env bash
# Fails when the built app claims a newer macOS than the floor, or is not
# validly signed. Every Mach-O in the bundle may require the floor or older,
# never newer, and Info.plist must state the floor itself.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
app="${1:-$root/dist/Relo.app}"
floor="${MACOS_MIN_VERSION:-12.0}"

fail=0
plist="$(/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "$app/Contents/Info.plist")"
if [ "$plist" != "$floor" ]; then
  echo "LSMinimumSystemVersion is $plist, want $floor" >&2
  fail=1
fi

for bin in "$app/Contents/MacOS/Relo" "$app/Contents/MacOS/ReloLauncher" \
  "$app/Contents/Frameworks/Sparkle.framework/Versions/B/Sparkle"; do
  # One minos line per architecture slice.
  for minos in $(vtool -show-build "$bin" | awk '$1 == "minos" { print $2 }'); do
    newest="$(printf '%s\n%s\n' "$minos" "$floor" | sort -V | tail -n 1)"
    if [ "$newest" != "$floor" ]; then
      echo "${bin#"$app"/} requires macOS $minos, above the $floor floor" >&2
      fail=1
    fi
  done
done

codesign --verify --deep --strict "$app" || fail=1

[ "$fail" -eq 0 ] && echo "macOS floor $floor holds for ${app#"$root"/}"
exit "$fail"

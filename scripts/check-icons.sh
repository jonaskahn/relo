#!/usr/bin/env bash
# Fails when the committed artwork no longer matches what the brand
# sources generate, whether the sources are the plated logo.svg or the
# bare logo-mark.svg the tray reads. The icons are rendered into a
# temporary tree, so a developer's working copy is never rewritten by
# the check.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
staging="$(mktemp -d)"
trap 'rm -rf "$staging"' EXIT

mkdir -p "$staging/assets"
cp "$root/assets/logo.svg" "$staging/assets/"
cp "$root/assets/logo-mark.svg" "$staging/assets/"

echo "=== Relo asset drift check ==="
(cd "$root/daemon" && go run ./tools/genicons -root "$staging" >/dev/null)
"$root/scripts/gen-syso.sh" "$staging/daemon/cmd/relo" >/dev/null

# oksvg antialiasing is not identical across GOARCH, so the committed
# rasters are the linux/amd64 rendering CI produces. Other hosts still
# prove genicons runs and that every derived file is present.
host="$(cd "$root/daemon" && go env GOHOSTOS)/$(cd "$root/daemon" && go env GOHOSTARCH)"

drift=0
compare() {
  local generated="$1" committed="$2"
  if [ ! -f "$committed" ]; then
    echo "FAIL: $committed is missing"
    drift=1
    return
  fi
  if cmp -s "$generated" "$committed"; then
    return
  fi
  if [ "$host" != "linux/amd64" ]; then
    return
  fi
  echo "FAIL: $committed differs from the generated file (run make icons)"
  drift=1
}

compare "$staging/assets/logo.png" "$root/assets/logo.png"
compare "$staging/packaging/darwin/app-icon.png" "$root/packaging/darwin/app-icon.png"
compare "$staging/daemon/internal/desktop/assets/icon.png" "$root/daemon/internal/desktop/assets/icon.png"
compare "$staging/daemon/internal/desktop/assets/icon-template.png" "$root/daemon/internal/desktop/assets/icon-template.png"
compare "$staging/packaging/windows/icon.ico" "$root/packaging/windows/icon.ico"
for size in 16 24 32 48 64 128 256 512 1024; do
  icon="packaging/linux/icons/${size}x${size}/apps/relo.png"
  compare "$staging/$icon" "$root/$icon"
done
compare "$staging/daemon/internal/server/assets/favicon.ico" "$root/daemon/internal/server/assets/favicon.ico"
compare "$staging/console/static/favicon.ico" "$root/console/static/favicon.ico"
# The Windows resource object: icon.ico linked into every relo.exe.
for arch in amd64 arm64; do
  compare "$staging/daemon/cmd/relo/relo_windows_$arch.syso" "$root/daemon/cmd/relo/relo_windows_$arch.syso"
done
if ! cmp -s "$root/assets/logo.svg" "$root/console/static/logo.svg"; then
  echo "FAIL: $root/console/static/logo.svg differs from assets/logo.svg (run make icons)"
  drift=1
fi

if [ "$drift" -eq 0 ]; then
  echo "OK: every committed asset matches its source"
fi
exit "$drift"

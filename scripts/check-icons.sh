#!/usr/bin/env bash
# Fails when the committed artwork no longer matches what the brand
# sources generate, whether the sources are the plated logo.svg or the
# bare logo-mark.svg the tray reads. genicons compares the rasters as
# decoded pixels rather than as bytes: compressing the same rendering is
# the encoder's business, and the encoder's output moves between Go
# releases, so a byte compare reports drift no designer caused. oksvg
# antialiasing moves with the build platform, so the pixel check only
# binds on linux/amd64, where CI runs; other hosts prove the generator
# runs and every derived file exists. The Windows resource object is
# generated into a temporary tree, so a developer's working copy is never
# rewritten by the check.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
staging="$(mktemp -d)"
trap 'rm -rf "$staging"' EXIT

echo "=== Relo asset drift check ==="
host="$(cd "$root/daemon" && go env GOHOSTOS)/$(cd "$root/daemon" && go env GOHOSTARCH)"
if [ "$host" = "linux/amd64" ]; then
  (cd "$root/daemon" && go run ./tools/genicons -root "$root" -check)
else
  echo "host is $host, not linux/amd64: proving the generator runs and every derived file exists"
  mkdir -p "$staging/assets"
  cp "$root/assets/logo.svg" "$staging/assets/"
  cp "$root/assets/logo-mark.svg" "$staging/assets/"
  (cd "$root/daemon" && go run ./tools/genicons -root "$staging" >/dev/null)
  for derived in \
    assets/logo.png \
    packaging/darwin/app-icon.png \
    daemon/internal/desktop/assets/app-icon.png \
    daemon/internal/desktop/assets/icon.png \
    daemon/internal/desktop/assets/icon-template.png \
    packaging/windows/icon.ico \
    daemon/internal/server/assets/favicon.ico \
    console/static/favicon.ico \
    packaging/linux/icons/16x16/apps/relo.png \
    packaging/linux/icons/1024x1024/apps/relo.png; do
    if [ -f "$staging/$derived" ] && [ -f "$root/$derived" ]; then
      continue
    fi
    echo "FAIL: $root/$derived is missing (run make icons)"
    exit 1
  done
fi

# The resource object wraps the committed icon and the generator is pinned,
# so it compares byte for byte.
"$root/scripts/gen-syso.sh" "$staging/daemon/cmd/relo" >/dev/null
drift=0
for arch in amd64 arm64; do
  syso="daemon/cmd/relo/relo_windows_$arch.syso"
  if ! cmp -s "$staging/$syso" "$root/$syso"; then
    echo "FAIL: $root/$syso differs from the generated file (run make icons)"
    drift=1
  fi
done
if ! cmp -s "$root/assets/logo.svg" "$root/console/static/logo.svg"; then
  echo "FAIL: $root/console/static/logo.svg differs from assets/logo.svg (run make icons)"
  drift=1
fi

if [ "$drift" -eq 0 ]; then
  echo "OK: every committed asset matches its source"
fi
exit "$drift"

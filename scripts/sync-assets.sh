#!/usr/bin/env bash
# Copies the shared brand assets into the app trees that serve them. The
# console build only sees files under console/static, so the vector mark
# is mirrored there instead of being maintained twice.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
source_file="$root/assets/logo.svg"
target_file="$root/console/static/logo.svg"

if [ ! -f "$source_file" ]; then
  echo "no brand asset at $source_file" >&2
  exit 1
fi

mkdir -p "$(dirname "$target_file")"
if cmp -s "$source_file" "$target_file"; then
  echo "assets: logo.svg already current"
  exit 0
fi

cp "$source_file" "$target_file"
echo "assets: synced logo.svg into console/static"

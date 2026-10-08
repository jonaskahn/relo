#!/usr/bin/env bash
# Copies the compiled console into the Go package that embeds it.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
build_dir="$root/console/build"
target_dir="$root/daemon/internal/dashboard/dist"

if [ ! -f "$build_dir/index.html" ]; then
  echo "no console build at $build_dir; run make build-console first" >&2
  exit 1
fi

rm -rf "$target_dir"
mkdir -p "$target_dir"
cp -R "$build_dir"/. "$target_dir"/
# The placeholder keeps go:embed satisfied for a checkout that never built
# the console.
touch "$target_dir/.gitkeep"
printf 'synced %s files into daemon/internal/dashboard/dist\n' "$(find "$target_dir" -type f | wc -l | tr -d ' ')"

#!/usr/bin/env bash
# Builds the Svelte console into console/build.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root/console"

if [ ! -d node_modules ]; then
  npm ci
fi
npm run build

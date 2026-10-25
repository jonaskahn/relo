#!/usr/bin/env bash
# Builds the Svelte console into console/build.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root/console"

# A stray or partial node_modules passes a directory check but holds no
# binaries, so guard on the build tool itself rather than the directory.
if [ ! -x node_modules/.bin/vite ]; then
  npm ci
fi
# SvelteKit generates .svelte-kit/tsconfig.json (and friends) during sync;
# `vite build` needs them, and a clean checkout or `package-clean` removes them.
npm run prepare
npm run build

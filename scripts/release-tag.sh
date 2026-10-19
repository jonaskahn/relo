#!/usr/bin/env bash
# Tags a release from latest main, callable from any branch.
#
# TAG defaults to the version main declares (scripts/version.py), so a bare
# `make release-tag` tags what the release tree already declares. The tag always
# lands on main: a clean tree is required, main is checked out and
# fast-forwarded to origin/main first. The default is read after that, since a
# called-from branch may still declare the previous version.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if [ -n "$(git status --porcelain)" ]; then
  echo "release-tag: working tree is not clean; commit or stash first" >&2
  exit 1
fi

git fetch origin

branch="$(git rev-parse --abbrev-ref HEAD)"
if [ "$branch" != "main" ]; then
  git checkout main
fi
git pull --ff-only origin main

tag="${1:-${TAG:-}}"
if [ -z "$tag" ]; then
  tag="v$(python3 scripts/version.py current)"
fi

python3 scripts/version.py check "$tag"
git tag "$tag"
git push origin "$tag"

echo "release-tag: tagged $tag on main"

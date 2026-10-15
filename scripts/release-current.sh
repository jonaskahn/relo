#!/usr/bin/env bash
# Tags the current declared version and opens the next version on a branch.
#
# Flow: main must be clean and latest, the current version (from
# scripts/version.py) must have no tag yet, then the tag is created and pushed.
# Afterwards the version is bumped (prompt defaults to patch), committed on a
# new branch, and pushed so CI runs on it. Nothing is committed to main
# directly.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

kind="${1:-${KIND:-}}"
if [ -z "$kind" ]; then
  if [ -r /dev/tty ]; then
    printf 'bump kind [major/minor/patch/pre or x.y.z] (default: patch): ' > /dev/tty
    IFS= read -r kind < /dev/tty || true
  fi
  kind="${kind:-patch}"
fi

# Fail before the tag is pushed when the kind could never bump: version.py
# accepts major, minor, patch, pre, or an explicit semantic version.
case "$kind" in
  major|minor|patch|pre) ;;
  *)
    if ! [[ "$kind" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-.+)?$ ]]; then
      echo "release-current: invalid kind '$kind'; use major, minor, patch, pre, or x.y.z" >&2
      exit 2
    fi
    ;;
esac

git fetch origin

if [ -n "$(git status --porcelain)" ]; then
  echo "release-current: working tree is not clean; commit or stash first" >&2
  exit 1
fi

branch="$(git rev-parse --abbrev-ref HEAD)"
if [ "$branch" != "main" ]; then
  git checkout main
fi
git pull --ff-only origin main

ver="$(python3 scripts/version.py current)"
tag="v$ver"

if git show-ref --verify --quiet "refs/tags/$tag"; then
  echo "release-current: local tag $tag already exists" >&2
  exit 1
fi
if git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null 2>&1; then
  echo "release-current: remote tag $tag already exists" >&2
  exit 1
fi

python3 scripts/version.py check "$tag"
git tag "$tag"
git push origin "$tag"

newver="$(python3 scripts/version.py bump "$kind")"
python3 scripts/version.py check

newbranch="feat/auto-increase-version-v$newver"
if git show-ref --verify --quiet "refs/heads/$newbranch"; then
  echo "release-current: local branch $newbranch already exists" >&2
  exit 1
fi
if git ls-remote --exit-code --heads origin "refs/heads/$newbranch" >/dev/null 2>&1; then
  echo "release-current: remote branch $newbranch already exists" >&2
  exit 1
fi

git checkout -b "$newbranch"
files=""
for file in Makefile daemon/internal/cli/root.go console/package.json \
  console/package-lock.json packaging/npm/package.json README.md; do
  if [ -f "$file" ]; then
    files="$files $file"
  fi
done
# shellcheck disable=SC2086
git add $files
git commit -m "chore: bump version to $newver"
git push -u origin "$newbranch"

echo "release-current: tagged $tag, bumped to $newver on $newbranch"

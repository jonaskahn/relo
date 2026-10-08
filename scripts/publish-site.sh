#!/bin/sh
# Refresh the local gh-pages branch from site/. Does not push.
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
site="$root/site"

if [ ! -f "$site/index.html" ] || [ ! -f "$site/appcast.xml" ]; then
	echo "publish-site.sh: site/ is missing index.html or appcast.xml" >&2
	exit 1
fi

if ! git -C "$root" show-ref --verify --quiet refs/heads/gh-pages; then
	git -C "$root" branch gh-pages
fi

work="$(mktemp -d)"
cleanup() {
	git -C "$root" worktree remove --force "$work" >/dev/null 2>&1 || true
	rm -rf "$work"
}
trap cleanup EXIT

git -C "$root" worktree add --force "$work" gh-pages
find "$work" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
cp -R "$site/." "$work/"
git -C "$work" add -A
if git -C "$work" diff --cached --quiet; then
	echo "gh-pages already matches site/"
	exit 0
fi
git -C "$work" commit -m "Publish Relo site"
echo "updated local gh-pages; push when you want it live"

#!/usr/bin/env bash
# Per-package coverage gate. Reads the source/test mapping in
# daemon/tests/coverage.tsv.
#
# The gate fails when a mapped test package is missing, when a source package
# under daemon/internal is neither mapped nor declared exempt, when a mapped
# test run fails, or when a source package falls below the threshold.
set -uo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
backend="$root/daemon"
threshold="${COVERAGE_THRESHOLD:-90.0}"
mapping="$backend/tests/coverage.tsv"
fail=0

cd "$backend" || exit 1

declare -A mapped
profiles=()

# The gate fails when a mapped test package is missing, when a source package
# under daemon/internal is neither mapped nor declared exempt, when a mapped
# test run fails, or when a package measures below its floor. The floor is the
# global threshold unless the row names its own.
measure() {
	local source="$1" tests="$2" floor="$3" profile target directory
	local targets=() resolved=()
	IFS=',' read -r -a targets <<<"$tests"
	for target in "${targets[@]}"; do
		directory="$(printf '%s' "$target" | sed 's|/\.\.\.$||')"
		if [ ! -d "$directory" ]; then
			printf 'FAIL %-42s missing test package %s\n' "$source" "$target"
			fail=1
			return
		fi
		resolved+=("./$target")
	done
  profile="$(mktemp "${TMPDIR:-/tmp}/relo-cover.XXXXXX")"
  profiles+=("$profile")
  local log
  log="$(mktemp "${TMPDIR:-/tmp}/relo-cover-log.XXXXXX")"
  if ! CGO_ENABLED=0 go test -count=1 -covermode=atomic \
	    -coverpkg="./$source" -coverprofile="$profile" "${resolved[@]}" >"$log" 2>&1 </dev/null; then
    printf 'FAIL %-42s tests failed\n' "$source"
    cat "$log"
    rm -f "$log"
    fail=1
    return
  fi
  rm -f "$log"
  local total
  total="$(go tool cover -func="$profile" | awk '/^total:/ {sub("%","",$3); print $3}')"
  if [ -z "$total" ]; then
    printf 'FAIL %-42s no coverage data\n' "$source"
    fail=1
    return
  fi
	if awk "BEGIN {exit !($total >= $floor)}"; then
		printf 'ok   %-42s %s%% (floor %s%%)\n' "$source" "$total" "$floor"
	else
		printf 'FAIL %-42s %s%% (floor %s%%)\n' "$source" "$total" "$floor"
		fail=1
	fi
}

while IFS=$'\t' read -r source tests rest; do
  [ -z "$source" ] && continue
  case "$source" in \#*) continue ;; esac
  if [ "$source" = "exempt" ]; then
    if [ -z "$tests" ]; then
      printf 'FAIL exempt row names no package\n'
      fail=1
      continue
    fi
    mapped["$tests"]="exempt"
    printf 'skip %-42s %s\n' "$tests" "${rest:-exempt}"
    continue
  fi
	mapped["$source"]="measured"
	measure "$source" "$tests" "${rest:-$threshold}"
done <"$mapping"

# Every source package must be a decision, not an oversight.
while read -r package; do
  source="$(printf '%s' "$package" | sed "s|^github.com/jonaskahn/relo/||")"
  case "$source" in internal/*) ;; *) continue ;; esac
  if [ -z "${mapped[$source]:-}" ]; then
    printf 'FAIL %-42s not listed in tests/coverage.tsv\n' "$source"
    fail=1
  fi
done < <(go list -f '{{.ImportPath}}' ./internal/...)

for file in "${profiles[@]:-}"; do
  [ -n "$file" ] && rm -f "$file"
done

if [ "$fail" -eq 0 ]; then
	echo "coverage: every measured package met its floor (target $threshold%)"
else
  echo "coverage: gate not met"
fi
exit "$fail"

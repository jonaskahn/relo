#!/usr/bin/env bash
set -uo pipefail
FAIL=0
echo "=== Relo Plan Validation ==="
echo

count_files() { find "$@" | wc -l | xargs; }
count_matches() { grep "$@" 2>/dev/null | wc -l | xargs; }

TC=$(count_files plans/phases -name "P*.md")
[ "$TC" -eq 63 ] && echo "OK: $TC task files" || { echo "FAIL: Expected 63, found $TC"; FAIL=1; }

DUPS=$(grep -h "^id:" plans/phases/*/P*.md | sort | uniq -d || true)
[ -z "$DUPS" ] && echo "OK: All task IDs unique" || { echo "FAIL: Duplicate IDs: $DUPS"; FAIL=1; }

CC=$(count_matches -rl "^conventions: plans/CONVENTIONS.md" plans/phases/*/P*.md)
[ "$CC" -eq "$TC" ] && echo "OK: All tasks reference CONVENTIONS.md" || { echo "FAIL: $CC/$TC"; FAIL=1; }

BT=$(count_matches -rn "internal/.*_test\.go" plans/phases/*/P*.md)
[ "$BT" -eq 0 ] && echo "OK: No test files inside internal/" || { echo "FAIL: $BT refs"; FAIL=1; }

BD=$(count_matches -rn "internal/.*/testdata" plans/phases/*/P*.md)
[ "$BD" -eq 0 ] && echo "OK: No testdata inside internal/" || { echo "FAIL: $BD refs"; FAIL=1; }

FC=$(count_matches -rn "internal/clock/fake" plans/)
[ "$FC" -eq 0 ] && echo "OK: No fake clock inside internal/" || { echo "FAIL: $FC refs"; FAIL=1; }

ALL_IDS=$(grep -h "^id:" plans/phases/*/P*.md | sed "s/^id: *//" | sort)
ALL_DEPS=$(grep -h "^depends_on:" plans/phases/*/P*.md | sed "s/^depends_on: *\[//;s/\].*//;s/,/ /g" | tr " " "\n" | sed "s/^ *//;s/ *$//" | grep -v "^$" | sort -u || true)
MISSING=""
for dep in $ALL_DEPS; do
  echo "$ALL_IDS" | grep -q "^${dep}$" || MISSING="$MISSING $dep"
done
[ -z "$MISSING" ] && echo "OK: All dependencies resolve" || { echo "FAIL: Missing:$MISSING"; FAIL=1; }

echo
echo "Phase breakdown:"
for d in plans/phases/*/; do
  c=$(count_files "$d" -name "P*.md")
  echo "  $(basename "$d"): $c"
done

echo
[ $FAIL -eq 0 ] && echo "PASSED: All checks green" || { echo "FAILED: $FAIL errors"; exit 1; }

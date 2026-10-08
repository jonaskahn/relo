#!/usr/bin/env bash
# Design preflight. Usage: bash docs/design/specs/tools/check-design.sh [dir=console/src] (from the repo root)
# Fails on: partial borders, uppercase text, raw colours in components.
# Skip a line deliberately with the comment: design-allow
dir="${1:-console/src}"
fail=0
scan () {  # label, regex, [extra grep -v pattern]
  local out
  out=$(grep -RInE --include='*.svelte' --include='*.css' --include='*.ts' --include='*.html' "$2" "$dir" \
        | grep -v 'design-allow' | grep -vE "${3:-^$}" )
  if [ -n "$out" ]; then echo "✗ $1"; echo "$out" | sed 's/^/    /'; fail=1; fi
}
# 1. partial borders: Tailwind (border-l, border-t-2, divide-y, md:border-b …) and CSS (border-left:, border-inline-start:)
scan "Partial border (use a full border or none: docs/design/specs/foundations/surfaces-borders.md)" \
  '(^|[^a-zA-Z0-9_-])(border|divide)-(l|r|t|b|x|y|s|e)([^a-zA-Z]|$)|border-(left|right|top|bottom|inline-start|inline-end|block-start|block-end)[[:space:]]*:' \
  'border-t-transparent'
# 2. uppercase / tracked-out labels
scan "Uppercase or tracked label (sentence case only: docs/design/specs/foundations/typography-spacing.md)" \
  '(^|[^a-zA-Z0-9_-])uppercase([^a-zA-Z]|$)|text-transform[[:space:]]*:[[:space:]]*uppercase|tracking-(wide|wider|widest)'
# 3. raw colours in components (tokens only), app.css and logos excluded
out=$(grep -RInE --include='*.svelte' "#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(|(bg|text|border|ring|fill|stroke)-(red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|slate|gray|zinc|neutral|stone)-[0-9]{2,3}" "$dir" \
      | grep -v 'design-allow' | grep -vE 'Icon\.svelte|provider-logos|&#[0-9]+;|href="#|url\(#')
if [ -n "$out" ]; then echo "✗ Raw colour (use tokens: docs/design/specs/foundations/color.md)"; echo "$out" | sed 's/^/    /'; fail=1; fi
[ $fail -eq 0 ] && echo "✓ design checks passed"
exit $fail

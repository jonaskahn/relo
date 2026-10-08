---
name: appearance-popover
description: Theme, accent and language switcher and its data attributes.
requires: [color, i18n-a11y]
---

# Appearance popover

Opened from the 38px icon button at the right of the top bar (`aria-haspopup="dialog"`, `aria-expanded`). Level-3
surface, `--radius-2xl`, width `min(360px, 100vw - 2rem)`, anchored under the button, padding 1.125rem. Escape and
outside click close it and return focus to the button. It opens and closes with the shared popover motion. The same
three controls appear on the Login page and in Settings → Appearance.

Sections use a 12.5px `--muted` 600 title in sentence case ("Theme", "Accent", "Language"), 20px apart.

## Theme

Three radio cards in a 3-column grid: **System**, **Light**, **Dark**. Each shows a 54px mini preview (sidebar bar, two
lines, an accent pill) above the label. System is split light and dark. Selected card: 2px `--ink` full border,
`--canvas` fill, check icon before the label. The accent pill in the previews follows the current accent.

## Accent

Ten swatches in a 5×2 grid, 44px tiles, `--radius` 12px, `role="radiogroup"`. Selected: check icon in `--on-accent`,
ring `0 0 0 3px var(--surface), 0 0 0 5px var(--ink)`. Each has an `aria-label` (Red, Orange, Amber, Green, Teal, Cyan,
Blue, Indigo, Purple, Rose). Swatches show the light-theme base in both themes.

## Language

One select, full width, native language names (see `i18n-a11y`). 22 options: use the Bits UI select with a scrollable
list.

## Contract

```ts
document.documentElement.dataset.theme  = resolvedTheme;   // "light" | "dark"; System follows prefers-color-scheme
document.documentElement.dataset.accent = accentId;        // red … rose; absent = red
document.documentElement.lang           = locale;
```

Persist in this browser only (the daemon does not store appearance). Applying a choice changes tokens, never component
code. Re-focus the selected radio after a change so keyboard users keep their place.

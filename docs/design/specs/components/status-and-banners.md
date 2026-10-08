---
name: status-and-banners
description: Status pills, banners, empty states, toasts, loading.
requires: [color, surfaces-borders]
---

# Status, banners, empty and loading

## Status pill

Inline, full radius, 12px 500, padding 4px 10px. A 6px dot (or an icon for warn and danger) then text.

| Tone    | Text           | Fill          |
|---------|----------------|---------------|
| ok      | `--ok`         | `--ok-bg`     |
| warn    | `--warn`       | `--warn-bg`   |
| danger  | `--danger`     | `--danger-bg` |
| accent  | `--accent-ink` | `--accent-50` |
| neutral | `--muted`      | `--sunken`    |

Words: Ready, Near limit, Limit reached, Sign in again, Paused, Error. State what is true; never show an absence as
success. A name that resolves to nothing reads "No models", not "OK".

## Banner

Full 1px border in `color-mix(in srgb,<tone> 26%,var(--surface))`, fill `color-mix(in srgb,<tone> 6%,var(--surface))`,
`--radius-xl`, padding 12px 16px. Leading 30px icon tile (`--<tone>-bg`, tone icon), message 14px, then the next useful
action as a button or link. No stripe on any side.

**Attention strip** (top of Connections, Dashboard): a neutral surface band — full hairline `--line` border,
`--radius-card`, `--surface` fill, no tint, padding 10px 16px. On the left a 15px alert icon in the worst tone and
`N need attention` (600, 14px). Then one wrapping line of text buttons, one per item — a 6px tone dot, the name in
`--ink` 500, the reason in `--muted` (`weekly limit reached, resets in 1h 30m`, `at 92%`) — separated by 24px gaps. No
pill fills and no icon tiles; danger dot for limit reached, warn for near limit and for the unpriced item; hover lifts
the reason to `--ink`. An item is a button that opens what it names. Hide the band when nothing needs attention; the
whole band is hidden below 640px.

## Empty state

Say what is missing and offer the action. Dashed 1.5px full border (`--accent-border`), `--accent-50` fill,
`--radius-xl`, centred: title 600 then one line `--muted`, then the button. Examples: "No connections yet" with Add
connection; "No members yet" with "Add a model from the list to start routing requests." Never draw fictitious data.

**Add tile** (last cell of a grid): same dashed border, transparent fill, 44px accent-tinted plus circle, label 500
`--accent-700`, one `--muted` line. Hover fills `--accent-50`.

## Toast

Bottom centre, `--ink` fill with `--canvas` text, `--radius`, 10px 16px, level 3 shadow, 2s. Wording reuses the action's
verb ("Saved group", "Prices refreshed"). Failures do not use a toast: they stay inline next to the thing that failed,
with the reason and the fix. Toasts have `role="status"`.

## Loading

- Slot loading: the layout stays; each slot shows a centred spinner until its data arrives.
- Spinner: 16–20px SVG arc in `--accent`, 700ms linear; under reduced motion a static arc.
- Buttons: spinner in place of the icon (see `buttons`).
- Long operations (restart, verify) report each result before the flow ends: credential, model list, pricing.

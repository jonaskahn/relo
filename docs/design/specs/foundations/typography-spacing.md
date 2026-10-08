---
name: typography-spacing
description: Type scale, spacing, control height, radius.
requires: [color]
---

# Typography, spacing, radius

## Type

- UI: `system-ui, -apple-system, "Segoe UI", sans-serif`. No web font for UI text.
- Data, ids, slugs, prices, counts, durations: JetBrains Mono from `/fonts/`, via `.mono-data`. Add `tabular-nums`.
- Weights: 400, 500, 600 only.

| Role                      | Size / line           | Weight                              |
|---------------------------|-----------------------|-------------------------------------|
| Page title                | 1.875rem / 1.1        | 600, tracking -0.025em (Latin only) |
| Modal title               | 1.125rem / 1.3        | 600                                 |
| Card title, section title | 1rem (card 0.9375rem) | 600                                 |
| Page body                 | 0.9375rem / 1.5       | 400                                 |
| Controls, dense body      | 0.875rem / 1.4        | 400–500                             |
| Label, helper             | 0.8125rem             | 500 / 400                           |
| Caption, meta, mono data  | 0.75rem minimum       | 400–500                             |

Rules: sentence case everywhere. No `uppercase`, no `text-transform`, no tracked-out eyebrow labels (breaks Turkish and
German casing and CJK, Thai, Hindi typography). Prose lines at most 72ch. Reset tracking to 0 under
`:lang(ja),:lang(ko),:lang(zh),:lang(th),:lang(hi)`.

## Spacing (4px base)

| Token        | Value                             | Use                   |
|--------------|-----------------------------------|-----------------------|
| `--gutter`   | 1.75rem (1rem at 640px and below) | Page side padding     |
| `--section`  | 1.625rem                          | Between page sections |
| `--grid-gap` | 1.125rem                          | Card grids            |
| `--card-pad` | 1.125rem                          | Card inner padding    |
| `--stack`    | 0.5rem / 0.75rem / 1rem           | Inside components     |

Max content width 1360px, centred. Page body keeps the 15px size and the shared section rhythm.

## Control height

```css
:root{--control-h:2.375rem;--control-h-sm:2rem}               /* 38px, 32px */
@media (pointer:coarse),(max-width:640px){:root{--control-h:2.75rem;--control-h-sm:2.75rem}}  /* 44px */
```

Every control in one row shares `--control-h`: buttons, search, selects, filter chips, icon buttons, toggle-group
buttons. `--control-h-sm` is for dense actions inside cards and rows.

## Radius

`--radius` is the base control radius. Tailwind tokens map to these.

| Token              | Value    | Use                                                        |
|--------------------|----------|------------------------------------------------------------|
| `--radius-sm`      | 0.5rem   | Chips inside cards, small inputs, tags                     |
| `--radius` / `-md` | 0.625rem | Buttons, inputs, selects, icon buttons                     |
| `--radius-lg`      | 0.75rem  | Logo tiles and inner panels (quota panel)                  |
| `--radius-xl`      | 1rem     | Cards, modals, pane panels                                 |
| `--radius-2xl`     | 1.125rem | Popovers, menus                                            |
| full               | 9999px   | Pills, switches, dots, avatars, progress bars, tab numbers |

Nested radii step down: an inner panel is one step smaller than the card holding it.

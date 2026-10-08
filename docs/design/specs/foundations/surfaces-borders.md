---
name: surfaces-borders
description: Elevation, translucent chrome, full-border-only rule.
requires: [color]
---

# Surfaces and borders

## The border rule

**A border is either all four sides or absent. Never draw one, two or three sides.**
No `border-l/r/t/b`, `border-x/y`, `border-s/e`, `divide-x/y`, and no `border-left:` etc. in CSS.
Separation comes from surface tone, spacing, or a full border on a rounded container.

| Instead of                               | Use                                                                                  |
|------------------------------------------|--------------------------------------------------------------------------------------|
| Accent stripe on a banner (`border-l-4`) | Full 1px tinted border + tinted fill + leading icon tile                             |
| Divider above a card footer              | Spacing (`mt-1`, `pt-4`) or a `--sunken` footer band                                 |
| Divider between list rows                | One bordered row-card per item, `gap-2` between                                      |
| Header / footer line in a modal          | Footer band on `--canvas`; header on `--surface`                                     |
| Vertical rule between two panes          | Two `--sunken` rounded panels with `gap-4`                                           |
| Sidebar `border-r`, top bar `border-b`   | Translucent chrome over canvas; scrolled state adds `0 8px 24px -12px var(--shadow)` |
| Active nav indicator on the left         | Filled pill: `--accent-50` background, `--accent-ink` text                           |
| Table row lines                          | Row-cards (see `data-rows`), or tone alternation                                     |

Allowed exceptions: SVG strokes in charts and diagrams (axes, baselines, routing map hairlines), a connector line in a
stepper or timeline, and spinners drawn with `border-t-transparent` (prefer SVG).

Lint: `bash docs/design/specs/tools/check-design.sh console/src`

## Elevation

| Level | Surface     | Border                | Shadow                        | Use                                      |
|-------|-------------|-----------------------|-------------------------------|------------------------------------------|
| 0     | `--canvas`  | none                  | none                          | Page background                          |
| 1     | `--surface` | 1px `--line`          | `0 1px 2px rgb(20 20 30/.04)` | Cards, inputs, rows                      |
| 1s    | `--sunken`  | none                  | none                          | Recessed panel inside a card, pane panel |
| 2     | `--surface` | 1px `--accent-border` | `0 10px 28px var(--shadow)`   | Card hover, focus-within                 |
| 3     | `--raised`  | 1px `--line`          | `0 18px 48px var(--shadow)`   | Popover, menu, select list               |
| 4     | `--raised`  | none                  | `0 24px 60px var(--shadow)`   | Modal over a 55% scrim                   |

Hover does not lift cards: border and shadow change only. Press scales to `0.985`.

## Translucent chrome

```css
.chrome{background:var(--chrome);backdrop-filter:blur(20px) saturate(180%)}
@media (prefers-reduced-transparency:reduce){.chrome{background:var(--surface);backdrop-filter:none}}
```

Sidebar and top bar use `.chrome`. Their edge is the colour change against `--canvas`, not a line.

## Dashed borders

Only for "nothing here yet" panels (empty state, add tile, no-quota hint): 1.5px dashed on all four sides,
`--accent-border` or `--line-strong`.

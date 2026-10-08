---
name: cards-and-quota
description: Card anatomy, logo tile, quota meter and its states.
requires: [surfaces-borders, typography-spacing, status-and-banners]
---

# Cards and the quota meter

## Card shell

Level-1 surface, `--radius-xl`, 1px `--line`, no padding on the shell (sections own it). Cards in one grid have equal
height.

**Hover** is one rule on every card surface in the console, whatever its size or page — object cards (key, group, agent,
connection, account, model row-card), KPI tiles, dashboard and usage panels and boards: border `--accent-border`,
`--shadow-hover` (level 2), 150ms, no lift and no travel. A card that carries a state border (a card at its limit) keeps
it on hover: the state is a fact, the hover is only an affordance. A surface that is not a card (a band, a form panel, a
skeleton) wears `flat-surface` and does not react.

**Press**: a clickable card settles to scale `0.985`, 80ms, unless the press began on a control inside it, which keeps
its own feedback.

## Navigation

The whole card is the hit area: clicking anywhere on the card opens it. The title button (`aria-label="Open <name>"`) is
the accessible target and opens the same detail. A click that starts on a control (button, link, input, select,
textarea, summary, or a role button, radio, switch or checkbox) or that ends in a text selection never opens the card;
those controls keep their own jobs.

| Card           | Click anywhere on the card opens                                                                     |
|----------------|------------------------------------------------------------------------------------------------------|
| Connection     | the connection's pane                                                                                |
| Group          | the Preview modal                                                                                    |
| Client key     | the detail modal (the card is one full-card button; the selection checkbox stays a separate control) |
| Agent          | the detail modal                                                                                     |
| Model row-card | the model modal                                                                                      |
| Log row        | the log detail                                                                                       |

Every card click goes through one guard (`isCardOpenClick`); a card that hand-rolls its own click can swallow the
controls inside it.

## Connection card anatomy

```
┌───────────────────────────────────────────┐
│ [logo]  Claude.ai                 ● Ready │  head: padding 18 18 14
│         claude                            │
│ tuyen.nguyen@plugilo.com        ① ②      │  account: name + numbered tabs
│ ┌──────────────────────────────────────┐  │
│ │ 5 hours                  4h 49m left │  │  quota panel (--sunken, r12)
│ │ ▓░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  3%  │  │  reserves four rows
│ │ Weekly                    4d 9h left │  │
│ │ ▓▓▓▓░░░░░░░░░░░░░░░░░░░░░░░░░░  19%  │  │
│ └──────────────────────────────────────┘  │
│ ▬▬▬ 4 of 13 models on      🏷 18 unpriced │  footer: padding 16 18 18
└───────────────────────────────────────────┘
```

- **Logo tile**: 46px, `--radius-lg`, full `--line` border, `--surface`. Only logos mirrored by
  `console/scripts/fetch-provider-logos.mjs` into `console/static/provider-logos/`. Placeholder: provider initial.
- **Title**: name 15px 600, slug `.mono-data` 12px `--muted`. Truncate with ellipsis and `title`.
- **Status pill** top-right: Ready, Near limit, Limit reached, Sign in again, Paused. Derived from the worst window
  across all accounts. A card at its limit gets a `color-mix(in srgb,var(--danger) 30%,var(--line))` full border.
- **Accounts**: every card numbers its accounts, even a single one. The header line names the selected account. Numbers
  are 26px circles; selected is `--accent` filled. An account needing a new sign-in marks its number and replaces the
  quota panel content with "Sign in again" (a button).
- **Quota panel**: `--sunken`, `--radius-lg`, padding 14. Reserves four rows whether windows exist or not, so cards
  align: `min-height: calc(4 * 2.5rem + 3 * 0.75rem)`.
- **Footer**: models bar (56×5px, `--accent` on `--accent-100` track) plus "`x` of `y` models on" (`x` is 600 `--ink`);
  unpriced tag on the right when `n > 0`.

## Quota row

Two lines, 2.5rem tall, 0.75rem between rows.

1. Window name left (13px 500) and time left on the right (`.mono-data` 11.5px `--muted`, "4h 49m left").
2. Bar (7px, full radius, track `--accent-100`) and the percentage (`.mono-data` 12.5px 500, 38px wide, right-aligned).

| Used      | Bar        | Number     | Card status   |
|-----------|------------|------------|---------------|
| under 90% | `--accent` | `--ink`    | Ready         |
| 90–99%    | `--warn`   | `--warn`   | Near limit    |
| 100%      | `--danger` | `--danger` | Limit reached |

Bars with a value under 2% still render a 2% sliver; 0% renders none. Colour is never the only signal: the status pill
says "Near limit" / "Limit reached" with an icon.

Non-window accounts use the first slot area, left aligned:

- **Balance / Spent**: label 12.5px `--muted`, value `.mono-data` 26px 500.
- **No data**: dashed full border, "No quota data yet", one hint line.
- **Missing values** read as a dash, never as zero.

## Unpriced tag

`--warn` text on `--warn-bg`, `--radius-sm`, tag icon, "18 unpriced". It is a button that opens the model prices for
that connection.

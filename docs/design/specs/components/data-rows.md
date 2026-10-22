---
name: data-rows
description: Row-card lists, KPI tiles, meters, charts.
requires: [surfaces-borders, typography-spacing]
---

# Data rows, tables and KPI tiles

No row lines, no `divide-y`. Each row is its own container.

## Row-card list

```
 Provider        Account       Status      Models        Quota          ← header row: transparent, 12px --muted 500
┌────────────────────────────────────────────────────────────────────┐
│ [logo] Claude.ai  you@…   ● Ready    4 of 13    ▓▓░░░ 19%          │  row: --surface, 1px --line, --radius-xl
└────────────────────────────────────────────────────────────────────┘  gap-2 between rows
```

- Row padding 12px 18px, grid columns defined once on the container and reused by header and rows.
- Hover: the shared card hover (`cards-and-quota`): border `--accent-border`, `--shadow-hover` 150ms, no lift. Selected:
  same plus `aria-selected`.
- Below 1000px hide secondary columns; the row stays one line of identity plus status.
- Column text: names 14px 500, ids and numbers `.mono-data` right-aligned, units in `--muted`.
- Sorting: header cell is a button with an arrow icon; `aria-sort` on the column.
- Dense variant (logs, attempts): padding 8px 14px, 13px text. 40–80 visible rows virtualised.

## Modal-backed model row-card (Connections)

Each model is its own card, not a table row: leading enabled switch, identity (name over `.mono-data` id) with status
badges, capability chips and limit pickers, the rate with its source tag, then Edit and an overflow. Row click opens the
model modal. Anatomy and states: `pages/connections`.

Missing is a dash, never zero. Time as relative text with the exact value in a tooltip. Money in the locale currency
with `.mono-data`. A quiet day has no average latency or error rate to claim.

## KPI tile

Level-1 card, `--radius-xl`, padding 16, the shared card hover and no click target. The label row carries the tile's
icon and its label (12.5px `--muted`) on the left and, when the window brought a change, a pill on the right holding
the signed percentage with an arrow icon. The value `.mono-data` 26px 500 sits below, and one line of context (12px;
`--ok` or `--danger` only when the change is a real signal) under it. Six tiles lead the Dashboard: two a row on a
phone, three on a tablet, six on a wide screen. Values are plain numbers: no gradient, no decorative sparkline unless
it carries data.

## Meter and bar chart

- Meter: 7px bar, full radius, `--accent` on `--accent-100` (see `cards-and-quota`).
- Bar chart: bars `--accent-200` for history, `--accent` for the current period, hovered bar updates the readout without
  losing the overall total. Baseline is an SVG stroke. Axis labels `.mono-data` 11.5px.
- Column chart: one column per group on the same 640×84 baseline, the longest in `--accent` and the rest in
  `--accent-200`; the heading and the readouts state the unit.
- Spend list: the costliest connections first — the connection's mark, its name, and what it cost. Every set is
  priced, the free lanes included; they are never charged.

## Donut and heatmap

- Donut: one share per slice on the accent ladder (`--accent`, `--accent-600`, `--accent-400`, `--accent-800`,
  `--accent-200`, `--line-strong` for the tail), drawn on its own square viewBox so no stretched baseline distorts it.
  Hovering a slice or its legend row updates the centre readout without losing the overall total. The legend pairs
  each slice with its share and its figure in the unit the donut counts (money, tokens, or the chosen series).
- Activity calendar: the trend's days folded into a full-width Monday-first month grid, weekday labels across the
  top, the month named where a week opens it, and the day number in each cell, intensity on `--sunken` to
  `--accent` in four steps with a Less-to-More legend. Hovering a cell names the day and its figure; a quiet day
  stays quiet.
- Dashboard activity: the trailing year in the same Monday-first grid and accent ladder, without day numbers — 53
  week columns stretched across the card as 10px-tall rectangles, month abbreviations spanning the weeks they cover,
  and a hover readout. It is hidden below 640px.

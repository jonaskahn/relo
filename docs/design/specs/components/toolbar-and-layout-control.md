---
name: toolbar-and-layout-control
description: Page toolbar and the shared Vertical/Horizontal layout control.
requires: [form-controls, buttons]
---

# Toolbar and layout control

## Toolbar

One row under the page header, wraps on narrow widths. Everything is `--control-h` tall and shares one baseline.

```
[Filter ▾]  [🔍 Search …]   ····   [All models] [☁↓] [▦][☰]
```

Left to right: the filter select — one dropdown holding the page's filter choices, each option carrying its count — and
search; then a flexible gap; then secondary actions, icon actions, layout control. The page's one primary action lives
in the page header. On Connections, Groups and Client keys the filter select and search together span exactly one card
column of the grid below and re-width with the layout control (four, three, two or one per row): the filter select takes
the first third of that column and search the remaining two thirds. The pair eases to the new card width (200ms,
`prefers-reduced-motion` honoured) when the layout control is pressed; a window resize re-solves it without a
transition.

Below 640px: search takes a full row, the layout control hides, and the filter select shares the next row with the
secondary actions.

## Layout control

The same control on Connections, Groups, Client keys and Agents. Two icon buttons in a toggle group: **Vertical** (grid
icon, default) and **Horizontal** (rows icon).

| Mode       | Wide screen        | Card                                                                  |
|------------|--------------------|-----------------------------------------------------------------------|
| Vertical   | four cards per row | Stacked: identity, account, meters, footer                            |
| Horizontal | three wider cards  | Identity and meta on the left half, meters or stats on the right half |

Cards on one page share one size; vertical and horizontal may differ from each other. From small screens up, a grid is
two columns. The choice is remembered in this browser per page. The control sits next to the page's primary list action
(All models, New group, Create key) and above both sections on Agents.

**Motion.** Pressing the control re-solves the grid as one 200ms transform morph (`card-layout-motion`): every card
moves at once, no stagger, and under reduced motion the grid changes instantly. The filter and search pair re-widths in
the same window. A page adds none of its own. The same holds for the control inside a connection's accounts tab and for
the one on Agents.

Grid: `grid-template-columns:repeat(auto-fill,minmax(310px,1fr))` vertical, `minmax(420px,1fr)` horizontal, gap
`--grid-gap`. Optional dashed "Add" tile at the end of the grid fills the last slot (see `status-and-banners` for its
style).

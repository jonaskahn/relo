---
name: dashboard
description: KPI tiles, trend, Routing Universe, recent requests.
requires: [data-rows, status-and-banners, motion]
---

# Dashboard

1. **Six KPI tiles** (see `data-rows`): 24-hour requests, success rate, paid spend, tokens, latency, ready accounts.
2. **Account health and spending**, then the **seven-day trend** (bar chart, `--accent-200` history, `--accent` today).
3. **Lower grid**: the Routing Universe beside recent requests and doctor checks.

## Routing Universe

SVG map with Relo at the centre and up to six configured connections around it, each with its logo (in a 46px tile).
Lines are hairline strokes (`vector-effect:non-scaling-stroke`, `--line-strong`); the accent is spent on the traffic
pulse only. It reflects 24-hour traffic (stroke weight steps, not decoration) and pulses about 600ms on live events;
pulses vanish under reduced motion. With no connections the map area shows explanatory copy and an **Add connection**
action.

## Rules

- The layout stays in place; each slot shows a spinner until its data arrives.
- Empty lists say what is missing; no fictitious data.
- Doctor checks are status rows: pill, check name, result, and the fix when failing.
- Recent requests are dense row-cards linking to the Logs detail modal: fifteen rows, stretching to the height of the
  blocks beside them so both end on one line. The live control is the shared equalizer button the Logs page uses.
- The window, spend-set, period and trend switches are the shared surface segmented control; they morph what they
  govern in place and never slide a pane: trend bars reshape through the existing 400ms geometry transition, the token
  donut glides to its new shares (220ms), readouts and the spend board fade in (120–200ms), with no stagger and no
  travel. Reduced motion swaps instantly.
- The window switch is a header action: right-aligned on the title's row, wrapping to its own right-aligned row inside
  the header when it cannot fit, and scrolling inside its strip on a phone rather than overflowing.
- KPI tiles and boards wear the shared card hover and are not click targets.

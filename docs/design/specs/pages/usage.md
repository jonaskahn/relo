---
name: usage
description: Usage overview and tabs.
requires: [data-rows, form-controls]
---

# Usage

Route `/usage`. A page header (title, subtitle, queried-at caption, refresh action) opens the page; the numbers below
are read fresh on every visit, tab change, filter change and refresh press. The ledger aggregates at write time, so a
read never waits on a rollup job — the caption says how fresh the answer is.

Tabs are icon-and-label triggers (overview, day, connection, model, account, client key) in the shared surface strip
per `form-controls`: a sliding thumb names the active pane, no underline bar. The strip shares one toolbar row with
the filter toggle on its left and the range select on its right, centred between them; the metric picker sits beside
the range select only while the overview is open. On a phone
the strip takes its own full-width row. The panes slide 16px from the travel
direction with a fade; the first paint mounts still. The overview opens directly on its cards with the range chip
above them. Other filter controls (38px) narrow the data.

The overview combines configurable metric cards (KPI tiles per `data-rows`: 12.5px `--muted` label, `.mono-data`
26px value, one context line), at most twelve in rows of six, a 30-day trend, six more charts, and top connection and
model lists. The charts all
read the trend's own days, so no extra read draws them:

- **Trend**: one series at a time (requests, tokens, API spend, errors, latency) on the shared 640×84 baseline;
  history in `--accent-200`, the current day in `--accent`, axis labels `.mono-data` 11.5px.
- **Token mix**: stacked daily columns — input, output, cache read, cache write — with a legend.
- **Reliability**: paired daily columns, errors in `--danger` with an icon and a word wherever else they appear,
  retries beside them, plus the range error rate.
- **Latency**: average-duration columns with the day's slowest request marked as a tick; a quiet day claims neither.
- **Spend share**: a donut of the largest connections by cost with their share of what the list shows. A plan-covered
  connection shows plan usage, a local engine nothing at all — never a dollar figure either of them did not charge.
- **Tokens by connection**: the same donut over the tokens each connection carried, ranked the same way.
- **Activity calendar**: the trend's days as Monday-first week columns, one cell per day on the accent ladder; a quiet
  day stays quiet.

Hovering a bar or column updates the readout without losing the overall total. Tiles and panels wear the shared card
hover and are not click targets.

A grouped tab opens with its window chip and headline (requests, errors, tokens, API spend, plan usage), then the
trend (day) or the comparison charts plus the group's rows. A comparison draws an SVG column chart of the groups by
the selected metric, and then the rows. Rows are row-cards per `data-rows`, spend-ordered: identity
and spend on the first line, a 7px `--accent` on `--accent-100` share meter, and the wrapped counters beneath —
requests, errors (icon plus word, never colour alone), tokens in and out, cache reads and writes, average and slowest
duration, attempts where there were any. Missing is a dash; local-engine spend is a dash, never $0.00.

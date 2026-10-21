---
name: dashboard
description: KPI tiles, trend, Routing Universe, recent requests.
requires: [data-rows, status-and-banners, motion]
---

# Dashboard

1. **One row, two columns**: the **Routing Universe** on the wider half, the **six KPI tiles** (see `data-rows`) on the
   other, read as two by three — 24-hour requests, success rate, paid spend, tokens, latency, ready accounts. Below the
   wide breakpoint each takes the full width, the figures first.
2. **Account health and spending**.
3. **One row, two columns**: the **seven-day trend** (bar chart, `--accent-200` history, `--accent` today), the **token
   mix** and the **doctor checks** stacked in the left column, recent requests filling the right column.

## Routing Universe

A tunnel of the window's traffic: the coding clients that sent the requests on the left rail, Relo in the middle, and
the connections that served them on the right. The card reads the window the page's control selects, so its rails and
its counts answer the same question as the figures beside it. Each rail holds the ten nodes that carried the most
traffic, so both sides fill independently, and the newest request's own two nodes always stand on the rails, so the
live route has somewhere to run. Every request is one line: it runs flat beside the node's name and then sweeps into
Relo, where the lines gather into one bundle. A line's weight steps with the number of requests it carried, and the
name states how many requests reached a node in the window. A name too long for its rail is cut with an ellipsis and
keeps its whole text in the tooltip.

A rail is a map and not a list: a node's place comes from a hash of its id, so the order holds while the traffic moves
under it, and a client that appears for the first time joins the rail in its own place.

The map fills the card it stands in: the card gives the tunnel the height it has left, the rows spread across it, and
the card never falls below the room its rows need. The two rails mirror each other, and a row never lands on its
neighbour — the scattered rows always keep ten pixels of air between them.

Relo is a filled pill, and the lines land on its edge. The newest request's route is drawn over the lines it shares, in
the accent, as a line of travelling dots. The dots run while the live tail runs. They stop when the tail stops, and
under reduced motion the route stays marked as a plain accent line. A new route pulses the connection it reached for
about 600ms. A line that takes more traffic thickens over 200ms instead of jumping, and a node that joins a rail fades
in.

Each node wears the mark of its own side: a coding client wears the harness's own mark, never its vendor's. A wide
card draws the rails side by side and holds ten nodes a side; a phone draws the tunnel upright, the agents above Relo
and the connections below it, and holds five a side, with Relo the spine on the right that both rails run into, so the
card stays a block rather than a long column. With no connections the card shows explanatory copy and an **Add
connection** action; with connections but no traffic in the window it says the window is empty.

## Rules

- The layout stays in place; each slot shows a spinner until its data arrives.
- Empty lists say what is missing; no fictitious data.
- Doctor checks are status rows: pill, check name, result, and the fix when failing.
- Recent requests are dense row-cards linking to the Logs detail modal: nineteen rows, filling the right column of
  the lower row, so both columns end on one line. The live control is the shared equalizer button the Logs page uses.
- The three cards of the left column keep their own heights — the trend's chart and the token mix's donut are compact,
  the doctor's list takes the rest — and the doctor's list scrolls when the card is shorter than its checks.
- The window, spend-set, period and trend switches are the shared surface segmented control; they morph what they
  govern in place and never slide a pane: the tiles and the tunnel read the window they are given, the trend bars
  reshape through the existing 400ms geometry transition, the token donut glides to its new shares (220ms), readouts
  and the spend board fade in (120–200ms), with no stagger and no travel. Reduced motion swaps instantly.
- The window switch is a header action: right-aligned on the title's row, wrapping to its own right-aligned row inside
  the header when it cannot fit, and scrolling inside its strip on a phone rather than overflowing.
- KPI tiles and boards wear the shared card hover and are not click targets.

---
name: dashboard
description: KPI tiles, trend, Routing Universe, recent requests.
requires: [data-rows, status-and-banners, motion]
---

# Dashboard

1. **The attention band**: the accounts that need an operator, read before the figures.
2. **One row of six KPI tiles** (see `data-rows`): requests, success rate, paid spend, tokens, latency, ready accounts.
   Two a row on a phone, three on a tablet, six on a wide screen.
3. **One row, two columns**: the **Routing Universe** on the wider half, the **seven-day trend** (bar chart,
   `--accent-200` history, `--accent` today) and the **token mix** stacked in the other. Below the wide breakpoint each
   takes the full width.
4. **One row, two columns**: **account health** on the wider half, **paid spend** on the other.
5. **One row, two columns**: **recent requests** on the wider half, the **doctor checks** on the other.

## Routing Universe

A tunnel of the window's traffic: the coding clients that sent the requests on the left rail, Relo in the middle, and
the connections that served them on the right. The card reads the window the page's control selects, so its rails and
its counts answer the same question as the figures beside it. Each rail holds the ten nodes that carried the most
traffic, so both sides fill independently, and the newest request's own two nodes always stand on the rails, so the
live route has somewhere to run. Every request is one line: it runs flat beside the node's name and then sweeps into
Relo, where the lines gather into one bundle. A line's weight steps with the number of requests it carried, and the
name states how many requests reached a node in the window. A name too long for its rail is cut with an ellipsis and
keeps its whole text in the tooltip.

A node stands as a pill: its own mark, its name and the number of requests it carried. The rails hug Relo, so the
lines run the space between a rail and the hub, and Relo is a box that states the requests the window held. The map
stands on a recessed panel whose dot grid reads as depth.

Every line is drawn twice: a quiet base that carries the traffic, and an accent layer of dashes that travels along it
while the live tail runs, so the direction of the traffic reads. The newest request's route is drawn over the lines it
shares, in the accent, as a line of travelling dots. The dots run while the live tail runs. They stop when the tail
stops, and under reduced motion the route stays marked as a plain accent line. A new route pulses the connection it
reached for about 600ms. A line that takes more traffic thickens over 200ms instead of jumping, and a node that joins a
rail fades in.

A rail is a map and not a list: a node's place comes from a hash of its id, so the order holds while the traffic moves
under it, and a client that appears for the first time joins the rail in its own place.

The map fills the card it stands in: the card gives the tunnel the height it has left, the rows spread across it, and
the card never falls below the room its rows need. The two rails mirror each other, and a row never lands on its
neighbour — the scattered rows always keep ten pixels of air between them.

Each node wears the mark of its own side: a coding client wears the harness's own mark, never its vendor's. A card
wide enough for two rails draws them side by side and holds ten nodes a side; a card too narrow for that draws the two
rails as lists instead, every node with the share of the rail it carried. With no connections the card shows
explanatory copy and an **Add connection** action; with connections but no traffic in the window it says the window is
empty.

## Rules

- The layout stays in place; each slot shows a spinner until its data arrives.
- Empty lists say what is missing; no fictitious data.
- Doctor checks are status rows: pill, check name, result, and the fix when failing.
- Recent requests are dense row-cards linking to the Logs detail modal: nineteen rows, filling the wider half of the
  last row, so both columns end on one line. The live control is the shared equalizer button the Logs page uses.
- The doctor's list scrolls when the card is shorter than its checks.
- The window, spend-set, period and trend switches are the shared surface segmented control; they morph what they
  govern in place and never slide a pane: the tiles and the tunnel read the window they are given, the trend bars
  reshape through the existing 400ms geometry transition, the token donut glides to its new shares (220ms), readouts
  and the spend board fade in (120–200ms), with no stagger and no travel. Reduced motion swaps instantly.
- The window switch is a header action: right-aligned on the title's row, wrapping to its own right-aligned row inside
  the header when it cannot fit, and scrolling inside its strip on a phone rather than overflowing.
- KPI tiles and boards wear the shared card hover and are not click targets.

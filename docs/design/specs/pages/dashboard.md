---
name: dashboard
description: KPI tiles, trend, Routing Universe, recent requests.
requires: [data-rows, status-and-banners, motion]
---

# Dashboard

1. **The attention band**: the accounts that need an operator on one scrolling row, read before the figures.
2. **One row of six KPI tiles** (see `data-rows`): requests, success rate, paid spend, tokens, latency, ready accounts.
   Two a row on a phone, three on a tablet, six on a wide screen.
3. **One row, two columns**: the **Routing Universe** on the wider two thirds, a narrow third beside it holding the
   **seven-day trend** (bar chart, `--accent-200` history, `--accent` today) on top and the **token mix** below. The two
   charts take the room the map leaves them, so the map's height is the trend's plus the mix's. Below the wide
   breakpoint each takes the full width.
4. **One row, one card**: the **subscription accounts**, the accounts that need an operator, across the whole page.
5. **One card, three blocks**: a **Spend** card that names the window's money, holding the **API spend**, **plan usage**
   and **free usage** blocks as recessed panels of its own.
6. **One row, two columns**: **recent requests** on the wider half, the **doctor checks** on the other. Both stop at half
   the height their rows would take, and scroll inside.

## Routing Universe

A tunnel of the window's traffic: the coding clients that sent the requests on the left rail, Relo in the middle, and
the connections that served them on the right. The card reads the window the page's control selects, so its rails and
its counts answer the same question as the figures beside it. Each rail holds the ten nodes that carried the most
traffic, so both sides fill independently, and the newest request's own two nodes always stand on the rails, so the
live route has somewhere to run. Every request is one line: it runs flat beside the node's name and then sweeps into
Relo, where the lines gather into one bundle. A line's weight steps with the number of requests it carried. A name too
long for its rail is cut with an ellipsis and keeps its whole text in the tooltip.

A node stands as a pill: its own mark, its name and the number of requests it carried. The count wears a chip of its
own, a margin off the name, so the number reads apart from what it counts. The rails hug Relo, so the lines run the
space between a rail and the hub, and Relo is a box that states the requests the window held. The map stands on a
recessed panel whose dot grid reads as depth.

Every line stands still: it carries the traffic, and its weight says how much. The newest request's route is the only
one that moves — it is drawn over the lines it shares, in the accent, as a line of travelling dots. The dots run while
the live tail runs. They stop when the tail stops, and under reduced motion the route stays marked as a plain accent
line. A new route pulses the connection it reached for about 600ms. A line that takes more traffic thickens over 200ms
instead of jumping, and a node that joins a rail fades in.

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

The card reads the window whenever the page reads it, and it says when it last did: a caption states how old the map
is, and an icon action reads the window again in the window the page's control selected. A read keeps the map on
screen, so a refresh shows no spinner and empties no card.

New traffic takes the map's place in place: a node that takes another row slides there, a node that joins a rail fades
in and one that leaves fades out, the counts roll to their new figures, and the lines redraw into the rows they
serve. A window that held no traffic says so over the map it would have drawn, so the card keeps its shape.

## Spend cards

Three blocks answer what the window cost, each a recessed panel inside one card: the **API spend** block for the
connections that pay per token, the **plan usage** block for the ones a plan covers, and the **free usage** block for
the keyless free lanes. All three state money: the free lanes' traffic is priced like any other, it is only never
charged. Each names its three costliest connections, the costliest first, with a `3+` mark when the set holds more. A
connection reads as its mark, its name and what it cost.

## Rules

- The layout stays in place; each slot shows a spinner until its data arrives.
- The routing universe reads the window the page's control selects, states when it last read it, and takes new traffic
  in place: no spinner, no emptied card, no jump.
- Empty lists say what is missing; no fictitious data.
- Doctor checks are status rows: pill, check name, result, and the fix when failing.
- Recent requests are dense row-cards linking to the Logs detail modal: nineteen rows, scrolling inside a card that
  stops at half the height they would take. The live control is the shared equalizer button the Logs page uses.
- The doctor's list scrolls when the card is shorter than its checks.
- The accounts board measures itself, not the viewport: one card a row, two when the board is half a page, three when
  it is a full row.
- The trend names its days under the bars only while the window runs a week or less; a longer window leaves the axis
  unnamed and gives the room to the plot.
- The window, period and trend switches are the shared surface segmented control; they morph what they govern in place
  and never slide a pane: the tiles and the tunnel read the window they are given, the trend bars reshape through the
  existing 400ms geometry transition, the token donut glides to its new shares (220ms), and the readouts fade in
  (120–200ms), with no stagger and no travel. Reduced motion swaps instantly.
- The window switch is a header action: right-aligned on the title's row, wrapping to its own right-aligned row inside
  the header when it cannot fit, and scrolling inside its strip on a phone rather than overflowing.
- KPI tiles and boards wear the shared card hover and are not click targets.

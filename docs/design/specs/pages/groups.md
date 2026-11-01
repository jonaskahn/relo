---
name: groups
description: Group cards, Preview modal, three-step editor.
requires: [toolbar-and-layout-control, cards-and-quota, wizard-and-pickers, modal-and-dialogs]
---

# Groups

A group is the name several models answer, with the rule that picks one of them. Route `/groups`.

Toolbar: the filter select and search share exactly one card column of the grid below — the filter select the first
third, search the remaining two thirds — and re-width with the layout control, which sits at the right edge of the row.
Pressing the layout control re-solves the grid with the shared morph; the page adds no motion of its own. Primary **New
group** (page header). Vertical packs four cards per wide row; horizontal three wider cards. The grid's last cell is the
dashed **New group** tile ("Several models under one name"), which opens the editor.

## Group card

Top to bottom:

1. **Label** (card title, opens the group).
2. **Capabilities** line: Vision or No vision, context size (1M, 800K), Reasoning or No reasoning. A capability the
   members never state is left out.
3. **Id and ordering**, then the **name an agent asks for** in `.mono-data` with a copy button.
4. **State badges** (status pills) and a **stats row**: connections, models, eligible of total members.
5. **Connection logos**: circular 32px logos, each with the model count from that connection at the lower right (16px
   badge). Show up to five logos on More layout and seven on Less; extra connections collapse into one circular overflow
   mark with three dots.
6. **Actions**: **Inspect routing** (tablet and desktop only; hidden below `sm`), Preview and Edit as outline buttons,
   Delete on the right with the same outline icon button in `--danger`. Delete opens an AlertDialog.

Clicking anywhere on the card opens the Preview modal; preview, edit, remove and copy keep their own jobs. The label
button is the accessible target (`aria-label="Open <name>"`).

## Preview modal (`wide`)

Name an agent asks for with copy action; ordering and member count; members in full with rates, weights and account
coverage; and, resolved live from the daemon on open, the ordered candidates a request would read plus the members it
would skip and why (see the resolution path in `wizard-and-pickers`). A name that resolves to nothing reads as an
absence ("No member can serve this name"). Footer hands off to **Edit** or **Delete**; Delete closes the modal first,
then opens the confirmation.

## Routing inspector (`xl`)

The route icon on a group card opens a read-only inspector. It shows the **Agent Harness** box as the request origin,
a dotted routing canvas, the ordered member fan-out, the current/last-served member, the next preview candidate, quota
windows with used percent, window length, and reset time in hours or days. Member cards carry the rank, theme-aware
provider mark, model, state, account coverage, and up to two quota rows; the current member takes the accent border and
the next start takes the accent outline.

The canvas has two responsive layouts, both sized to the modal's width and neither scrolling sideways: wide (≥1024px)
keeps the two capsules on the left and fans smooth cubic Bézier routes to member cards on the right; compact
(640–1023px, and below 640px when the modal is resized there) puts the capsules in a top row and stacks full-width cards
under a curved spine. Long ids and model names truncate with a title. Connectors are drawn beneath the cards: idle
branches use `--line-strong` dashed, the in-use branch is solid `--accent` with an accent arrow, the next branch is
accent dashed, and one moving accent dot follows the in-use path (hidden under reduced motion).

Quota and account failures degrade to `No quota reported` or an unavailable state. Recent requests stay in a
`table-fixed` table below the diagram with Status, Time, Request, Model, Target, and Duration columns; below the
`lg` breakpoint Model and Target collapse into a second line under Request. The table never prints raw attempt-chain
ordinals; the Target cell already names the member that served. Request links use the stored `RequestID` when present
and open the right-side request detail drawer on `/logs?section=agent&request=…`; the query stays in the address so
the deep link remains shareable. The footer offers Refresh, Open latest request's log, and Close. The inspector never
mutates rotation, pins, or random state. Every colour comes from console theme tokens, so the canvas, cards, chips,
quota bars, and legend are correct in light and dark mode.

## Editor (`xl`)

A new group walks four steps — Name it, Billing, Pick members, Preview — while an existing group keeps the
three-step sheet (Name it, Pick members, Preview). Details: `wizard-and-pickers`. Save group is the final primary;
the toast says "Saved group <name>".

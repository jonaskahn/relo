---
name: wizard-and-pickers
description: Stepper, ordering cards, member rows, model picker, resolution path.
requires: [modal-and-dialogs, form-controls, cards-and-quota]
---

# Wizard, member rows and pickers

## Stepper

Three or four steps in a row under the modal header: circle (26px) plus label, connected by a 2px line that fills
`--accent` as steps complete.
Current: `--accent` fill, `--on-accent` number. Done: `--accent-50` fill, accent check, clickable to go back. Upcoming:
`--line` circle, `--faint` label, disabled. Below 640px only the current label shows. The numbers are real order, so
numbered markers are correct here.
Add connection steps: Provider, Connect, Verify, Review. Group editor (new): Name it, Billing, Pick members,
Preview. Group editor (edit): Name it, Pick members, Preview.

## Provider picker (Add connection)

Search on top, focused on open, then grouped provider rows with sentence-case section titles and counts. Each row is a
selectable card (radius card, full border): 32px logo, label 14px 500, an "Added" badge when a connection exists, a kind
pill (Sign in / API key / Cloud / Local), and a `.mono-data` hint with catalog model count. Selecting fills
`--accent-50`, borders `--accent` and shows a check; the footer's Next advances. A provider with no usable path stays
visible with the reason inline in `--warn`. The dashed **Custom endpoint** card closes the list. The models.dev note
sits at the end of the scroll, with Retry when unreachable.

## Verify rail (Add connection)

The resolution path's shape for a checklist: a vertical rail with one node per check (Credential, Model list, Pricing),
dashed connector, ordered badges — spinner while running, `--accent-50` check on pass, `--danger` fill on fail, neutral
dash when skipped. Each node is a card with title, one line of detail and a status word plus icon; a failed card takes
the full `--danger` border and tint and carries its own Retry and fix. A skip states the next action ("add model ids
under Connect"), never success.

## Step "Name it" (group editor)

1. Group name (mono) and Label side by side. The name is what an agent requests; validate letters, numbers, dots,
   dashes, no spaces, inline. Label placeholder is the group name.
2. **Ordering**: five radio cards (priority, round-robin, weighted, cheapest, fastest), each with icon, name and short
   description.
3. **Flow diagram** under the cards: a `--sunken` panel, `--radius-xl`. Left a request chip (`.mono-data`, `--accent`
   fill) with the group name, an arrow, then three sample member nodes; right a sentence of what the rule does. It
   changes with the ordering: priority shows fallback ("if A fails"), round-robin a loop, weighted percentage bars,
   cheapest prices rising, fastest times rising. The sample nodes are illustrative; label them "Member A/B/C".
4. Two toggle cards: Enabled, Listed.

## Step "Billing" (new group only)

Four toggle cards in a two-column grid: **Pay as you go**, **Paid plan**, **Free plan**, **Manual**. Each card carries
its lane icon, its name and the number of configured connections in that lane. Default: the three lanes on, Manual off.
The lanes are multi-select; choosing Manual clears them, choosing any lane clears Manual, and turning all three off
falls back to Manual so the picker is never empty. A card with no connection in its lane is disabled and says so in
words, not colour alone. Footer: `Next` and a secondary `Skip, choose manually` that jumps straight to Pick members
with no filter. The choice lives only in the editor; it never reaches the daemon.

## Step "Pick members"

Two `--sunken` panes, gap 16: **Add models** (left) and **Members** (right, scrolls). The left pane holds a connection
select, an "On only" chip and search above a list that scrolls on its own, with a legend "Price per 1M tokens, in /
out". Added models show a disabled "Added" button with a check. The list paints its first 40 rows, loads the next 40 as
its foot scrolls into view, and a muted line under them names how many of the matches are on screen.

When the group is new, a **billing scope** chip row sits above the Members list and mirrors the Billing step; each
model row also carries its lane chip. **Similar models** sit above the member list: one heading per family, newest
first, each row a chip with logo, name, lane chip and price, plus `Add all (n)` per family. A model whose tools
setting would conflict with the group is left out and one line says how many.

**Member row** (white card on the pane, `--radius-xl`, padding 12):

- Identity line: drag handle, rank circle (priority and cheapest only), logo tile 32px, model name (600) over its id
  (`.mono-data` `--muted`, wraps, never truncated).
- Controls line: price chip (`$4.00 / $20.00`), coverage chip ("1 of 1 accounts"), weight field plus share percent when
  weighted, the on switch, the two move buttons, remove. On a pane wider than about 520px this line sits beside the
  name; narrower, under it.
- A paused member dims to 55%. Ordering decides what is editable: priority has handle and move buttons; cheapest shows a
  note "Sorted by price" and no move buttons; fastest, "Sorted by response time when requests run"; round-robin,
  "Members take turns".
- Drag target gets a full `--accent` border and halo. Moves also work with the buttons, so drag is never required.
- Empty: the empty state with "No members yet".

Footer states the order a request would read, on one line.

## Step "Preview" (resolution path)

Left: summary card (group name, label, ordering, Enabled and Listed pills). Right: vertical path, request chip at the
top, then one row per member: numbered circle on a dashed connector (first filled `--accent`, rest `--accent-100`),
member card with logo, name, and one line of why ("Tried first", "Used if the one before is unavailable", "Takes its
turn", "34% of requests"), rates on the right. First candidate has `--accent` border and `--accent-50` fill. Members the
request would skip follow, dimmed, each with the reason (Paused, No eligible account). If nothing resolves, say "No
member can serve this name", not an empty list.

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
6. **Actions**: Preview and Edit as outline buttons, Delete on the right with the same outline icon button in
   `--danger`. Delete opens an AlertDialog.

Clicking anywhere on the card opens the Preview modal; preview, edit, remove and copy keep their own jobs. The label
button is the accessible target (`aria-label="Open <name>"`).

## Preview modal (`wide`)

Name an agent asks for with copy action; ordering and member count; members in full with rates, weights and account
coverage; and, resolved live from the daemon on open, the ordered candidates a request would read plus the members it
would skip and why (see the resolution path in `wizard-and-pickers`). A name that resolves to nothing reads as an
absence ("No member can serve this name"). Footer hands off to **Edit** or **Delete**; Delete closes the modal first,
then opens the confirmation.

## Editor (`xl`)

The same three-step sheet for new and existing groups, and the same for **New group** and **Edit group**: Name it, Pick
members, Preview. Details: `wizard-and-pickers`. Save group is the final primary; the toast says "Saved group <name>".

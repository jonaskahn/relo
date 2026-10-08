---
name: settings
description: Every daemon choice — console, access, network, providers, daemon, retention.
requires: [form-controls, appearance-popover, cards-and-quota, modal-and-dialogs, status-and-banners]
---

# Settings

Route `/settings`. One page of level-1 cards, one per task: **Console** (theme, accent, quota display, language),
**Access and secrets** (external access; the read-only sign-in and vault), **Network** (bind address, console port,
client protocol ports), **Providers** (model directory, proxy, call wait, retry waits), **Daemon** (start at login, log
level, update feed), and **Retention** (budget and
maintenance). Each card is a level-1 surface with a title, a one-line description, and rows of label, helper text and
control. Cards wear `flat-surface`: a form card is not interactive and does not react to hover. Appearance and language
controls are the same components as `appearance-popover`. Loading keeps one centred spinner in the frame; a failed load
answers one card with the reason and a Retry action.

## Row anatomy

Every row of every card is one grid, `.settings-field`: the label and its helper text hold the left **third** of the
row, the control or the value the two thirds right of it — flush against the card's right edge — from 768px, and the two
areas stack under it so a translated label never squeezes a control. Nothing deviates from that grid:

- A **switch** sits on the value column's right edge, not on the card's left beside its label, so it lines up with the
  inputs, selects and read-only values of the rows above and below it.
- A **read-only value** — the sign-in, the vault, the key file — is plain text or `.mono-data` ending on that same
  right edge, never a disabled input.
- A **group inside one row** keeps its own small grid in the value cell: the three protocol ports of Network, the
  fields of a disclosure body, a row of chips. A group fills the value column and ends on its right edge; only a
  control with a width of its own hugs that edge.
- A **disclosure** is a normal row: the label and its helper text on the left, an **Edit** toggle with a chevron and
  the expanding body in the value cell.
- A **save row**, a **validation message** under its own input, and a card-level **failure** all end on that same right
  edge (`.settings-field-action`), level with the values they belong to.

## Card grid

One column below 1280px. From `xl` the small cards pair and the sections with many settings keep a full row:

| Row | Cards                            |
|-----|----------------------------------|
| 1   | Console (full)                   |
| 2   | Access and secrets + Desktop app |
| 3   | Network + Daemon                 |
| 4   | Providers (full)                 |
| 5   | Retention (full)                 |

Pairing is never forced: a card that holds wide fields keeps its own row, and below `xl` everything stacks because a
paired port or URL row needs the width. Cards in one grid row share their height, and an explicit-save card pins its
save row to the card bottom (`mt-auto` in the card footer), so the two Save buttons of a pair sit on one baseline. Each
card description reserves two lines, so the first row of a pair starts on the same line in both cards. The restart
banner spans the full grid above the cards.

## Save semantics

| Card               | Applies                                                             | Save                               |
|--------------------|---------------------------------------------------------------------|------------------------------------|
| Console            | At once                                                             | — (the pickers apply on selection) |
| Access and secrets | At once                                                             | — (the switch is its own panel)    |
| Network            | At the next daemon start                                            | Explicit **Save**                  |
| Providers          | Proxy and waits at once; model directory at the next start          | Explicit **Save**                  |
| Daemon             | Start at login at once; log level and update feed at the next start | Explicit **Save**                  |
| Retention          | At once                                                             | Explicit **Save**                  |

A switch that applies at once never shares a panel with a form that has an explicit Save; the one switch inside an
explicit-save card (Start at login) is part of that card's form and is written with its Save.

An explicit-save card ends with one Save button, disabled until the draft differs from what is stored, with "Unsaved
changes" beside it while it waits. Success answers with a toast that names the card; a failure stays inline under the
button with its reason, both on the card's value column. The marker fades in with a 6px rise over 180ms, the shared
content swap.

## Restart banner

When a stored value that only applies at boot differs from the one the running daemon booted with — a listener or
protocol port, the bind address, the log level, the update feed, the model directory, the sign-in, or the vault — a
warn banner sits above the cards: `--warn` with an icon, a sentence naming the cause, and one **Restart daemon** action.
The action confirms through the shared restart dialog, posts the daemon restart, and reports through the shared toast.
The banner enters with the shared fade + 6px rise.

## Retention card

Retention edits save with an explicit Save. The three budget values — usage history, maximum events, maximum size — are
rows on the shared grid; the floor rule rides on the usage-history row as its helper text, and a rejected value reports
under its own input. **Preview** asks what the draft would prune without changing a row and reports the count and the
estimated space beside the button; **Clean up now** applies the stored budget and answers as a toast. Both sit with Save
on the value column. The card also carries the maintenance action: **Reclaim space now**. It asks for confirmation first
(AlertDialog) because every captured request and response body is deleted for good. Serving is never stopped: the
stored budget is applied and the stored bodies are deleted while the data plane keeps answering, and the database
compacts itself as soon as traffic pauses. Afterwards the panel shows exactly four facts — days archived, requests
removed, captured bodies removed, database size before and after — revealed with the shared rise; when the compaction
had to wait for quiet traffic it says so, and the maintenance tick retries. Usage totals and request records survive
the cleanup.

The maintenance action sits in a `--sunken` inner panel one radius step below its card, with the sweep's progress and
failure inside the panel, never as toasts.

## Motion

The page adds no entrance motion of its own; it inherits the page switch from `page-motion` and never animates its
cards. The shared controls keep their own motion: the switch and segment transitions (150ms), the popover pickers
(`popover-motion`), the disclosure grid-rows expansion (220ms), the save marker, the restart banner, and the report
reveals (fade + 6px rise, 180ms), plus the confirmation dialog (`modal-motion`). Everything reduces under
`prefers-reduced-motion`.

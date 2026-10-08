---
name: chat
description: Chat workspace.
requires: [app-shell, buttons, form-controls, status-and-banners]
---

# Chat

Lets an operator test a connection, route or model before pointing a real client at Relo. Each turn is one ordinary
request through the same relay a client uses and lands in the log as internal traffic; the daemon keeps no prompt and
no reply. Threads and drafts persist in this browser. **Stop** drops the request and keeps the text already shown, ready
for a retry. On Chat the console rail opens Compact (see `app-shell`).

## Workspace

The page fills the content area under the top bar and owns its own scroll regions; the document does not scroll. Three
regions share the workspace on `--canvas`, with `gap-4` between them and 16px padding on the outer edges.

| Region                  | Treatment                                                                                  | Width |
|-------------------------|--------------------------------------------------------------------------------------------|-------|
| Thread rail             | `--sunken` pane, 1px `--line`, `--radius-card`                                             | 288px |
| Transcript and composer | `--sunken` pane, 1px `--line`, `--radius-card`; messages and composer on `--surface` cards | flex  |
| Target panel            | `--sunken` pane, 1px `--line`, `--radius-card`                                             | 320px |

Each region is one rounded container — header row and scroll region inside it. There are no vertical rules anywhere;
the panes are separated by tone, gap and the full hairline each one carries.

```
┌ top bar (.chrome): Relo / Chat · daemon pill · + · ◐ ─────────────┐
├───────────┬────────────────────────────────────────────┬──────────┤
│ threads   │ thread title   [New chat][threads][target]  │ target   │
│ (sunken)  │ ┌ turn card (--surface) ─────────────────┐  │ (sunken) │
│           │ │ You                     [Answered] pill │  │          │
│ New chat  │ │ ┌ prompt (--sunken inset) ───────────┐  │  │ mode     │
│ search    │ │ └────────────────────────────────────┘  │  │ target   │
│ thread    │ │ answer…                                 │  │ model    │
│ rows      │ │ [copy] [Retry]     meta (.mono-data)    │  │ format   │
│           │ └─────────────────────────────────────────┘  │ stream   │
│           │ ┌ composer (--surface) ──────────────────┐   │ max tok. │
│           │ │ text          [target ▾]   [Send]       │   │          │
│           │ └─────────────────────────────────────────┘   │          │
└───────────┴────────────────────────────────────────────┴──────────┘
```

## Turn row-card

One turn is one level-1 card (`--surface`, 1px `--line`, `--radius-card`, `--card-pad`), gap 24 between turns, the
transcript column capped at a reading width. The card never travels or lifts; it is read, not pressed.

- **Head**: "You" in `--faint` 12px 500 on the left, the answer's status pill on the right — **Answered** (`pass`),
  **Receiving** (`muted`), **Interrupted** (`warn`), **Failed** (`fail`) — one of the shared status pills, so state is
  never colour alone.
- **Prompt**: a `--sunken` inset (`--radius-panel`, one step under the card), 14px `--ink`, preserved line breaks.
- **Answer**: the reply as markdown, page body 15px/1.5, prose at most 72ch. While streaming, the text shown so far in
  plain form. An answered turn that carries no text says so in `--muted` instead of an empty box.
- **Actions and meta** on one row: copy (when there is text to copy) and a labelled **Retry** for the one turn that can
  be retried, then what answered in `.mono-data` `--muted` — connection, model, request id, tokens, duration, and on a
  failure its HTTP status and error code. Identifiers truncate with a tooltip; the meta wraps.

A failed turn keeps its prompt and shows the reason in a danger banner inside the card (`--danger` tint, icon and the
sentence), never as a toast. An interrupted turn keeps the text already shown; with nothing shown it says so. Retry
re-sends the same prompt in place.

## Composer

A level-1 card at the bottom of the transcript column, in normal flow — it never floats over the answer. The textarea
grows with the draft to 200px, then scrolls. Focus-within lifts the card to elevation 2 (border `--accent-border`,
`--shadow-hover`). Enter sends, Shift+Enter opens a line.

The action row shares `--control-h` (38px): the **target chip** on the left (the model or route this turn calls, with
the lock when the conversation keeps its target) opens the target panel; **Send** on the right is the view's one
primary action and pairs an icon with its label. While the answer to this conversation is on its way the same slot
becomes **Stop** (outline, icon and label). Under the card, one `--faint` 12px line states that prompts stay in this
browser.

## Thread rail

- **Head**: "Conversations" 15px 600 `--ink` with the count in `.mono-data`, the overflow menu (Clear all) and the pane
  toggle.
- **New chat**: one outline button, icon and label, full width.
- **Search**: one `--control-h` field filtering titles; the list says when nothing matches.
- **Rows**: two-line row targets — title 14px 500 truncated with a tooltip, when it was last touched in `.mono-data`
  `--faint`. The active row is the filled pill: `--accent-50`, `--accent-ink`, lifted with
  `0 4px 14px var(--accent-glow)`.
  Hover is `--hover`. No left bar. Rename and delete sit in the row's own menu; delete and Clear all confirm with an
  AlertDialog.
- **Grouping**: Today, Yesterday, Previous 7 days, Older — sentence-case 12px `--faint` 500.
- **Empty**: the dashed "nothing here yet" panel (`--accent-border`, `--accent-50`), one line naming what is missing.

## Target panel

"What this test calls", one press from the composer. Every choice applies the moment it is made.

- **Head**: the panel title 15px 600 and the pane toggle.
- **Fields** (`form-controls`): mode (Direct / Client format), target (Provider model / Route), connection, model or
  route, format, system prompt, max tokens, streaming. Labels 12.5px 500; every control shares `--control-h`.
- **Locked**: a conversation that has sent a turn keeps the target it was tested with — every field is disabled and a
  neutral hint card (`--surface` inset, lock icon, one sentence) points at **New chat**. The composer's target chip
  shows the same lock.
- **Nothing to call yet**: one line naming what is missing and a link to Connections — no fictitious target.

## Responsive

The rail docks from 1024px and the target panel from 1280px, both remembered across visits. Below its breakpoint a
pane slides over the transcript as the same rounded pane on a `--scrim` backdrop, one at a time; Escape closes it and
focus returns to the control that opened it. Crossing the breakpoint while a sheet is open docks the layout and
dismisses the sheet.

## Motion

The page adds no motion of its own beyond its panes. It inherits the page switch (`page-motion`), popovers
(`popover-motion`) and the button press (0.985, 80ms). The panes move with Svelte transitions on a 200ms budget: a
docked pane re-solves its width with `slide`, a sheet travels in from its edge with `fly`, and the scrim fades. Under
reduced motion every one of them swaps instantly. No turn card animates in; streaming text appears as it arrives. The
only status feedback is the shared pill, the spinner in a control that is acting, and the daemon dot in the top bar.

## Action contract

| Action                    | In flight                 | Success                         | Failure                         |
|---------------------------|---------------------------|---------------------------------|---------------------------------|
| Send                      | **Send** becomes **Stop** | the turn card fills             | danger banner in the card       |
| Stop                      | the request is dropped    | Interrupted pill, kept text     | —                               |
| Retry                     | the acting control spins  | the turn fills in place         | danger banner stays in the card |
| Copy                      | —                         | the icon reads "check" for 1.5s | an error toast                  |
| New chat                  | —                         | composer takes focus            | —                               |
| Rename, delete, Clear all | confirm first             | the row updates in place        | inline on the row               |

Storage failures stay next to the history they affect: a blocked or unreadable store shows a warn banner above the
transcript with its own action, never a toast.

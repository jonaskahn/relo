---
name: agents
description: Coding client cards and setup.
requires: [toolbar-and-layout-control, cards-and-quota, status-and-banners, modal-and-dialogs]
---

# Agents

Route `/agents` (`/integrations` redirects). Lists supported coding clients with setup state; configured agents above
those not set up. The layout control sits above both sections and re-solves both grids with the shared morph; the page
adds no motion of its own. Clicking anywhere on a card opens the detail modal; Setup, Manage and the action buttons keep
their own jobs, and the title button (`aria-label="Open <name>"`) is the accessible target. Warnings scroll inside the
card; Setup and Manage sit under the body. OpenCode, Claude Code, Claude Desktop, Codex, Pi and OMP carry a **Tested**
pill in the head's right column; every other client carries **Not tested yet** (neutral pill). The detail modal
shows the client key, managed-file status or manual setup steps, model choices and verification. Setup, repair and
rotation report their result before the operator leaves; destructive changes ask for confirmation.

## Page

1. **Page header**: title, subtitle, no primary action — the page's work is per agent, and each card carries its own.
2. **Layout control** (`hidden sm:flex`, right-aligned) above both sections. Vertical is the default: four cards per
   wide
   row; horizontal, three wider. Pressing it re-solves both grids with the shared morph. Nothing on the page animates
   itself.
3. Two grids under **Configured** and **Not configured** headings. A section is hidden when it is empty.
4. An installation with no supported client shows the empty panel (what is missing, one line); no fictitious card.

Both sections render in a **stable order** — the client label, with the id as the tiebreak — so a background re-sync
never reorders a card an operator is looking at.

## Agent card

```
┌───────────────────────────────────────────┐
│ [logo]  Codex                    1M  Test │  head
│         Routes coding agents through…     │
│ Client key                                │
│ ▬▬▬▬▬▬ sk-relo-…c4f1          [Set up]   │  body
└───────────────────────────────────────────┘
```

- **Shell**: level-1 surface, `--radius-xl`, 1px `--line`, `card-press`. Hover is the shared card rule (border
  `--accent-border`, `--shadow-hover`, 150ms); press settles to 0.985 for 80ms. Cards in one grid share a height.
- **Head**: 46px logo tile (full `--line` border, `--surface`), then name 15px 600 over one truncated line of summary
  (12px `--muted`) with the whole sentence in a tooltip. The name is the card's accessible target
  (`aria-label="Open <name>"`); clicking anywhere on the card opens the detail modal through the shared guard.
- **Coverage tags**, in the head's right column over the status pill: `1M` (accent pill, Codex with the million-token
  window on) and the coverage pill — **Tested** (accent pill) or **Not tested yet** (**neutral** pill, `--muted` on
  `--sunken`) — so a long translated label wraps the row instead of colliding with the title. Coverage
  marks Relo's verified wiring for that client, not whether this machine has it set up.
- **Key row**: "Client key" in `--muted`, then the stored hint in `.mono-data`, or a dash when the agent has none.
- **Warnings**: a `max-h-16 overflow-y-auto` box under the key row, one line per fact — a setup refusal (not installed,
  relative path, unrecognised), "Model list changed", "Restart needed". Every line is `--warn` **with an icon and its
  sentence**, never colour alone. An enabled agent with nothing to report shows no box, so the cards stay even.
- **Status pill** under the head: Ready, Drifted, Error, Off, plus the refusal and restart words that outrank them.
- **Action cluster**, `mt-auto`, under the body: not configured → **Set up** primary; configured → Manage, the Codex
  1M toggle, Restart where the client keeps its environment, Repair where the wiring drifted. Remove is the separated
  destructive icon action on the right.

## Detail modal (`wide`)

**One modal for one agent.** Header carries the agent name and its summary line; the body and footer swap per step
(`view`, `result`, `reveal`) and never stack a second modal.

### Step: view

Two panes side by side, stacking under the dock breakpoint with the wiring first.

**Left — the wiring:**

- **Client key**: the hint in a read-only `.mono-data` field, an eye that reads the stored secret on demand, copy while
  revealed. Under it: the env-var note when the client's own file names a reference, else "Relo manages this key". An
  agent with no key says so; a refused read reports the reason beside the hint, never as a toast.
- **Managed files** (clients Relo configures): every file with its `.mono-data` path (truncated, tooltip carries the
  full path) and its state — present, drifted, missing. Drifted and missing pair `--warn`/`--danger` with an icon and a
  word. An agent with no file says "No files written yet".
- **Manual setup steps** (clients Relo does not configure): the daemon's own numbered steps, `.mono-data`, wrapping. A
  client with no steps falls back to the two facts that matter — the base URL and the key hint.
- **Write preview** (before a set-up, or while disabled): each planned file with its path and the fragment Relo would
  write, and a refused write with its reason inline in `--warn` with an icon.

**Right — the test pane**, a `--sunken` panel:

- The models this agent's own key reaches, read in the shape its protocol answers with, plus a reload action.
- While the agent is off, one line says the pane needs a key; it never reads as a failure of the client.
- Filters: search and provider, offering only the providers the listing actually names.
- A model select, and under it the chosen model's window and the connection it came from.
- A prompt, **Send test** (the pane's one primary while nothing is running), and the answer: status and duration in
  `.mono-data`, the text in a scrollable `<pre>`, and any Relo doubts as warn pills. A failed turn stays in the pane
  with its reason.

**Footer** — summary above (name, status pill, coverage tag) and the shared `.modal-footer` below, so the actions
stack full-width with the safe-area inset on a sheet:

| Agent state                 | Primary    | Also                                                    |
|-----------------------------|------------|---------------------------------------------------------|
| not configured, no refusal  | **Set up** | —                                                       |
| not configured, foreign key | **Set up** | — (the overwrite confirm opens as its AlertDialog)      |
| configured, needs repair    | **Repair** | Verify, Rotate, Restore where files exist, 1M (Codex)   |
| configured, healthy         | **Verify** | Rotate, Restore where files exist, 1M (Codex)           |
| —                           | —          | Remove (labeled destructive button, first in the group) |

One primary per view. A blocked Set up stays disabled with its refusal already visible above it. Verification reports
inline in the test pane (`--ok` with an icon on pass, `--danger` with the reason on fail) rather than as a toast.

### Step: result

What an action did, before the operator leaves:

- **Set up** on a client Relo configures itself: "Set up Codex", one line on what Relo wrote, **Done**.
- **Set up** on a manual client: this step is the **reveal** below.
- **Repair** and **Rotate**: one line naming the agent and what changed. Rotation's minted key is shown here, once.
- A failed action stays in this step with the reason and the fix, never as a toast.

The step crossfades in (fade + 6px rise, 180ms) from the shared modal motion; the footer band does not move.

### Step: reveal

A one-time key, shown in the step that minted it:

- The token in a read-only `.mono-data` field with a copy button.
- Under it, the snippet the operator pastes (`.mono-data`, scrollable), or the env-var note when the client's own file
  reads a reference, or the managed-key note when Relo holds the file.
- A line stating that this key cannot be shown again.

Closing the modal clears the token from memory. **Done** returns to the view step; it does not close the modal, so the
operator can still read what Relo wrote.

## Destructive and irreversible actions

Confirmation follows the shared hand-off: the detail modal closes first, then the AlertDialog opens, and the modal
returns directly on the **result** step.

| Action                  | Confirm names                                                                | Result                                      |
|-------------------------|------------------------------------------------------------------------------|---------------------------------------------|
| Remove                  | the agent, and every drifted file the removal will restore from the snapshot | toast, card returns to Not configured       |
| Restore                 | the snapshot the files come back from                                        | toast, files shown restored                 |
| Rotate                  | that the old key stops working                                               | **result** step with the new key shown once |
| Overwrite (foreign key) | whose key is being replaced                                                  | **result** or **reveal** step               |

Each AlertDialog states the consequence and whether it can be undone; the destructive confirm is on the right and takes
default focus last, never the first.

The modal footer's **Remove** is a labeled destructive button (trash icon + label), the first button of the footer
action group, apart from the primary — never a bare trash icon action.

## Action contract

Every action reports its result before the flow ends, and a failure stays next to the control that failed.

| Action     | In flight                                           | Success                      | Failure                         | State              |
|------------|-----------------------------------------------------|------------------------------|---------------------------------|--------------------|
| Set up     | the acting control spins; other cards stay usable   | result or reveal step        | inline on the step              | patched in place   |
| Repair     | the acting control spins                            | result step                  | inline on the step              | patched in place   |
| Rotate     | AlertDialog, then the confirm spins                 | result step with the new key | inline on the step              | patched in place   |
| Remove     | AlertDialog, then the confirm spins                 | toast                        | inline on the card              | patched in place   |
| Restore    | AlertDialog, then the confirm spins                 | toast                        | inline on the card              | patched in place   |
| Restart    | the acting control spins, label reads "Restarting…" | toast                        | inline on the card              | patched in place   |
| 1M context | the acting toggle                                   | toast                        | inline on the card              | patched in place   |
| Verify     | the acting control                                  | inline line in the test pane | inline in the pane with the fix | —                  |
| Reveal key | the eye spins                                       | the field shows the secret   | inline beside the hint          | cached until close |
| Send test  | **Send test** spins                                 | the answer in the pane       | inline in the pane              | —                  |

Only one action is in flight per agent, and its own control carries the spinner while every other control stays live —
one agent's set-up never freezes the page. An action patches its own agent into the list in place; the list keeps its
order, the open step and the modal's scroll, and a silent background re-sync reconciles the daemon's answer without a
skeleton flash.

## Motion

The page adds none of its own. It inherits the page switch from `page-motion`, the card grid re-solve from
`card-layout-motion`, the modal from `modal-motion`, and tooltips from `popover-motion`. One moment belongs to this
page's shared control and lives in `modal-motion`: the step swap inside the detail modal, a fade with a 6px rise over
180ms, which under reduced motion swaps instantly.
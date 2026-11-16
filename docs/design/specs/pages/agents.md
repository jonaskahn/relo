---
name: agent-harness
description: Coding client cards and setup.
requires: [toolbar-and-layout-control, cards-and-quota, status-and-banners, modal-and-dialogs]
---

# Agent Harness

Route `/agent-harness` (`/agents` and `/integrations` redirect). Lists supported coding clients with setup state;
configured agents above those not set up. The layout control sits above both sections and re-solves both grids with the
shared morph; the page adds no motion of its own. Clicking anywhere on a card opens the detail modal; Setup, Manage and
the action buttons keep their own jobs, and the title button (`aria-label="Open <name>"`) is the accessible target.
Warnings scroll inside the card; Setup and Manage sit under the body. OpenCode, Claude Code, Claude Desktop, Codex, Pi
and OMP carry a **Tested**
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
  relative path, unrecognised), or a reason the daemon sent: which file Relo wrote was changed or removed, how many
  models Relo added or stopped listing, or that Codex started before Relo last wrote its model list. Every line is
  `--warn` **with an icon and its sentence**, never colour alone. An enabled agent with nothing to report shows no
  box, so the cards stay even.
- **Status pill** under the head: Ready, Drifted, Error, Off, plus the refusal and restart words that outrank them.
- **Action cluster**, `mt-auto`, under the body: not configured → **Set up** primary; configured → Manage, the Codex
  1M toggle, Reload where the client can reread its configuration, Restart where the client keeps its environment,
  Repair where the wiring drifted. Remove is the separated destructive icon action on the right. OpenCode reloads
  without losing its sessions; its Restart stays available for a startup-environment or key change, and Codex restarts
  after a repair that changed what its app-server would read.

## Detail modal (`wide`)

**One modal for one agent.** Header carries the agent name and its summary line; the body and footer swap per step
(`view`, `result`, `reveal`) and never stack a second modal.

### Step: view

One column, row by row, never split into side-by-side panes. A verification result, when there is one, sits above
the rows as a single status line.

**The mode diagram** (Claude Code and Claude Desktop, while they are set up) is one full-width `--sunken` row directly under the Connection
mode section: a title, the drawing stretched across the row, and one caption sentence that says the same path in words:

- Environment: `Claude Code` → `Relo` → connections. `Anthropic` stands dashed and dim, "not used".
- Proxy: `Claude Code` → `Intercept` → forks. The branch for models bound to Relo goes to `Relo` (chip "to Relo"), the
  rest goes to `Anthropic` (chip "to Anthropic") with the caller's own login, untouched.
- Claude Desktop draws the same two pictures with `Claude Desktop` / "Code tab" as the first node. Environment is its
  gateway profile, which points every tab at Relo. Proxy is its Code tab alone; the caption says Chat and the model
  picker stay Anthropic's.
- Dots travel the lines only while that mode's wiring is applied (proxy: `proxy_state` is `applied`); otherwise the
  lines stand still and the panel says the wiring is not applied. Reduced motion draws the lines alone. While the proxy
  warning is open the panel previews proxy mode, static, and says nothing is written until the operator confirms.
- On a narrow panel the path stacks in a column. Node and chip widths follow the words of the operator's language.

**The wiring rows:**

- **Client key**: the hint in a read-only `.mono-data` field, an eye that reads the stored secret on demand, copy while
  revealed. Under it: the env-var note when the client's own file names a reference, else "Relo manages this key". An
  agent with no key says so; a refused read reports the reason beside the hint, never as a toast.
- **Managed files** (clients Relo configures): every file with its `.mono-data` path (truncated, tooltip carries the
  full path) and its state — present, drifted, missing. Drifted and missing pair `--warn`/`--danger` with an icon and a
  word. An agent with no file says "No files written yet".
- **Why this needs attention** (shown when the agent has a reason): the same lines the card reports, each with the
  whole file path the card truncates, above the managed-files list and beside the Repair or Restart primary.
- **Manual setup steps** (clients Relo does not configure): the daemon's own numbered steps, `.mono-data`, wrapping. A
  client with no steps falls back to the two facts that matter — the base URL and the key hint.
- **Write preview** (before a set-up, or while disabled): each planned file with its path and the fragment Relo would
  write, and a refused write with its reason inline in `--warn` with an icon.
- **Model preferences** (Claude Code only, and only while it is set up): under the wiring, a default-model select, a
  subagent-model select, an effort select, and an auto-compact control. Auto leaves that choice to Claude Code. The
  subagent model is the default for subagents, teammates, and workflow agents that do not name their own; a definition
  or a per-spawn model still wins. The subagent select offers Auto, then the documented aliases best, fable, sonnet,
  opus, haiku, sonnet[1m], opus[1m], and opusplan, then the same models as the default select. A saved value outside
  that list stays selected and is labelled unavailable. Effort and auto-compact stay on the default model. Effort
  offers the levels that model can store, plus a saved level that is no longer offered, and never the session-only
  max. A custom window accepts
  100,000–1,000,000 tokens and reports a bad value beside the control. Save writes inline and does not become the
  modal's primary action. The current model stays selected when it is missing from the list, labelled as unavailable,
  rather than being replaced. A line under the controls says an open Claude Code session keeps the previous choice
  until it is started again. Claude Desktop and every other agent omit this section.

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
inline as a status line above the wiring (`--ok` with an icon on pass, `--danger` with the reason on fail) rather than as a toast.

OpenCode's modal also offers **Reload** and **Restart service**. Reload asks the running server to reread its
configuration; Restart service replaces its startup environment and key, and is the right action when Relo just
minted or rotated a key. A repair reports a completed write with a warning when the reload or restart it needed
failed, so the operator keeps the repaired state and can retry the process action.

OpenCode's setup modal keeps V1 visible but disabled, so an operator can see which legacy format Relo used without
being able to select it. New setup writes V2 only.

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
| Reload     | the acting control spins, label reads "Reloading…"  | toast                        | inline on the card              | patched in place   |
| 1M context | the acting toggle                                   | toast                        | inline on the card              | patched in place   |
| Mode       | the chosen card                                     | the other card reads chosen  | inline in the section           | patched in place   |
| Verify     | the acting control                                  | status line above the wiring | inline, with the fix            | —                  |
| Reveal key | the eye spins                                       | the field shows the secret   | inline beside the hint          | cached until close |

## Connection mode (Claude Code and Claude Desktop)

Claude Code can reach Relo two ways. **Environment** writes Relo's address into the client's own settings;
**Proxy** leaves the client talking to Anthropic and puts Relo's intercept in the path. The mode belongs to the
integration, so one agent is in exactly one mode and switching writes the new mode before retiring the old one.

Before set-up, the modal of Claude Code and of Claude Desktop shows the same two choices above the diagram, so the mode
is picked first and the set-up writes it. Choosing writes nothing: the diagram previews the chosen mode, the proxy
warning stays in view while Proxy is picked, and **Set up** is the confirmation. Every other agent has one way to be wired
and shows no choice. Desktop in proxy mode needs Claude Code in it, and a set-up that cannot have that is refused with
the reason in the result step.

The section sits above the model preferences in the manage modal, and only while the integration is on. Choosing
Proxy asks once in place — not in an AlertDialog — because the warning is about the operator's own Anthropic account
and the choice is reversible from the same control. The warning copy has one owner, the message the section renders,
and the daemon never repeats it. Model preferences are hidden in proxy mode: they belong to the settings file it takes
back out.

### Claude Desktop

Desktop has the same two modes with its own copy. **Environment** is the gateway profile that makes the whole app a Relo
client. **Proxy** keeps Desktop's own login and routes its Code tab: the processes Desktop starts read Claude Code's
settings file, so Desktop rides Claude Code's intercept and writes no file of its own. Entering proxy mode takes the
gateway profile out and puts the previous selection back; leaving it writes the profile again with Desktop's own key.

Two refusals are named rather than worked around, and the daemon's message is shown where the choice was made. Desktop
cannot enter proxy mode while Claude Code is not in it, and Claude Code cannot leave proxy mode, be disabled or be
restored while Desktop uses it. Each client has its own bindings list and presents its own Relo key, and a client that
is not in proxy mode has every request relayed to Anthropic untouched. Desktop's own model picker and the Chat tab are
not routed.

#### Desktop picker (proxy mode, macOS and Windows)

Under Desktop's bindings, one section adds Relo's models to the model picker of Desktop's own Code tab. It is off by
default and is the only control that asks the operating system to trust a certificate, so **Turn on the picker** opens
the risk warning in place and writes nothing until the operator confirms with "I accept the risk, turn it on". The
warning has one owner, the message the section renders: Relo adds a certificate limited to claude.ai to the system trust
store and terminates TLS for claude.ai on this machine; Anthropic's terms may treat that as a violation and an account
can be suspended; Desktop must be quit and reopened and reaches the network only while Relo runs.

The section reads the daemon's reason code as one sentence (off, on, restart needed, trust pending, trust declined, trust
remains, and the refusals), shows the system's trust of the certificate and the certificate's SHA-256, and offers **Turn
off** while it is on. **Remove trust** appears only while the picker is off and the certificate is still trusted, which
is what is left when turning it off could not remove it. On a system Relo cannot trust a certificate on, the section
says so and offers nothing. The one certificate is the one every client shares; there is no per-client certificate.

### Model bindings (proxy mode)

In proxy mode Claude Code keeps its own model names, so the section that replaces the model preferences binds a
native Claude id to the Relo route or model that answers it. A model with no binding still reaches Anthropic with the
caller's own login.

Each binding is a row: the Claude id, the target (a route, or one connection's model), who answers first, and — only
while Relo answers first — what a refusal does. **Relo answers** sends the request to the target in the id's place.
**Anthropic first** asks the vendor and replays the request to the target when the vendor says wait or fails before
answering. A replaced request that Relo cannot answer shows the error unless the binding asks to go back to Anthropic,
so a bound request never spends the operator's vendor quota unasked. A caller's own 4xx is never replayed, and a
request is passed to the other side once.

Ids are matched without the release date, the `[1m]` marker or case, so one binding covers every spelling. The
official ids offered are a seed list, the native ids a Claude connection lists, and the ids already bound; any Claude
id can be bound. A binding whose target was deleted reads **Target is gone** and is left to Anthropic rather than
failing the request. The row states and the caveat line (the picker keeps the Claude name, and the model is told it is
that Claude model) are the only copy the section adds. Bindings are one JSON document per integration in
`app_documents`, loaded into memory at startup and replaced whole on every save, so a request never waits on the
database.

Only one action is in flight per agent, and its own control carries the spinner while every other control stays live —
one agent's set-up never freezes the page. An action patches its own agent into the list in place; the list keeps its
order, the open step and the modal's scroll, and a silent background re-sync reconciles the daemon's answer without a
skeleton flash.

## Motion

The page adds none of its own. It inherits the page switch from `page-motion`, the card grid re-solve from
`card-layout-motion`, the modal from `modal-motion`, and tooltips from `popover-motion`. One moment belongs to this
page's shared control and lives in `modal-motion`: the step swap inside the detail modal, a fade with a 6px rise over
180ms, which under reduced motion swaps instantly.

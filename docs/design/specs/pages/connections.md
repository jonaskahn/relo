---
name: connections
description: Overview, connection pane, models, model modal, All models, Add connection flow.
requires: [cards-and-quota, toolbar-and-layout-control, status-and-banners, wizard-and-pickers, modal-and-dialogs]
---

# Connections

Route `/connections`. Old Models and Providers links redirect here, keeping the selected connection or opening All
models. One surface at a time: the overview grid, one connection's pane, or the combined model list. Clicking anywhere
on a card (the title button is the accessible target) opens that connection's pane, and its account numbers, unpriced
tag and reload keep their own jobs; **Back** returns to the grid and restores filters and scroll.

## Overview

1. **Page header**: title, subtitle ("The accounts, keys and endpoints Relo routes to, and the models each one
   serves."), right: the primary **Add connection**.
2. **Attention strip** when any connection is near or at its limit, or models have no price. A single-line element
   in the toolbar row (full hairline border, no tint): an alert icon and "`N` need attention" (600), then the text
   items in one scrolling row — a 6px tone dot, the name and the reason, e.g. `Claude.ai · 5-hour limit reached,
   resets in 4h 9m` — not filled chips. An item is a button: the connection items open that pane, the unpriced item
   opens its Models tab filtered to Unpriced. It mounts with a horizontal slide that eases the search beside it (parked
   under `prefers-reduced-motion`); hidden when nothing needs attention and below 640px.
3. **Toolbar**: the filter select **All / Needs attention / Unpriced** with counts in its options, search (name, slug,
   account labels), then **All models**, a refresh-prices icon action, and the layout control. Filter and search share
   exactly one card column of the grid below and re-width with the layout control, the filter select taking the first
   third of that column and search the remaining two thirds; below 640px the search takes its own row. Pressing the
   layout control re-solves the grid with the shared morph; the page adds no motion of its own.
4. **Grid** of connection cards (vertical default, four per wide row; horizontal, three wider). The last cell is the
   dashed **Add connection** tile ("Account, API key or endpoint"), which opens the add flow.

An empty installation shows the empty panel: title, one line and the **Add connection** button, no grid.

### Connection card

Top to bottom:

1. **Head**: 46px logo tile, name 15px 600 over slug `.mono-data` 12px `--muted`, status pill on the right. The name
   button is the card's accessible target (`aria-label="Open <name>"`); clicking anywhere on the card opens the pane.
   Hover on the head: the head row scales to 1.02 from its left edge, 150ms, while the content below stays put; reduced
   motion keeps the head still.
2. **Account row**: person icon and the selected account's label, then numbered 26px circles — one per account, selected
   filled `--accent`. An account needing a new sign-in marks its number warn. A connection that never needs one shows a
   single neutral number.
3. **Quota panel** (`--sunken`, `--radius-panel`, padding 14) reserving four rows whether windows exist or not. Window
   rows: name 13px 500 and "4h 49m left" `.mono-data` 11.5px `--muted`; 7px bar on `--accent-100`; percentage
   `.mono-data` 12.5px 500, 38px, right-aligned. Under 90% `--accent`, 90–99% `--warn`, 100% `--danger` — with the
   status pill saying Near limit or Limit reached. Balance and Spent show label and `.mono-data` value; no reading shows
   the dashed "No quota data yet" panel, never a zero. An account that must sign in again replaces the panel with the
   sign-in action.
4. **Footer**: models bar (56×5px, `--accent` on `--accent-100`) with "`x` of `y` models on" (`x` 600 `--ink`), and the
   unpriced tag on the right when `n > 0`.

Status pill: Ready, Near limit, Limit reached, Sign in again, Paused, derived from the worst window across all accounts.
A card at its limit wears a full `color-mix(in srgb,var(--danger) 30%,var(--line))` border. Status is never colour
alone: every pill keeps its icon or word.

## Connection pane

Header: 46px logo tile, name, and (640px and up) the status word with its refresh age, e.g. `Ready (Refreshed 54
minutes ago)`; a stale quota reading shows its clock beside the status, opening Accounts. One toolbar row: **Back**
with a contextual control (left, returns to the grid) — model search with the one
filter menu on the Models tab — the Models / Accounts / Settings strip centred with the card-layout More/Less toggle
beside it on the Accounts tab when accounts exist, and pause/enable (pause icon while on,
play while paused), refresh models and metadata, and **Delete connection** (right, confirms first). One banner directly
under the row when the connection needs one ("Sign in again to resume Claude.ai", "Paused — resume", "No model list
yet — refresh"), each carrying the shortest way out of the state. Tab panels replay a 160ms rise on switch (instant
under reduced motion) without remounting the lists behind them.

Tabs: **Models**, **Accounts**, **Settings**, each with its count where one exists, in the shared surface strip the
usage page uses. Tab content scrolls; the header and
banner stay.

### Models tab

Toolbar, one row that wraps: the bulk-actions menu (turn the shown models on or off, set one context window
across the connection, set a capability across the connection). Search and the one filter menu (category, price
paid/free, status, Unpriced / Overridden / Unavailable with an active-dimension badge and Clear filters) sit in the
pane toolbar beside Back. Active filters mark their control with `--accent-soft`.

**Model row-card** (no table, no row lines — each model is its own container, `--surface`, full 1px `--line`,
`--radius-card`, gap 8px between rows):

```
[switch]  Claude Opus 4.6                         1M ctx · 64K out              [⋯]
          claude/claude-opus-4-6                  tools reasoning
          [Clone of …] [Upstream …] [No price]
```

- Leading: the enabled switch, `aria-label` naming the model.
- Identity: name (truncate ~30ch, title tooltip) over `provider/model` `.mono-data` 12px `--muted`, then a badge row:
  Clone of, Upstream id, Deprecated, Unavailable, coverage ("2 of 3 accounts"). Row click opens the model modal.
- Middle: context and max output as the value on one line with its source mark beside it, and a quiet ⋯ trigger under it
  that opens the preset picker; tools / reasoning / vision as capability chips.
- Price: not on the row — the model modal holds the rates, their source ("Provider" / "models.dev" / "Manual") and the
  editor. An unpriced model reads "No price" in warn tone, never "$0".
- Actions: an overflow menu — View, Edit, Clone. The menu is identical on an enabled and a disabled row.

A paused model dims to 55% except its overflow menu, and disables nothing it can still read; an empty roster says what
is missing ("This connection lists no models yet", "No models match your filters" with Clear filters).

### Model modal (`wide`)

One modal for reading and editing one model. The body swaps content; the header and footer band stay.

**View** — what the model is and what it will do:

- Header: name, `provider/model` `.mono-data`, description and its source.
- Overview tiles (2×2 on wide): category, context window, max input, max output. Capability chips for tools, reasoning,
  vision. Coverage line when accounts are known. Group references.
- Pricing: input and output tiles in `.mono-data` with the source named under each, then "per 1M tokens".
- Overrides: always shown — the fields the operator pinned (name, description, category, context, max input and output,
  capabilities, rates) with their values, otherwise "No overrides — this model follows the provider, then models.dev."
  This is what tells an override apart from the provider's own values without entering the form.
- Sources: the effective value of every field with the layer it came from. Compare sources and Advanced disclosures; the
  **Priced as** reference lives here and re-prices the model immediately.
- Footer: the price summary and the source pill ("Provider" / "models.dev" / "Manual"), then Clone (ghost), Delete
  (danger ghost, opens an AlertDialog after the modal closes), and **Edit** primary.

**Edit** — the same modal, content swapped:

1. **Identity**: display name (the alias an agent sees; empty falls back to the listed name), upstream model id under
   Advanced, enabled switch.
2. **Pricing**: input and output per 1M tokens, the other cache and long-context rates under More rates. The **Priced
   as** reference stays under Advanced in the read view; a manual rate never gets overwritten by a price refresh.
3. **Limits**: context window and max output with presets; each offers the provider and models.dev value as a one-click
   fallback, and clearing removes the override.
4. **Capabilities**: tools, reasoning, vision as Default / On / Off, so "no override" stays distinct from "no".
5. **Advanced**: description, category, delete model.

Footer: the price summary, source pill and an "Overridden" pill when the operator changed anything; ghost **Back**
(returns to View) and primary **Save changes**. Only changed fields are written; a failed save stays in the modal with
the reason; closing with unsaved changes asks the discard AlertDialog.

## Accounts tab

The same compact meters as the cards, with credential state and quota windows. A card's actions are icon buttons:
**Edit** (renames the account), **Override context** and pause/resume on the left, and **Remove** on the right. The
card-layout More/Less toggle sits beside the centred tab strip while accounts exist. Remove is an AlertDialog after the
pane's own dialogs close. Another account is added from **Add connection**, which opens on the connect step with this
connection already chosen. A sign-in is offered only when the account's own status asks for one, never because a model
refresh failed on the network.

## Settings tab

Connection controls and one explicit **Save** (primary). Unsaved changes are indicated next to the button. Destructive
or irreversible settings use the same inline confirm pattern as elsewhere. On a wide pane the sections read as a 2x2
card grid (General, Routing, Upstream waits, Advanced; a sign-in full-width below for a Claude or ChatGPT connection);
a narrow pane keeps the stacked rows. Cards in a grid row share the taller card's height in every state. The sign-in
section carries **Refresh the token automatically** for both, and nothing else — a model that reaches a million tokens
is published at 200K and at 1M whichever connection serves it, and a model whose own name says `1M` is published at that
window alone, so no switch governs it. On a wide pane its switches sit in one row, stacked on a narrow pane.

## All models

One toolbar row, no page header: **Back** left, `All models` with its model-count chip centred, search with the
category, status and label filter menus right. On a phone Back shares the first row with the title and the menus while
search takes its own second row. Below: the same row-card list — logo, name over mono id, connection, on switch, and
the same model modal on click.

## Add connection flow (`xl`)

One modal for provider selection, credential setup, verification and review. The header ("Add connection", one line:
"Pick a provider, prove the credential, then choose what it adds.") and the footer band stay; the body swaps steps.

- **Stepper**: Provider, Connect, Verify, Review — circles joined by 2px bars that fill as steps complete; done steps
  are clickable, upcoming steps disabled. Adding a key to an existing connection and signing in both stop at Verify
  (Provider, Connect, Verify): the key path commits there, the sign-in path ends with **Done**.
- **Footer summary** left: provider logo, label, then pills — path (New connection / Add key / Sign in / Custom
  endpoint), format, and "`N` models found" once a probe answers. Right: ghost Cancel on the first step, ghost Back
  after it, then the step's single primary: **Next**, **Verify connection**, **Continue** / **Add key** / **Done**, or
  **Add connection**.
- Closing with a draft asks the discard AlertDialog; a probe that was not committed is deleted. Never stack modals.

### Step Provider

Search on top, opens focused. Groups of provider rows, sentence-case section titles with their counts. A row is a
selectable card: 32px logo, label 14px 500 with an "Added" badge when a connection already exists, kind pill (Sign in /
API key / Cloud / Local), and a `.mono-data` hint with the models.dev model count. A row with no path shows the reason
inline in warn tone. Selecting marks the card (`--accent` border, `--accent-50` fill, check at the end); **Next**
advances. The last row is the dashed **Custom endpoint** card ("Any OpenAI- or Anthropic-compatible URL"). The catalog
line — "Models from models.dev · fetched 3 hours ago", or unreachable with Retry — sits at the end of the list, and an
empty search offers Custom endpoint.

### Step Connect

One form column. Sign-in methods are radio cards (icon, name, one line); the running state shows the code or link inline
with copy and a ghost Cancel. Key, cloud and custom paths keep every field: label, key, base URL, API format, variables,
headers, deployments (Azure, Vertex), and hand-typed model ids for connections that publish no list. Field errors sit
under their field; the primary focuses the first problem.

### Step Verify

What is being tested as a summary card (provider, connection id, label, base URL, format, credential), then a vertical
rail with three checks — **Credential**, **Model list**, **Pricing**:

- Badge: `--accent` spinner while running, `--accent-50` check on pass, `--danger` fill on fail, neutral dash when
  skipped.
- Card: title, one line of detail (counts and ids in `.mono-data`), status word and icon on the right.
- A failed check takes the full `--danger` border and `--danger`-tinted fill and carries its own Retry, plus the fix
  next to it.

A provider that publishes no list reads "Listing skipped — add model ids under Connect", never as success. The footer
summary shows "`N` models found"; the primary stays disabled until the credential passed.

### Step Review

Two panes: left, the summary card — connection id (editable `.mono-data`), label (editable), provider, base URL, API
format, account, source (listing or manual); right, "Models this connection will serve" — row-cards with the enabled
switch, name, `.mono-data` id, price in/out and source chip, a legend "Price per 1M tokens, in / out", Turn all on/off
and search when the list is long. The add-key path reviews the target connection and the new models it takes.

Footer: logo, label, id, and "`x` of `y` models on"; primary **Add connection** (or **Add key**). A failed commit keeps
the modal open with the reason above the footer.

On success the modal closes, the new connection appears in the grid and its pane opens; the toast names it and its model
count ("Added Claude.ai — 13 models").

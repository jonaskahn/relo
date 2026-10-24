---
name: logs
description: Request, daemon, and startup logs.
requires: [data-rows, modal-and-dialogs, status-and-banners, form-controls]
---

# Logs

Route `/logs`. Three sections behind one segmented switch — **Agent**, **Daemon**, **Startup** — sharing the page header
("Logs" with the subtitle naming all three) and the address (`?section=agent|daemon|startup`, agent when unnamed). A
`?request=` link lands on the agent section and opens its detail.

One toolbar row sits above the panes and stays mounted, so the section thumb springs instead of jumping; only the pane
below it slides. The section switch (Agent, Daemon, Startup, without counts) stays on the left of the toolbar. The open section's own controls sit at the right end of that row. Live — or Refresh on Startup — sits on the title row, at the right. On a phone the switch takes a full-width row, one third per section; the status segment and the daemon filter toggle stand down, so search is the only control under it. Only
the open section mounts; a visit back reads fresh.

The request list and the daemon and startup lists are one bordered panel, a logs exception to the separate row-card
rule: a full border on the panel, hairline rows inside it, and no sideways scroll. Below 640px each request becomes its
own card.

## Agent

The request log. The toolbar holds a search (model, client key, provider, time, over the loaded rows) and an All /
Success / Errors segment. Success and Errors still ask the activity read for `2xx` and `4xx,5xx`. Four compact KPI tiles
summarise the visible rows: requests loaded, success rate, tokens, and average · slowest.

The panel has a sentence-case header and hour bands. A row carries time (exact value in a tooltip), model over provider,
client key over origin, status with icon and code, tokens with a proportional bar, duration (warn tone at 2 minutes or
more), and cost. The client column hides below 900px. Reaching the end of the loaded rows reads the next page, and keeps reading while the end stays in view, until the answer carries no cursor. A failed page shows its error with Retry and does not loop. There is no load button. Live mode subscribes
to the `logs` event stream only while the section is open and reloads once on return.

A row opens a trailing drawer, the logs exception to `CenteredModal`: up to 1040px wide, scrim behind it, previous and
next through the visible rows, Escape and the scrim close it, and focus returns to the row. Below 640px it is a bottom
sheet. The body keeps overview, tokens and cost, the agent exchange and the per-attempt provider exchange.

When the traffic log holds the request whole (the `Full trace` badge), the drawer also offers **Impersonate**: the Chat
tester opens on a new conversation seeded with the captured request — its dialect, system prompt, token limit, stream
choice, and every completed exchange — with the last prompt left in the composer. Nothing is sent until the operator
presses Send. Tools, images and thinking parts cannot travel in a text-only conversation, and the seeded conversation
says which of them was dropped rather than hiding it. An unreadable body lands on the Chat page's empty state with one
line saying so, never as a toast.

## Daemon

The daemon's own JSONL log (`$RELO_HOME/logs/daemon.log`), newest first, with a live tail. One row per line: time
(`.mono-data`, exact value in a tooltip), severity pill (the daemon's own word — info, warning, error — with the tone
to match, never colour alone), the message sentence, and the flattened fields under it (an HTTP access line reads
`METHOD path · status · duration`). A disclosure opens the full message and fields with a copy action.

- **Controls**: level floor (all levels, warnings and errors, errors only) and an HTTP-requests selector (hidden or
  shown), both selection boxes behind the toolbar filter toggle and their own storage key; live/pause in the page toolbar.
  Requests
  hide by default: the access lines of every poll would bury the events the section exists for.
- **Paging**: scrolling to the oldest line continues from there; the live tail polls newer lines every 2s and
  merges them by byte offset, so a line never shows twice. A failed poll stays silent and retries; a failed load
  answers inline with the reason and a Retry action, never as a toast.
- **Empty**: "The daemon has not written a log yet." Missing is a dash; the section never invents rows.

## Startup

One row per boot transcript (`startup-*.log`), newest first. A row carries the outcome pill (`Started` settled,
`Failed` with icon and word), the start time, the failing line beside it when there is one, and a `Current` pill on
the transcript of this run. Opening a row reads its transcript in place with the same line component as the daemon
section, oldest first; an empty transcript says so. A footer line notes boot transcripts are kept for 14 days. The
Refresh control lives in the page toolbar, right of the section switch.

## Backend

Three read-only management routes serve the page; all answers are `{items: [...]}` shaped and `Cache-Control:
no-store`:

- `GET /api/v1/logs/daemon?limit=&before=&after=&level=&hide_requests=` — one page of parsed lines, newest first.
  `before` pages backwards from a byte offset, `after` tails newer lines past one; the answer carries `next_cursor`
  and the `end` offset the next tail passes back.
- `GET /api/v1/logs/startups` — the transcripts, newest first, with outcome, failing line, size and the current
  marker.
- `GET /api/v1/logs/startups/{name}` — one transcript in full. The name is a file name, never a path; anything else
  is a 400 and a missing transcript a 404.

The reads live in `application/status` beside the home directory it already holds; the handlers map them to DTOs the
way the activity reads do. Rotated daemon archives (`.gz`) stay out: the section reads the current log.

## Motion

A row that arrives after the list is already showing uses the shared slide, and flip eases the rows it pushes down. The first page stays still. The request drawer slides in from the trailing edge and back out the same way. Otherwise the page adds no motion of its own. Sections slide 16px from the travel direction with a fade like the usage panes;
the shared live control pumps like a music wave while the tail runs; disclosures, pills and the filter fields keep their
shared motion. Everything honours `prefers-reduced-motion`.

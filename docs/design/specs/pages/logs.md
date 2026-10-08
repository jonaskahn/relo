---
name: logs
description: Request, daemon, and startup logs.
requires: [data-rows, modal-and-dialogs, status-and-banners, form-controls]
---

# Logs

Route `/logs`. Three sections behind one segmented switch — **Agent**, **Daemon**, **Startup** — sharing the page header
("Logs" with the subtitle naming all three) and the address (`?section=agent|daemon|startup`, agent when unnamed). A
`?request=` link lands on the agent section and opens its detail.

One fixed toolbar row sits above the panes: the filter toggle on the left, the section switch (one surface strip per
`form-controls` with a thumb that slides under the active one) centred, and the live control — or Refresh on Startup —
on the right. The row stays mounted across sections, so the thumb springs instead of jumping; only the pane below it
slides. On a phone the strip takes a full-width second row. The fields the toggle opens render inside the pane, under
its own storage key. Only the open section mounts; a visit back reads fresh.

## Agent

The request log: filters (provider, status class, origin, coding client, access key, all selection boxes) in the pane's
fields grid, the dense row-card list, `Load more` paging, and the `wide` CenteredModal with
overview, tokens and cost, the agent exchange and the per-attempt provider exchange. Live mode subscribes to the `logs`
event stream only while the section is open and reloads once on return, so a burst of announcements still lands as one
read.

Rows are dense row-cards per `data-rows` (8px 14px, 13px text): a transparent sentence-case header row, then one card
per request — time (exact value in a tooltip), model, provider, origin, client key, status with icon and code, tokens,
duration, cost. Secondary columns hide below wide screens instead of scrolling sideways; identity, status and cost
survive every width. The whole card opens the detail modal.

## Daemon

The daemon's own JSONL log (`$RELO_HOME/logs/daemon.log`), newest first, with a live tail. One row per line: time
(`.mono-data`, exact value in a tooltip), severity pill (the daemon's own word — info, warning, error — with the tone
to match, never colour alone), the message sentence, and the flattened fields under it (an HTTP access line reads
`METHOD path · status · duration`). A disclosure opens the full message and fields with a copy action.

- **Controls**: level floor (all levels, warnings and errors, errors only) and an HTTP-requests selector (hidden or
  shown), both selection boxes in the pane's fields grid behind their own storage key; live/pause in the page toolbar.
  Requests
  hide by default: the access lines of every poll would bury the events the section exists for.
- **Paging**: `Load older` continues from the oldest line on screen; the live tail polls newer lines every 2s and
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

The page adds no motion of its own. Sections slide 16px from the travel direction with a fade like the usage panes;
the shared live control pumps like a music wave while the tail runs; disclosures, pills and the filter fields keep their
shared motion. Everything honours `prefers-reduced-motion`.

---
name: login-and-errors
description: Sign-in and error pages.
requires: [form-controls, status-and-banners]
---

# Login and error pages

## Login

Shown when `[admin] login = true`, and for a forwarded request while `[admin] allow_external = true` even with the
sign-in off: a centred card (`--radius-xl`, level 1 over `--canvas`) with the Relo mark, an admin-token field (mono,
show/hide), a visible error line and a submit button with a loading state. The appearance popover is available before
signing in. Success returns to the intended route.

## Missing and error pages

The server returns the shell for every address, so the console answers unknown pages. An error page drops the chrome: no
rail, top bar or command palette. It offers the one action that helps: sign in for 401, home for 404 or 403, try again
for a daemon failure. The sign-in carries the address the operator came from; an unauthenticated visitor sees the error
rather than a redirect.

No page carries a heading. Each status has a mark: the same doorway, drawn like the routing map (hairline non-scaling
strokes over `--line-strong` and `--surface`, the accent used once). Missing = a door open onto nothing (a handle, no
room). Session ended = the same door with a padlock. May not open = barred across. Daemon fault = a fault across the
opening. Under the mark: the status, the requested address in `.mono-data`, and the action.

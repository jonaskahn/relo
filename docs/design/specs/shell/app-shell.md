---
name: app-shell
description: Sidebar, top bar, scroll regions, page header.
requires: [surfaces-borders, color]
---

# App shell

`100dvh`. Document scroll is off at `html/body`. Two independent scroll regions:

1. **Sidebar nav.** Fixed header (logo and branding) and fixed footer (GitHub and Buy me a coffee on the left, Manage
   daemon on the right). Only the nav list scrolls.
2. **Main content.** `flex-1 overflow-y-auto`.

```
┌ sidebar (.chrome) ┐┌ top bar (.chrome, sticky) ───────────────────────────────┐
│ logo              ││ ☰  Relo / Connections        ● Daemon running 0.1.0  + ◐ │
│ nav (scrolls)     │├──────────────────────────────────────────────────────────┤
│ ─ footer ─        ││ main (scrolls): page header, content                     │
└───────────────────┘└──────────────────────────────────────────────────────────┘
```

No lines between regions: the chrome tint against `--canvas`, plus a soft right-edge shadow on the rail
(`8px 0 24px -12px var(--shadow)`), is the edge (see `surfaces-borders`).

## Sidebar

- **Compact** (default): 64px icon rail, always visible. **Full**: 224px rail at 1024px and wider, icons plus names,
  pushes content.
- Below 1024px, or on a page that asks for the overlay, the menu opens over the content at full width from the top bar.
- The mode lives in this browser and is switched from the top bar. On Chat a saved Full opens as Compact; the top bar
  can expand it for that visit.
- Nav item: 38px high, `--radius`, icon plus label. Active = `--accent-50` fill, `--accent-ink` text and icon, lifted
  with `0 4px 14px var(--accent-glow)`. Hover = `--hover`. No left bar.
- Manage daemon: hover names it; click opens a compact dialog with three circular actions (restart, shut down, force
  restart). Sign-out sits under that row when login is on.

## Top bar

- 60px, `.chrome`, breadcrumb on the left (`Relo / Page`, last segment `--ink` 600, earlier ones `--muted`).
- Right cluster, 38px controls, 8px gap: daemon pill, quick add `+`, appearance button (opens `appearance-popover`).
- Daemon pill: `--surface`, full `--line` border, `--radius-control` corners (matches the buttons beside it), 8px dot
  (`--ok`, soft ring pulse while running), text "Daemon running" in sentence case, version in `.mono-data` `--muted`.
  Stopped = `--faint` dot, no pulse, text says what is wrong and offers the next action.
- Scrolled content under it adds `0 8px 24px -12px var(--shadow)`.

## Page header

Inside `.wrap` (max 1360px): title left (page title style) with one-line subtitle in `--muted` (max 56ch) below it; page
actions right, on the title's row, primary action last and the only filled button. Secondary meta (for example "Prices
from models.dev · fetched 3 hours ago") sits beside the actions in `--muted` 12.5px. Below 640px the subtitle hides,
leaving the title and its actions on one line. The header band holds title, subtitle and page
actions; a page-wide control (the dashboard's window switch) may sit in the actions, while search and filters belong
to the toolbar below it.

## Pages without chrome

Error pages drop the rail, top bar and command palette (see `pages/login-and-errors`).

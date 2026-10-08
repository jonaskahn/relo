---
name: form-controls
description: Inputs, search, select, switch, chips, toggle group, radio cards, tabs.
requires: [color, typography-spacing, buttons]
---

# Form controls

All controls are `--control-h` tall with `--radius` corners, except switches and pills.

## Field

Label above (13px, 500). Control. Hint below (12px `--muted`). Error replaces the hint: icon plus text in `--danger`,
control border `--danger`. Optional fields say "optional" after the label in `--muted`; required ones are the default.

## Input, search, select

- Input: `--surface`, full 1px `--line-strong`, shadow-1. Focus: border `--accent`, halo `0 0 0 3px var(--accent-100)`.
- Ids, slugs, keys: add `.mono-data`.
- Search: leading 16px search icon at 12px inset, left padding 36px. Width `min(280px,100%)`. Placeholder "Search …"
  describes the scope.
- Select (Bits UI): same box, trailing chevron, list is level 3. Unavailable options stay visible with a reason.

## Switch

40×24 track (34×20 small), knob 18px white with shadow. Off: `--line-strong` track. On: `--accent`. `role="switch"` with
`aria-label` naming what it turns on. Disabled while saving. A switch applies immediately or sits in a form with an
explicit Save; never mixed in one panel.

## Filter chip

Pill, `--control-h`, 14px side padding, full `--line` border, `--muted` text, count in 12px at 70% opacity. Selected:
`--accent` fill and `--on-accent` text. Chips are a filter group: `aria-pressed`, one clear way to reset (the first
chip, "All").

## Filter row

One `1fr auto 1fr` row: the **Filter** toggle (funnel, label, active-count badge, chevron) left, a tab strip centred,
and the page's own control right (live on Logs, range plus metric picker on Usage). The strip is centred by the two
`1fr` side columns, not by its own width. Below 640px the sides share the first row and the strip takes a full-width
second row. The fields behind the toggle drop under the row under their own storage key, so a toggle that is open
stays open across visits. On a page whose panes swap (Logs), the row sits outside the panes: it never remounts, so
the thumb springs instead of jumping.

## Toggle group (view and mode switches)

One control per `motion`: a `--surface` strip with the button radius (`--radius-control`), a full `--line` border,
`--control-h` tall (44px on touch) and a soft shadow, holding a thumb that slides under the active segment
(`--accent-50` fill, `--accent-border`). Segments are muted 13px text with icons; the active one is `--accent-ink` at
550 weight. `aria-pressed`, group has `role="group"` and a label. The strip scrolls instead of wrapping, and a strip
that overflows brings the active segment back into view. Use for layout and view; use tabs for content panes.

## Radio card

Used for choices with explanation (ordering, theme). Full 1px `--line` border, `--radius-lg`, 14px padding, icon tile
(34px, `--sunken`), title 600, description 12px `--muted`. Selected: `--accent` border, `--accent-50` fill, 3px
`--accent-100` halo, icon tile becomes `--accent` with `--on-accent` icon. `role="radiogroup"` with `role="radio"`
cards. 5 across on wide screens, 2 across below 760px.

## Toggle card

Switch with a title and a one-line description in a full-border `--radius-lg` card. Two side by side (Enabled, Listed).

## Tabs

Bits UI Tabs for content panes (Models, Accounts, Settings). The tablist is the same surface strip as the toggle
group: button radius, full `--line` border, sliding thumb, icon plus label per trigger, active is `--accent-ink`
at 550 weight, no underline bar. Swapping panes slides the new pane 16px from the travel direction with a fade
(`tab-motion`); the first paint mounts still. Numbered account tabs are 26px circles: selected filled `--accent`; an
account needing sign-in marks its number with a `--danger` dot.

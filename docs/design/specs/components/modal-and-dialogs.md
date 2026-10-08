---
name: modal-and-dialogs
description: CenteredModal, AlertDialog, popovers, sheets.
requires: [surfaces-borders, buttons, motion]
---

# Modals and dialogs

## CenteredModal

Reusable overlay for ordinary surfaces. Bits UI Dialog; `interactOutsideBehavior` on Content (not
`closeOnOutsideClick`); sticky header and footer, independently scrolling body; focus trap; Escape dismisses when
dismissible; `replaceState` query-param sync.

| Size       | Max width           | Use                                                     |
|------------|---------------------|---------------------------------------------------------|
| `compact`  | `max-w-lg` (32rem)  | Small forms                                             |
| `standard` | `max-w-2xl` (42rem) | Moderate detail views                                   |
| `wide`     | `max-w-4xl` (56rem) | Provider and log details, Preview, Add connection       |
| `xl`       | `max-w-6xl` (72rem) | Workspace editors with independent panes (group editor) |

Look: level 4, `--radius-xl`, scrim `rgb(10 10 14 / .55)`. Height `min(780px, 100%)` for workspace modals.

```
┌ header  (--surface): title 18/600, one-line description --muted, close icon (32px) ┐
│ stepper (optional)                                                                │
│ body    (--surface, scrolls)  or two --sunken panes with gap-4                    │
└ footer  (--canvas band): summary left, actions right                              ┘
```

No rules between header, body and footer. The footer band and spacing separate them.

- **Footer summary**: object name (600, truncated) plus pills: ordering, "`x` of `y` eligible" (warn tone when 0 of n).
  Actions: Cancel or Back (ghost), then the primary.
- **Destructive footer action**: a delete, revoke or remove in a footer is a labeled destructive button (icon + label),
  never a bare trash icon action, and it leads the action group. It stays clear of the primary's end of the row.
- **Multi-step flows** (Add connection, the model modal) swap the content of the same modal. Never open a second modal
  on top.
- **View ↔ edit swap**: a read view whose footer carries one primary **Edit** swaps the body to the edit form while
  header, footer band and summary stay. The ghost action next to the primary returns to the view. Closing with unsaved
  changes asks the discard AlertDialog; a destructive action closes the modal first, then opens its AlertDialog.
- **Workspace modal** (`xl`) takes a definite height and passes scrolling to its panes through `bodyClass`, so picker
  and list scroll apart.
- **Phone**: below 768px the modal becomes a bottom sheet that slides up; two-pane layouts switch panes through a
  two-button toggle group (Members, Add) so the list fills the sheet.
- Closing from a modal that hands off to a confirmation closes the modal first, then opens the AlertDialog.

## AlertDialog

Bits UI AlertDialog, `compact`. Only for destructive or irreversible actions; never inside CenteredModal.
Title names the action and the object ("Delete group fast?"). One sentence states the consequence and whether it can be
undone. Buttons: Cancel (secondary, default focus) and the destructive confirm (`--danger` fill, white text, verb
matching the trigger). Prefer reversible alternatives (pause, disable) and offer them first.

## Popover and menu

Level 3, `--radius-2xl`, padding 18, no arrow. Opens on click, not hover. Closes on Escape and outside click and returns
focus. Menu items are 38px, `--radius`, `--hover` on hover. Panels are portaled and mounted with `forceMount`, and open
and close with the shared popover motion (`popover-motion`); sizing and keyboard rules are unchanged.

## One-time secrets

A new client key is shown once in a read-only mono field with a copy button. Closing the modal clears it from memory;
the copy explains that it cannot be shown again.

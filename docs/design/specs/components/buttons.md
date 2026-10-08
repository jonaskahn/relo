---
name: buttons
description: Variants, sizes, states, icon-only and destructive buttons.
requires: [color, typography-spacing]
---

# Buttons

Action buttons pair an icon with a visible label. Icon-only buttons are for compact toolbar and row actions and always
carry `aria-label` and a tooltip. Chips, toggle groups and card triggers are not action buttons (see `form-controls`).

| Variant     | Look                                                                       | Use                                   |
|-------------|----------------------------------------------------------------------------|---------------------------------------|
| Primary     | `--accent` fill, `--on-accent` text, `0 4px 14px var(--accent-glow)`       | The one main action in a view or step |
| Secondary   | `--surface`, full 1px `--line` border, `--ink`                             | Everything else with a label          |
| Ghost       | No fill or border, `--muted` text                                          | Cancel, Back, low emphasis            |
| Destructive | Secondary shape, `--danger` text and icon; confirm dialog fills `--danger` | Delete, revoke, remove                |
| Icon        | Square `--control-h`, secondary look, 16–18px icon                         | Refresh, layout, menu, quick add      |

- Height `--control-h` (38px, 44px on touch). Dense row actions use `--control-h-sm` (32px), desktop only.
- Padding 0 1rem, gap 0.5rem, `--radius`, weight 500, 14px. Icon is 14–16px, 2px stroke, from `Icon.svelte`.
- One primary per view. In a wizard footer the primary is Next, then Save group on the last step.
- Hover: secondary gets `--hover`; primary darkens to `--accent-600`. Press: scale 0.985. Disabled: opacity 0.4, no
  pointer events, and explain why nearby if it is not obvious.
- Loading: replace the icon with a spinner, keep the width, keep the label. The button is disabled while loading.
- Destructive icon buttons use the same outline icon button as Preview and Edit, with `--danger` colour. They belong to
  cards and row actions; a destructive action in a **modal footer** is a labeled destructive button, first in the group.
- Labels are verbs and keep one name through a flow ("Save group" then toast "Group saved").
- Never render a link as a button or the reverse; navigation is an anchor.

```svelte
<Button variant="primary"><Icon name="plus" size={16} />{$_('connections.add')}</Button>
<Button variant="icon" aria-label={$_('common.refresh')} title={$_('common.refresh')}><Icon name="cloud-download" /></Button>
```
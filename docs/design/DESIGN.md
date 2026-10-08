---
name: "Relo — Spatial Control Surface"
description: "Relo design system: neutral layered surfaces, translucent chrome, one selectable accent, full borders only."
version: "3.2"
updated: "2026-10-05"
---

# Relo design system

## Direction

**Spatial Control Surface.** Neutral layered surfaces, translucent chrome, rounded containers with full hairline
borders, and one operator-chosen accent. Scales from laptop to wide monitor without becoming a shrunken table or a
stretched list. Every element earns its place; complexity lives one level deeper.

## Rules

1. **No one-sided borders.** A border is all four sides or absent. Separate by tone, spacing or row-cards.
2. **Tokens only.** No hex, `rgb()` or Tailwind palette colours in components.
3. **The accent is selectable.** Soft fills use `--accent-ink` text, filled accent uses `--on-accent`, accent text on a
   surface uses `--accent-700`.
4. **Status is never colour alone.** Warn and danger pair fixed semantic colours with text and an icon.
5. **One control height.** Controls in a row share `--control-h` (38px, 44px on touch).
6. **Sentence case.** No uppercase, no tracked labels.
7. **22 locales.** Every copy key exists in all of them. Truncate only identifiers, with a tooltip.
8. **One primary action per view.** Actions pair icon and label; icon-only controls carry `aria-label` and a tooltip.
9. **Never stack modals.** Steps swap the content of one modal; destructive actions use AlertDialog.
10. **Real data.** Missing is a dash; empty lists say what is missing.
11. **Restrained motion.** Two ambient moments: the daemon dot and the live-tail equalizer, each only while its own
    state runs. Hover feedback is never travel (connection card head zooms to 1.02 in place). Everything honours
    `prefers-reduced-motion`.
12. **Local assets.** System font for UI, JetBrains Mono for data, Tabler paths in `Icon.svelte`, mirrored provider
    logos, no new dependencies without a concrete need.

Check (from the repo root): `bash docs/design/specs/tools/check-design.sh console/src`

## Quick reference

| Need                                   | Token                                                                   |
|----------------------------------------|-------------------------------------------------------------------------|
| Page / card / popover / recessed panel | `--canvas` / `--surface` / `--raised` / `--sunken`                      |
| Text: strong / secondary / decoration  | `--ink` / `--muted` / `--faint`                                         |
| Border: standard / strong / selected   | `--line` / `--line-strong` / `--accent-border`                          |
| Accent: fill / soft fill / ink / ring  | `--accent` / `--accent-50` / `--accent-ink` / `--accent-100`            |
| Semantic                               | `--ok` `--warn` `--danger`                                              |
| Radius                                 | control 10px, panel 12px, card 16px, popover 18px, pill full            |
| Control height                         | `--control-h` 38px (44px coarse), `--control-h-sm` 32px                 |
| Type                                   | system-ui; data in `.mono-data`; page 15px, controls 14px, minimum 12px |
| Layout                                 | gutter 28px, grid gap 18px, card padding 18px, max width 1360px         |

## Where to look

| Building                       | See                                                                   |
|--------------------------------|-----------------------------------------------------------------------|
| Card or grid page              | `surfaces-borders`, `cards-and-quota`, `toolbar-and-layout-control`   |
| Modal, editor, multi-step flow | `modal-and-dialogs`, `wizard-and-pickers`, `form-controls`, `buttons` |
| Form or settings row           | `form-controls`, `buttons`, `status-and-banners`                      |
| Table, list, KPI               | `data-rows`, `surfaces-borders`                                       |
| Theme, accent, language        | `color`, `appearance-popover`                                         |
| Copy                           | `i18n-a11y`, `typography-spacing`                                     |
| A page                         | its file in `./specs/pages/`                                          |

## Files

<!-- file-map:start -->

### Foundations

| File                                        | Covers                                                                       |
|---------------------------------------------|------------------------------------------------------------------------------|
| `./specs/foundations/color.md`              | Theme, accent and semantic colours; accent scale derivation; contrast rules. |
| `./specs/foundations/i18n-a11y.md`          | Languages, label expansion, keyboard, focus, status rules.                   |
| `./specs/foundations/metrics.md`            | What every metric means on every surface — dashboard, usage page, tray rows. |
| `./specs/foundations/motion.md`             | Durations, easing, reduced-motion behaviour.                                 |
| `./specs/foundations/surfaces-borders.md`   | Elevation, translucent chrome, full-border-only rule.                        |
| `./specs/foundations/typography-spacing.md` | Type scale, spacing, control height, radius.                                 |

### Shell

| File                                  | Covers                                                       |
|---------------------------------------|--------------------------------------------------------------|
| `./specs/shell/app-shell.md`          | Sidebar, top bar, scroll regions, page header.               |
| `./specs/shell/appearance-popover.md` | Theme, accent and language switcher and its data attributes. |

### Components

| File                                               | Covers                                                                  |
|----------------------------------------------------|-------------------------------------------------------------------------|
| `./specs/components/buttons.md`                    | Variants, sizes, states, icon-only and destructive buttons.             |
| `./specs/components/cards-and-quota.md`            | Card anatomy, logo tile, quota meter and its states.                    |
| `./specs/components/data-rows.md`                  | Row-card lists, KPI tiles, meters, charts.                              |
| `./specs/components/form-controls.md`              | Inputs, search, select, switch, chips, toggle group, radio cards, tabs. |
| `./specs/components/modal-and-dialogs.md`          | CenteredModal, AlertDialog, popovers, sheets.                           |
| `./specs/components/status-and-banners.md`         | Status pills, banners, empty states, toasts, loading.                   |
| `./specs/components/toolbar-and-layout-control.md` | Page toolbar and the shared Vertical/Horizontal layout control.         |
| `./specs/components/wizard-and-pickers.md`         | Stepper, ordering cards, member rows, model picker, resolution path.    |

### Pages

| File                                | Covers                                                                           |
|-------------------------------------|----------------------------------------------------------------------------------|
| `./specs/pages/agents.md`           | Coding client cards and setup.                                                   |
| `./specs/pages/chat.md`             | Chat workspace.                                                                  |
| `./specs/pages/client-keys.md`      | Key cards, create, rotate, revoke, one-time reveal.                              |
| `./specs/pages/connections.md`      | Overview, connection pane, models, model modal, All models, Add connection flow. |
| `./specs/pages/dashboard.md`        | KPI tiles, trend, Routing Universe, recent requests.                             |
| `./specs/pages/groups.md`           | Group cards, Preview modal, three-step editor.                                   |
| `./specs/pages/login-and-errors.md` | Sign-in and error pages.                                                         |
| `./specs/pages/logs.md`             | Request, daemon, and startup logs.                                               |
| `./specs/pages/settings.md`         | Every daemon choice — console, access, network, providers, daemon, retention.    |
| `./specs/pages/usage.md`            | Usage overview and tabs.                                                         |

<!-- file-map:end -->

`./specs/reference/` holds static HTML demos of the look. Refresh the file map with
`python docs/design/specs/tools/build-index.py` (from the repo root).

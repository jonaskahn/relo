---
name: changelog
description: Changes from 2.4 to 3.2.
---

# Changelog

## 3.2

| Area       | 3.1                                       | 3.2                                                                                                                                                                                                                            |
|------------|-------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Logs page  | Request log only                          | Agent, Daemon and Startup sections behind one segmented switch; request list and daemon log as dense row-cards with selection-box filters; boot transcripts with outcome and current marker; one shared equalizer live control |
| Tabs       | Underline bar (usage)                     | Text triggers per `form-controls`: selected `--accent-50` fill with `--accent-ink`, no underline                                                                                                                               |
| Usage page | Trend plus comparison bars                | Page header with queried-at caption and refresh; token mix, reliability, latency and spend-share charts off the trend's own days; group rows as row-cards with a spend meter                                                   |
| Nav drawer | Panel and scrim appear with no transition | Svelte slide from the left edge (260ms in, 220ms out), drag-release continues from the finger; scrim fades 140ms; fade only under reduced motion                                                                               |

## 3.1

| Area                       | 3.0                                           | 3.1                                                                                                               |
|----------------------------|-----------------------------------------------|-------------------------------------------------------------------------------------------------------------------|
| Card grid (layout control) | Cards snap between four and three per row     | One transform-only morph, 200ms, every card at once, instant under reduced motion                                 |
| Popover / menu             | Panels appear                                 | 160ms fade + 4px drop; fade only, 140ms, under reduced motion                                                     |
| Card hover                 | Mixed (`hover:bg-muted/40`, bespoke per page) | One shared hover on every card surface                                                                            |
| Card click                 | Title button only                             | The whole card is the hit area; Connections opens its pane, Groups / Client keys / Agents open their detail modal |
| Connection card head zoom  | 1.08                                          | 1.02 per rule 11                                                                                                  |

## 3.0

| Area                | 2.4                                     | 3.0                                                                                 |
|---------------------|-----------------------------------------|-------------------------------------------------------------------------------------|
| Structure           | One file                                | Index plus one file per topic                                                       |
| Borders             | Translucent, fewer nested               | Full border or none; no one-sided borders anywhere                                  |
| Accent              | Hand-tuned `--accent-50…800` per accent | One `--accent-base` per accent, scale derived with `color-mix`; same variable names |
| Accent set          | Red default, selectable                 | Ten accents, per-accent `--on-accent` for 4.5:1 contrast                            |
| Radius              | `--radius: 0.5rem`                      | 8 / 10 / 12 / 16 / 18px scale; controls 10px, cards and modals 16px                 |
| Controls            | Unspecified                             | 38px (44px touch), identical across a row                                           |
| Cards               | Unspecified                             | Level-1 surface, 1px `--line`, `--sunken` inner panels, no hover lift               |
| Quota               | Near-limit number in the accent         | `--accent` / `--warn` / `--danger` plus a status pill                               |
| Sidebar active item | Unspecified                             | Filled accent-soft pill, no left bar                                                |
| Page toolbar        | Mixed                                   | Count, search, chips, actions, layout control                                       |
| Labels              | Some uppercase                          | Sentence case only                                                                  |
| Row lists           | Unspecified                             | Row-cards with gaps, no dividers                                                    |

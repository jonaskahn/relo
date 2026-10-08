---
name: motion
description: Durations, easing, reduced-motion behaviour.
requires: []
---

# Motion

Motion answers an action or shows what changed. Two ambient moments are allowed: the daemon dot and the live-tail
equalizer, each only while its own state runs.

| Moment                                        | Spec                                                                                       | Reduced motion          |
|-----------------------------------------------|--------------------------------------------------------------------------------------------|-------------------------|
| Button / row press                            | scale `0.985`, 80ms, immediate                                                             | none                    |
| Hover (border, shadow, tint)                  | 150ms ease                                                                                 | same, it is not travel  |
| Toggle / segment selection                    | thumb slides on a spring (~240ms), colour 150ms ease                                       | thumb parks instantly   |
| Tab pane swap (logs, usage)                   | in 16px from the travel direction + fade 200ms; out 160ms                                  | 120ms fade, no travel   |
| Connection card head hover                    | scale `1.02` from the left edge, 150ms ease, transform only                                | none                    |
| Inline model details                          | interruptible 220ms disclosure                                                             | opens without travel    |
| Page switch                                   | fade in + 6px rise, 180ms                                                                  | shorter fade only       |
| Modal (desktop)                               | brief zoom from the opening control (about 220ms)                                          | fade only               |
| Bottom sheet (phone)                          | slides up from the lower edge                                                              | fade only               |
| Nav drawer (phone)                            | slides in from the left edge, 260ms; out 220ms; scrim fades 140ms                          | fade only, 140ms        |
| Popover / menu                                | 160ms fade + 4px drop                                                                      | fade only, 140ms        |
| Quota or progress bar fill                    | width 400ms `cubic-bezier(.2,.8,.2,1)`, once on mount                                      | no transition           |
| Card grid re-solve (layout control)           | one transform-only morph, 200ms `cubic-bezier(.2,.8,.2,1)`, every card at once, no stagger | no transition (instant) |
| Filter and search pair re-width (More / Less) | width 200ms ease, once per press                                                           | no transition           |
| Routing pulse (live event)                    | about 600ms                                                                                | removed                 |
| Daemon running dot                            | 2.4s soft ring, only while running                                                         | removed                 |
| Live-tail equalizer (logs, dashboard)         | four rounded bars pump on a staggered seamless loop, only while the tail runs              | removed                 |

## Where motion lives

Five shared modules own every moment in the console: `page-motion` (page switch), `modal-motion` (modal, bottom
sheet and nav drawer), `card-layout-motion` (card grid re-solve), `popover-motion` (popover and menu panel) and
`tab-motion` (segment thumb, tab strip, pane swap). A page inherits the
motion of the shared control it renders and never animates its own cards or panels.

- A control that re-solves a grid animates it once; a card never animates on its own.
- Every popup opens with `popoverMotion`; a native select popup is drawn by the OS and does not animate.

## Rules

- Closing a modal or popover returns focus to the control that opened it.
- Do not stack entrance animations on every card. A page gets one page-switch motion, nothing per card.
- A spinner marks a slot that is loading. The layout stays in place; the slot shows the spinner until data arrives.
- Wrap every animation in `@media (prefers-reduced-motion:no-preference)` or neutralise it in the reduce block.
- Durations never exceed 400ms except the ambient moments.

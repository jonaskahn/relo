---
name: reference-readme
description: Static HTML demos of the look and where they differ from the spec.
---

# Reference demos

- `connections-page.html`: Connections overview with attention strip, toolbar, cards, list view, appearance popover.
- `connections-pane.html`: one connection's pane — header, tabs, model row-cards — with the model modal open, whose
  footer **Edit** swaps the body to the form.
- `add-connection-modal.html`: the Add connection wizard — provider picker, connect form, verify rail, review split —
  with the group editor's stepper and footer summary.
- `new-group-modal.html`: three-step group editor at 980px (production uses the `xl` modal).

Differences from the spec:

| Demo                                         | Spec                                              |
|----------------------------------------------|---------------------------------------------------|
| Single-account cards show no account number  | Every card numbers its accounts                   |
| Quota panel fits its data                    | Quota panel reserves four rows                    |
| Grid and list toggle                         | Vertical and Horizontal layout control            |
| Cards lift 2px on hover                      | Border and shadow only                            |
| Purple default, one derived hex              | Red default, ten accents with `--on-accent`       |
| White text on accent in dark theme           | Near-black on every accent in dark theme          |
| Hard-coded status colours                    | `--ok`, `--warn`, `--danger`                      |
| Pane demo opens the model modal on load      | The modal opens from a row, reading first         |
| Add demo pauses at Verify with fixed results | Production probes the provider and fills the rail |
| Add demo copy is illustrative English        | Production resolves all 22 locales                |

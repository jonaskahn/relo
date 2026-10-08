---
name: client-keys
description: Key cards, create, rotate, revoke, one-time reveal.
requires: [toolbar-and-layout-control, modal-and-dialogs, status-and-banners]
---

# Client keys

Client keys authenticate inbound inference requests and identify traffic in the request log. Provider credentials are
never accepted here.

- **Toolbar**: the status filter select and search share exactly one card column of the grid below — the filter select
  the first third, search the remaining two thirds — and re-width with the layout control, with the key maintenance
  actions (**Delete selected**, **Clear expired**) and the layout control on the same row to their right. Pressing the
  layout control re-solves the grid with the shared morph; the page adds no motion of its own. The heading band holds
  title, subtitle and the primary **Create key**, as on Groups.
- **Sections**: Active first; an inactive and history section below (expired, revoked). The first visible section's grid
  ends with the dashed **Create key** tile ("Traffic from one coding client"), which opens the create modal.
- **Card**: name, hint (short digest hint, `.mono-data`), status pill (Active, Expires in …, Expired, Revoked), created,
  last used. The card is one full-card button opening the detail modal (`aria-label="Open <name>"`); the selection
  checkbox stays a separate control. Edit switches to edit mode inside the same modal.
- **Create, rotate, revoke** use CenteredModal; revoke and rotate confirmations use AlertDialog with the key name and
  the consequence ("Clients using this key stop working immediately").
- **One-time reveal**: the new key appears once in a mono read-only field with Copy; closing the modal clears it from
  memory and the copy says it cannot be shown again.
- Keys can expire, be rotated or revoked. The log filter links from a key to its requests.

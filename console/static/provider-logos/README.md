Provider SVGs in this directory come from [models.dev](https://github.com/anomalyco/models.dev) (MIT license).
Run `npm run fetch:logos` from `console/` after updating the bundled catalog snapshot.
The download script also generates `src/lib/provider-logo-ids.json`, so the UI can use a text fallback without requesting a missing file.
The script scans this directory after it downloads, so a mark committed by hand stays in the manifest across a regeneration.

## Marks @lobehub/icons publishes

Run `npm run extract:lobehub` from `console/` to cut these. models.dev has no
entry for them at all — they are coding agents, self-hosted runtimes, or small
routers rather than model providers — so `fetch:logos` will never write them.

The script unpacks a pinned tarball itself and `@lobehub/icons` is not a
dependency of the console; it is MIT licensed and ships a `Mono` (or
`BrandMono`) React component per mark, which is one `fill="currentColor"` path.
Only the path is copied, so the console keeps drawing these itself.

Ids no icon set publishes stay on the monogram fallback: `aki-io`, `bee`,
`cortecs`, `crof`, `hpc-ai`, `iteracompute`, `litellm`, `mixlayer`, `pareto`,
`regolo-ai`, `requesty`, `sap-ai-core`, `sarvam`, `stackit`, `synthetic`, plus
the agents `aside`, `gajae`, `prime`, `raycast`, `zcode`.

A vendor-supplied SVG goes in through `scripts/reduce-mark.mjs`, which strips
the editor metadata an export carries and keeps only what draws. Add
`--colour` when the mark's own inks are part of the drawing, which also keeps
its `<defs>` and stylesheet.

`other.svg` and `shared.svg` are drawn by hand, and there is nothing to fetch
for them. Check a hand-drawn mark by rasterising it and looking at the alpha:
the console masks on alpha, so the silhouette is what an operator sees, and a
mark can satisfy every structural check above while drawing the wrong shape.

The coding agents `codex`, `claude-code`, `grok-build`, `cline`, `openclaw`,
`dsh`, and `mcode` are cut here too, under the client identifier the daemon
reports, so a row on the agents page wears that agent's mark rather than the
mark of whichever model vendor the agent talks to. `claude-desktop` has no mark
of its own and wears Anthropic's through an alias in `provider-logo.ts`.

Some of these marks are drawn as several separate paths rather than one, and all
of them are kept: OpenClaw is the clearest case, where its first path on its own
is a dot about a pixel across and the mark only appears once all five are drawn.
`provider-logo.test.ts` pins the path count so a truncated extraction cannot
come back.

`defs`, `clipPath`, `mask` and `pattern` are stripped before anything is read
out of them. The icon set carries a full-canvas `M0 0h24v24H0z` inside a clip
path, and that is geometry a mark is clipped _by_, never ink it draws. Copied
out as a path it is a fully opaque rectangle, which a mask renders as a solid
block — the mark disappears rather than shows up.

## Marks drawn as images rather than masks

Every mark here is a single `currentColor` path, so the console paints it as a
CSS mask and it takes the theme's ink. A mask keeps only alpha, so a mark that
cannot be reduced to a silhouette is listed in `providerLogoSrc`'s `colored`
set and drawn as an image instead:

- `302ai` — drawn in three greys.
- `atomic-chat` — a raster whose plate is opaque to its edge.

`openclaw` is a case where the vendor publishes a colour mark and the icon set
publishes a monochrome one, and the monochrome one ships. The vendor's mark
cannot be reduced to a single ink without losing the drawing: its eyes
(`#050810`) and highlights (`#00e5cc`) are contrast _inside_ a gradient body,
so at one ink they would merge into it. LobeHub's `OpenClaw` Mono is a
purpose-drawn silhouette of the same mark, which is what the console draws.

## Marks models.dev does not publish

- `kilo.svg` — models.dev _does_ publish a `kilo` entry, but what it serves there is a
  generic chevron that is not the vendor's mark, so the glyph was taken from the Kilo Code
  brand export (supplied by the project, light and dark variants identical in geometry and
  differing only in `fill`) and saved as seven `currentColor` paths in the original
  `0 0 32 32` viewBox. It does not go in through `scripts/reduce-mark.mjs`: that script keeps
  only `<path>`, and this export also draws with `<rect>` and `<polygon>`, so lifting it
  verbatim needed those rewritten into path data. `fetch:logos` skips this id (see
  `keepCommitted` in `scripts/fetch-provider-logos.mjs`) so regenerating the marks cannot put
  the generic chevron back.
- `cursor.svg` — Cursor publishes no mark on models.dev, so the glyph was taken from
  `https://cursor.com/favicon.svg` (fetched 2026-09-28) and reduced to its own cube path.
  The file that ships is Cursor's geometry verbatim; the dark plate behind it was dropped,
  because the console draws every mark on a light tile and a plate would read as a filled
  square at 24px. The remaining path is one ink (`#14120b`, Cursor's own plate colour), so it
  needs no theme-aware masking. Used for both the Cursor coding client and the Cursor
  upstream connection, which are the same brand on two surfaces.
- `teamorouter.svg` — TeamoRouter publishes no mark on models.dev, so the glyph was taken
  from the TeamoRouter console nav logo (`https://teamorouter.cn`, fetched 2026-09-30) and
  saved as one `currentColor` path in the original 15×16 viewBox.
- `orcarouter.svg` — OrcaRouter _is_ in models.dev, but what it serves there is a 42×22
  raster with no alpha channel, which a mask would reduce to a filled box and which cannot
  take the console's ink. The vector OrcaRouter supplies was committed instead, reduced to
  two `currentColor` paths in the original `0 0 98 98` viewBox. `fetch:logos` skips this id
  (see `keepCommitted` in `scripts/fetch-provider-logos.mjs`) so regenerating the marks
  cannot put the raster back.
- `hermes.svg` — supplied by the project. An Inkscape export: the glyph is
  drawn in a 24-unit space and scaled by `matrix(8.333333,…)` to fill a
  `0 0 200 200` viewBox, so the group transform is kept (dropping it would draw
  the mark at an eighth size in the corner) and the root's `fill-rule:evenodd`
  is moved onto the paths rather than lost. The geometry is ~9 kB of opaque path
  data, which is why it goes in through `scripts/reduce-mark.mjs`: the script
  lifts the `d` attributes out verbatim and never retypes them.
- `other.svg` — drawn here rather than taken from anywhere. "Other" in the key
  sheet stands for a client the operator names by hand, and every other row in
  that sheet is a command-line agent, so this is a terminal: a rounded window
  with a `>` and an underscore. An ellipsis was tried first and dropped — it says
  "more" rather than "a client", and at the 14px the xs tile uses it reads as
  decoration.
- `shared.svg` — drawn here too. "Shared" is a key reusable across clients
  rather than one bound to a single client, so it is one node fanning out to
  two: that depicts the relationship rather than naming an abstraction. A globe
  was tried first and dropped, because it reads as "internet", which is not what
  the key does.
- `pi.svg` — supplied by the project, not by either script. The glyph is already
  centred in its `0 0 300 300` viewBox (its ink measures 269.5 × 234 about
  (150.1, 150.0)), so nothing was rescaled.
- `omp.svg` — "Oh My Pi", composed from `pi.svg` rather than drawn: the Pi glyph
  at `scale(0.6)` inside a `currentColor` ring, the ring being the O. A lockup
  reading "O" then "Pi" side by side was rejected because it stops being legible
  at the 14px the xs logo tile uses. The scale is bounded by the ring: the glyph's
  half-diagonal is 178.5, so at 0.6 it needs 107.1 of the ring's 117 inner
  radius. `provider-logo.test.ts` pins the composition.

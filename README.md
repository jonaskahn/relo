# Relo site

Static introduction page for [Relo](https://github.com/jonaskahn/relo), published at
<https://relo.ifelse.on/> (`CNAME`). This branch holds only the published files; the
source code lives on `main`.

| Path              | Purpose                                                             |
| ----------------- | ------------------------------------------------------------------- |
| `index.html`      | The page                                                            |
| `style.css`       | Styles, including the "2050 layer" at the end (glass, depth, motion) |
| `field.js`        | The signal field canvas, the intro curtain, the shared rAF loop      |
| `scenes.js`       | Scroll-driven product story (needs `motion.js`)                      |
| `motion.js`       | Motion 12.23.24, MIT, vendored for the scroll story                  |
| `script.js`       | Menu, theme, language, copy, tabs, the terminal replay               |
| `i18n.js`         | Translations for 20 languages                                       |
| `install.sh`      | The macOS and Linux installer                                       |
| `install.ps1`     | The Windows installer                                               |
| `appcast.xml`     | Sparkle update feed for `Relo.app`; keep it beside `index.html`      |
| `media/`          | Logo, provider marks, JetBrains Mono                                |

No build step. Serve the folder as is, for example `python3 -m http.server`.

## The installers

Both live at the site root so that one line is all it takes:

```sh
curl -fsSL https://relo.ifelse.on/install.sh | sh
```

```powershell
irm https://relo.ifelse.on/install.ps1 | iex
```

They install **the daemon only**, from the latest GitHub release. Each one
picks the platform and architecture, shows a progress bar, verifies the
checksum against `checksums.txt`, reads the ports out of `config.toml`, stops
the running daemon, frees those ports, installs the binary, and starts the
daemon again.

An installation that belongs to another channel — Homebrew, npm, `Relo.app`,
a `.deb`, a `.rpm`, or `Relo-Setup` — is **refused**, and the message names the
command that removes it. Two Relos cannot share one state directory and one
set of ports. A re-run over their own install (marked by `.relo-install` next
to the binary) is treated as an upgrade.

Options: `--version`, `--dir`, `--home`, `--no-start`, `--no-autostart`,
`--force-ports`, `-y`, `--help`. Everything can also come from the environment
(`RELO_VERSION`, `RELO_INSTALL_DIR`, `RELO_HOME`, `RELO_INSTALL_YES`), which is
what a piped install needs. With a terminal attached, both scripts show an
options screen instead of taking flags.

`RELO_RELEASE_BASE` points the scripts at another host; the tests use it to
serve a mock release. See the table in the install section of the page for
the channels the site itself recommends.

## The page

One canvas `#field` sits behind everything: a grid of nodes that pulses travel
along, out of a tool, through the core, into a provider. Each section asks the
field for a different mood through `data-field` (`hero`, `story`, `providers`,
`install`), the pointer lights what it passes over, and the first load
assembles "Relo." out of the same particles before they scatter into the grid.

Everything rides one `requestAnimationFrame`. The loop stops when the tab is
hidden or the section is offscreen, and under `prefers-reduced-motion` the
field draws a single still frame and every other effect keeps its end state
with no travel. Motion is limited to transform, opacity and filter.

The story scrolls an iPad Pro 13 through boot, a nine-card connection grid,
and a harness bag, all driven by one scroll-progress value in `scenes.js`.
Without Motion, or under `prefers-reduced-motion`, the page shows the woken
iPad with the cards settled and the captions as a plain list.

The install section leads with the script, detects the visitor's operating
system, and keeps the other channels behind three tabs. Beside it a terminal
replays a real installer run — the command, the download bar, the checksum,
the freed ports, the health check — and pauses while it is offscreen.

Brand marks in `media/logos/` mirror `console/static/provider-logos/`:
models.dev entries (MIT) plus `@lobehub/icons` cut-outs (MIT). Copy them back
with the console script when the roster changes.

## Publishing

This branch is the published copy. Edit the files here, or copy the folder
over from `main` with `scripts/publish-site.sh` when the site also lives in
`site/` there.

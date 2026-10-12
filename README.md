# Relo site

Static introduction page for [Relo](https://github.com/jonaskahn/relo), published at
<https://jonaskahn.github.io/relo/>. This branch holds only the published files; the
source code lives on `main`.

| Path             | Purpose                                                        |
| ---------------- | -------------------------------------------------------------- |
| `index.html`     | The page                                                       |
| `style.css`      | Styles                                                         |
| `script.js`      | Intro, menu, theme, copy buttons                               |
| `scenes.js`      | Scroll-driven product story (needs `motion.js`)                |
| `motion.js`      | Motion 12.23.24, MIT, vendored for the scroll story            |
| `i18n.js`        | Translations for 20 languages                                  |
| `appcast.xml`    | Sparkle update feed for `Relo.app`; keep it beside `index.html` |
| `media/`         | Logo, provider marks, JetBrains Mono                           |

No build step. Serve the folder as is, for example `python3 -m http.server`.

The story scrolls an iPad Pro 13 through boot, a nine-card connection grid,
and a harness bag, all driven by one scroll-progress value in `scenes.js`.
Without Motion, or under `prefers-reduced-motion`, the page shows the woken
iPad with the cards settled and the captions as a plain list.

Brand marks in `media/logos/` mirror `console/static/provider-logos/`:
models.dev entries (MIT) plus `@lobehub/icons` cut-outs (MIT). Copy them back
with the console script when the roster changes.

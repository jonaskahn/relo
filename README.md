# Relo site

Static introduction page for [Relo](https://github.com/jonaskahn/relo), published at
<https://jonaskahn.github.io/relo/>. This branch holds only the published files; the
source code lives on `main`.

| Path             | Purpose                                                        |
| ---------------- | -------------------------------------------------------------- |
| `index.html`     | The page                                                       |
| `style.css`      | Styles                                                         |
| `script.js`      | Intro, menu, theme, count-up, copy buttons                     |
| `i18n.js`        | Translations for 20 languages                                  |
| `appcast.xml`    | Sparkle update feed for `Relo.app`; keep it beside `index.html` |
| `media/`         | Logo and dashboard screenshots (desktop and mobile)            |

No build step. Serve the folder as is, for example `python3 -m http.server`.

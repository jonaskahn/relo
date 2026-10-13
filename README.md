# Relo site

Static page for [Relo](https://github.com/jonaskahn/relo), published at
<https://relo.ifelse.on/>. This branch holds only the published files.

| Path           | Purpose                                                        |
| -------------- | -------------------------------------------------------------- |
| `index.html`   | The page                                                       |
| `style.css`    | Styles                                                         |
| `field.js`     | The background canvas                                          |
| `script.js`    | Menu, theme, language, copy, tabs, terminal                    |
| `scenes.js`    | Scroll story over the iPad (needs `motion.js`)                 |
| `motion.js`    | Motion 12.23.24, MIT, vendored for the story                   |
| `i18n.js`      | Translations for 20 languages                                  |
| `install.sh`   | Installer for macOS and Linux                                  |
| `install.ps1`  | Installer for Windows                                          |
| `appcast.xml`  | Sparkle update feed for `Relo.app`                             |
| `CNAME`        | The custom domain                                              |
| `media/`       | Logo, provider marks, JetBrains Mono                           |

No build step. Serve the folder as is, for example `python3 -m http.server`.

The installers sit at the site root so one line is enough:

```sh
curl -fsSL https://relo.ifelse.on/install.sh | sh
```

```powershell
irm https://relo.ifelse.on/install.ps1 | iex
```

They install the daemon from the latest release. An installation that belongs
to another channel is refused rather than stacked; `install.sh --help` lists
the flags.

Brand marks in `media/logos/` mirror `console/static/provider-logos/`. Copy
them back with the console script when the roster changes.

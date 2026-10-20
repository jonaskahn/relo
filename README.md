# Relo site

Static page for [Relo](https://github.com/jonaskahn/relo), published at
<https://relo.ifelse.one/>. This branch holds only the published files.

| Path                  | Purpose                                              |
| --------------------- | ---------------------------------------------------- |
| `index.html`          | The page                                             |
| `css/style.css`       | Styles                                               |
| `js/field.js`         | The background canvas                                |
| `js/script.js`        | Menu, theme, language, copy, tabs, terminal          |
| `js/scenes.js`        | Scroll story over the iPad (needs Motion)            |
| `js/i18n.js`          | Translations for 20 languages                        |
| `js/releases.js`      | Setup rows and changelogs, filled from the catalog   |
| `vendor/motion/`      | Motion 12.23.24, MIT, vendored for the story         |
| `media/`              | Logo, provider marks, JetBrains Mono                 |
| `install.sh`          | Installer for macOS and Linux                        |
| `install.ps1`         | Installer for Windows                                |
| `appcast.xml`         | Sparkle update feed for `Relo.app`                   |
| `releases.json`       | Every published release, with files and notes        |
| `CNAME`               | The custom domain                                    |

No build step. Serve the folder as is, for example `python3 -m http.server`.

The installers sit at the site root so one line is enough:

```sh
curl -fsSL https://relo.ifelse.one/install.sh | sh
```

```powershell
irm https://relo.ifelse.one/install.ps1 | iex
```

They install the daemon from the latest release. An installation that belongs
to another channel is refused rather than stacked; `install.sh --help` lists
the flags.

`releases.json` is written by `.github/workflows/publish-releases.yml` in the
main repository, which runs the catalog generator on every release and on
every release-notes edit. The page reads it to point each setup row at the
file that release actually carries and to show its changelog. A release with
no file for a platform leaves that row out, and a missing catalog leaves the
static release-page links in place.

Brand marks in `media/logos/` mirror `console/static/provider-logos/`. Copy
them back with the console script when the roster changes.

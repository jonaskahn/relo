# Third-Party Notices

Relo includes or links the third-party software, fonts, and artwork listed
below. Each remains under its own license; the full text ships with the
upstream project. Relo itself is licensed under the AGPL-3.0 (see
[LICENSE](LICENSE)); these notices cover only what it bundles.

Last reviewed against `daemon/go.mod` and `console/package.json`. Update this
file whenever you add, remove, or upgrade a dependency, font, or logo.

## Go modules (daemon)

Versions come from `daemon/go.mod`; licenses were read from each module's
LICENSE file. `github.com/gogpu/systray` is built from the vendored fork in
`daemon/forks/systray` (MIT, Copyright (c) 2026 Andrey Kolkov and GoGPU
Contributors).

| Module                                 | Version                            | License      | Kind     |
|----------------------------------------|------------------------------------|--------------|----------|
| `github.com/BurntSushi/toml`           | v1.6.0                             | MIT          | direct   |
| `github.com/andybalholm/brotli`        | v1.2.0                             | MIT          | direct   |
| `github.com/go-webgpu/goffi`           | v0.6.4                             | MIT          | direct   |
| `github.com/gogpu/systray`             | v0.3.0                             | MIT          | direct   |
| `github.com/klauspost/compress`        | v1.18.0                            | MIT          | direct   |
| `github.com/nicksnyder/go-i18n/v2`     | v2.6.1                             | MIT          | direct   |
| `github.com/sirupsen/logrus`           | v1.9.3                             | MIT          | direct   |
| `github.com/spf13/cobra`               | v1.10.2                            | Apache-2.0   | direct   |
| `github.com/spf13/pflag`               | v1.0.10                            | BSD-3-Clause | direct   |
| `github.com/srwiley/oksvg`             | v0.0.0-20221011165216-be6e8873101c | BSD-3-Clause | direct   |
| `github.com/srwiley/rasterx`           | v0.0.0-20220730225603-2ab79fcdd4ef | BSD-3-Clause | direct   |
| `github.com/tiktoken-go/tokenizer`     | v0.8.1                             | MIT          | direct   |
| `github.com/zalando/go-keyring`        | v0.2.8                             | MIT          | direct   |
| `golang.org/x/image`                   | v0.46.0                            | BSD-3-Clause | direct   |
| `golang.org/x/sys`                     | v0.48.0                            | BSD-3-Clause | direct   |
| `golang.org/x/text`                    | v0.42.0                            | BSD-3-Clause | direct   |
| `gopkg.in/yaml.v3`                     | v3.0.1                             | MIT          | direct   |
| `modernc.org/sqlite`                   | v1.60.1                            | BSD-3-Clause | direct   |
| `github.com/danieljoos/wincred`        | v1.2.3                             | MIT          | indirect |
| `github.com/dlclark/regexp2/v2`        | v2.8.0                             | MIT          | indirect |
| `github.com/dustin/go-humanize`        | v1.1.0                             | MIT          | indirect |
| `github.com/godbus/dbus/v5`            | v5.2.2                             | BSD-2-Clause | indirect |
| `github.com/google/uuid`               | v1.6.0                             | BSD-3-Clause | indirect |
| `github.com/inconshreveable/mousetrap` | v1.1.0                             | Apache-2.0   | indirect |
| `github.com/mattn/go-isatty`           | v0.0.24                            | MIT          | indirect |
| `github.com/ncruces/go-strftime`       | v1.1.0                             | MIT          | indirect |
| `github.com/remyoudompheng/bigfft`     | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause | indirect |
| `golang.org/x/net`                     | v0.59.0                            | BSD-3-Clause | indirect |
| `modernc.org/libc`                     | v1.77.1                            | BSD-3-Clause | indirect |
| `modernc.org/mathutil`                 | v1.7.1                             | BSD-3-Clause | indirect |
| `modernc.org/memory`                   | v1.12.1                            | BSD-3-Clause | indirect |

Regenerate the list with `go list -m -json all` in `daemon/`.

## Console runtime packages (npm)

These are bundled into the embedded console. Their own transitive
dependencies are bundled with them and carry their own licenses; run
`npm ls --omit=dev --all` in `console/` to list them.

| Package             | Version | License |
|---------------------|---------|---------|
| `bits-ui`           | 2.19.3  | MIT     |
| `clsx`              | 2.1.1   | MIT     |
| `svelte-i18n`       | 4.0.1   | MIT     |
| `svelte-sonner`     | 1.2.1   | MIT     |
| `tailwind-merge`    | 3.7.0   | MIT     |
| `tailwind-variants` | 3.3.1   | MIT     |

Development tooling (Svelte, SvelteKit, Vite, Tailwind CSS, TypeScript,
ESLint, Prettier, Vitest) is used to build the console and is not distributed
in the release.

## Fonts

Shipped in `console/static/fonts`, licensed under the
[SIL Open Font License 1.1](https://openfontlicense.org):

| Font           | Files                                                       |
|----------------|-------------------------------------------------------------|
| Inter          | `inter-var.ttf`                                             |
| JetBrains Mono | `jetbrains-mono-regular.ttf`, `jetbrains-mono-semibold.ttf` |

## Provider logos

Marks in `console/static/provider-logos` are the property of their respective
owners and are shown only to identify the provider. See
`console/static/provider-logos/README.md` for how each was obtained.

| Source                                                    | License |
|-----------------------------------------------------------|---------|
| [models.dev](https://github.com/anomalyco/models.dev)     | MIT     |
| [`@lobehub/icons`](https://github.com/lobehub/lobe-icons) | MIT     |

## macOS updater

`Relo.app` embeds [Sparkle](https://sparkle-project.org) 2.x for updates.
Sparkle is distributed under its own license (MIT-style, with bundled
external components); the full text is in Sparkle's `LICENSE` file at
<https://github.com/sparkle-project/Sparkle/blob/2.x/LICENSE>.

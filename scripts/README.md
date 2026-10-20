# scripts/

Helper scripts. Run from the repo root unless noted. Most are also reachable
via `make` targets; the mapping is listed per script.

## Traffic capture (debugging agent requests/responses)

`capture-addon.py` is the mitmproxy addon loaded by `capture.mjs`; not run
directly.

```console
$ node scripts/capture.mjs setup
$ node scripts/capture.mjs run -- opencode run "hi"
$ node scripts/capture.mjs run --show-secrets -- claude "hi"
$ node scripts/capture.mjs run --local opencode -- opencode run "hi"
$ node scripts/capture.mjs teardown
$ node scripts/capture.mjs teardown --purge
```

Requires `mitmproxy` (`brew install mitmproxy`). Captures land in
`debugger/captures/capture-*.jsonl`, secrets masked unless `--show-secrets`.
`setup --trust` installs the CA into the System keychain; `teardown` removes
it but never deletes `~/.mitmproxy` CA files.

## Build / sync

### build-console.sh (`make build-console`)

```console
$ ./scripts/build-console.sh
```

Builds the Svelte console into `console/build` (`npm ci` first if needed).

### sync-assets.sh (`make sync-assets`)

```console
$ ./scripts/sync-assets.sh
```

Copies `assets/logo.svg` → `console/static/logo.svg`.

### sync-console.sh (`make sync-console`)

```console
$ ./scripts/build-console.sh && ./scripts/sync-console.sh
```

Copies `console/build` → `daemon/internal/dashboard/dist`. Fails without a
console build.

### gen-syso.sh (`make icons`, with `go run ./tools/genicons`)

```console
$ ./scripts/gen-syso.sh
$ ./scripts/gen-syso.sh /tmp/syso-staging   # drift-check staging dir
```

Writes `relo_windows_*.syso` (default `daemon/cmd/relo`). Pinned `rsrc`.

## Checks

### check-icons.sh (`make icons-check`)

```console
$ ./scripts/check-icons.sh
```

Fails on artwork drift. Pixel-compare binds on `linux/amd64` (CI); other
hosts only prove the generator runs.

### check-macos-target.sh

```console
$ ./scripts/check-macos-target.sh
$ ./scripts/check-macos-target.sh dist/Relo.app
$ MACOS_MIN_VERSION=12.0 ./scripts/check-macos-target.sh dist/Relo.app
```

Fails when the bundle targets newer macOS than the floor or is badly signed.

### check-coverage.sh (`make cover`)

```console
$ ./scripts/check-coverage.sh
$ COVERAGE_THRESHOLD=90.0 COVERAGE_OUT=coverage.out ./scripts/check-coverage.sh
```

Per-package gate from `daemon/tests/coverage.tsv`. Runs the mapped Go test
suites, so it takes minutes.

### validate-plans.sh

```console
$ ./scripts/validate-plans.sh
```

Standalone; expects `plans/phases/*/P*.md`, which is not in this tree —
fails there. Legacy plan-tree validator.

### make-help.awk (internal)

```console
$ make          # renders the target list via scripts/make-help.awk
```

Renderer for `make help`; not run directly.

## Dev setup

### dev-setup.sh (`make dev-setup`)

```console
$ ./scripts/dev-setup.sh
$ ./scripts/dev-setup.sh 10201
$ RELO_HOME=/tmp/relo-dev ./scripts/dev-setup.sh 10209
```

Writes the dev `config.toml` once, leaves an existing config alone.

### dev-setup.ps1 (Windows)

```powershell
PS> powershell -File scripts/dev-setup.ps1
PS> powershell -File scripts/dev-setup.ps1 -Port 10201
PS> $env:RELO_HOME = 'C:\relo-dev'; powershell -File scripts/dev-setup.ps1
```

Same as `dev-setup.sh` for Windows contributors without bash/make.

## Packaging / release

### package-linux.sh

```console
$ ARCH="amd64 arm64" VERSION=0.1.0 ./scripts/package-linux.sh
$ ARCH=amd64 VERSION=0.1.0 ./scripts/package-linux.sh   # one arch
```

Requires `nfpm` (`make ci-package-linux`).

### package-macos.sh

```console
$ VERSION=0.1.0 BUILD=0.1.0 ./scripts/package-macos.sh
$ VERSION=0.1.0-beta.1 BUILD=0.1.42 MACOS_MIN_VERSION=12.0 ./scripts/package-macos.sh
```

macOS only (`lipo`, `sips`, `iconutil`). Beta builds (`BUILD` ≠ `VERSION`)
skip Sparkle auto-checks.

### package-windows.sh

```console
$ ARCH="amd64 arm64" VERSION=0.1.0 ./scripts/package-windows.sh
```

Requires `makensis` (`brew install makensis` / `apt install nsis`).

### generate-appcast.py

```console
$ python3 scripts/generate-appcast.py --help
$ python3 scripts/generate-appcast.py site/appcast.xml 0.1.0 0.1.0 \
    https://example.com/Relo-0.1.0.zip 123456 abc123== 12.0 \
    "Mon, 01 Jan 2026 00:00:00 +0000"
```

Inserts one release into the Sparkle appcast; used by
`.github/workflows/publish-appcast.yml`.

### generate-releases.py

```console
$ gh api --paginate --slurp "repos/jonaskahn/relo/releases?per_page=100" > /tmp/releases.json
$ python3 scripts/generate-releases.py /tmp/releases.json site/releases.json
```

Writes the catalog of every published release — stable and beta, drafts
excluded — that the site and the console read. Used by
`.github/workflows/publish-releases.yml`.

### publish-site.sh

```console
$ ./scripts/publish-site.sh
```

Refreshes the local `gh-pages` branch from `site/` (does not push).
`site/` is not in this tree.

### version.py (`make release-check`, `version-bump`, `release-tag`)

```console
$ python3 scripts/version.py current
0.1.0
$ python3 scripts/version.py check v0.1.0
$ python3 scripts/version.py bump patch
```

Single source of truth for version strings across `Makefile`, Go, and npm
package files.

### update-secrets.sh (`make update-secrets`)

```console
$ ./scripts/update-secrets.sh --list
$ ./scripts/update-secrets.sh
$ ./scripts/update-secrets.sh SPARKLE_PRIVATE_KEY VAR:APP_CAST_URL
```

Stores release secrets via `gh` (values via stdin only, never argv).
Requires `gh auth login`.

### sparkle-keys.sh (library)

```console
$ . ./scripts/sparkle-keys.sh && ensure_sparkle
```

Sourced by `package-macos.sh` / `update-secrets.sh`; not run directly.
Helpers print only the requested value on stdout.

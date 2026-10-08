# Development

Use the formatters and checks described in the
[Go](conventions/golang.md#tooling) and
[Svelte](conventions/svelte.md#verification) guides.

**Local verification** is available through these targets:

1. `make ci-backend` — Go dependency verification, build, formatting, vet, lint, and race tests
2. `make ci-console` — `svelte-check`, eslint, prettier, vitest, and the production build
3. `make ci-assets` — check generated artwork for drift
4. `make ci-coverage` — per-package backend coverage gate
5. `make ci-cross-build` — console plus six platform targets
6. `make ci-package-linux` — the `Relo-Setup` `.deb` and `.rpm`; requires nfpm and stays out of `make ci`
7. `make test-npm` and `make npm-pack` — installer rules and the tarball `files` list

`make ci` runs the first five checks before a push.

## Windows contributors

Go plus Node 22 is enough on native Windows: `scripts/dev-setup.ps1` writes
the dev home (the VS Code `relo: dev setup` task runs it automatically),
then `go run ./cmd/relo daemon run` in `daemon/` and `npm run dev` in
`console/`. `make ci`, the `scripts/*.sh` builds, and `make package-windows`
still need Git Bash or WSL.

`Relo.app` updates through Sparkle. Every other install opens
`system.updates.download` (GitHub latest by default). Point GitHub Pages at the
`gh-pages` branch once in the repository
settings.

`Relo.app` supports macOS 12 and later; `MACOS_MIN_VERSION` in the Makefile
is the one place that number lives, and `make package-macos` fails when a
binary in the bundle requires anything newer. The bundle is ad-hoc signed,
not notarized, so share `Relo-<version>-universal.zip` and never the bare
`.app` folder, which loses the symlinks inside `Sparkle.framework`. On
another Mac the first launch needs `xattr -dr com.apple.quarantine Relo.app`
or System Settings → Privacy & Security → Open Anyway. The appcast's
`minimumSystemVersion` in `.github/workflows/ci.yml` must match the floor.

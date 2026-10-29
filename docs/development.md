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

## models.dev build seed

Release binaries embed a models.dev dataset so a fresh install can price and
describe models before its first successful download. The generated
`daemon/internal/adapters/modelsdev/seeddata/modelsdev.json.gz` is ignored by
git; only the placeholder that keeps `//go:embed` compilable is committed.

- `make seed` downloads the dataset only when the existing seed is missing or
  invalid.
- `make seed-latest` downloads a fresh dataset unconditionally.
- `make clean` removes the generated seed and leaves the placeholder.

The Make-backed build, test, vet, lint, packaging, and cross-build targets
depend on `seed`. Beta and release workflows run `seed-latest` before
compiling, so a download or validation failure stops them before artifacts are
produced. A direct `go build ./...` or `go test ./...` without a generated seed
remains supported: the runtime fetches models.dev and writes its normal cache.
At runtime the network is tried first; a failed fetch falls back to the newest
of a valid cache and the embedded seed, while search, pricing, and state reads
never ask the network.

## Windows contributors

Go plus Node 22 is enough on native Windows: `scripts/dev-setup.ps1` writes
the dev home (the VS Code `relo: dev setup` task runs it automatically),
then `go run ./cmd/relo daemon run` in `daemon/` and `npm run dev` in
`console/`. `make ci`, the `scripts/*.sh` builds, and `make package-windows`
still need Git Bash or WSL.

`Relo.app` updates through Sparkle. Every other install opens the update
dialog with the command for the channel that installed it (shell, PowerShell,
npm, or Homebrew), or with every command when the channel cannot be detected;
the dialog always offers the GitHub release page. A stable build follows the
appcast, and a prerelease build follows the release catalog's newest beta,
falling back to the public releases feed. Point GitHub Pages at the
`gh-pages` branch once in the repository
settings.

`Relo.app` supports macOS 12 and later; `MACOS_MIN_VERSION` in the Makefile
is the one place that number lives, and `make package-macos` fails when a
binary in the bundle requires anything newer. With a Developer ID in the
keychain the build signs with it and prompts for Apple ID credentials to
notarize, so the app opens cleanly on another Mac; without one it falls back
to ad-hoc signing, and the first launch elsewhere needs
`xattr -dr com.apple.quarantine Relo.app` or System Settings → Privacy &
Security → Open Anyway. Share `Relo-<version>-universal.zip` and never the
bare `.app` folder, which loses the symlinks inside `Sparkle.framework`. The
appcast's `minimumSystemVersion` in `.github/workflows/ci.yml` must match the
floor.

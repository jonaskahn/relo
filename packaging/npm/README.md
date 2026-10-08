<div align="center">
<img src="assets/logo.png" alt="Relo" width="48">

<h2>Relo</h2>

<code>▚▚▚▚▚▚▚ Light • Fast • Reliable ▚▚▚▚▚▚▚</code>

</div>

> One loopback proxy for Codex, Claude Code, Cursor, and Grok keep their own config and still reach any upstream.


```console
$ npm install -g relo
$ relo daemon start
```

## What you get

This package installs the `relo` binary and puts a `relo` command on your
PATH. It is the daemon with its tray icon, downloaded from the GitHub release
and verified against the release checksums.

`relo` runs headless where there is no desktop session — a container or a
server — and shows its tray icon where there is one. The installers
(`Relo-Setup-*.exe` on Windows, the `Relo-Setup-*.deb` / `Relo-Setup-*.rpm`
packages on Linux, and `Relo.app` on macOS) install the same binary.

Every install provides the `relo` command, so they are never installed
together: if another relo is already on your PATH, this install refuses and
tells you where it found it. Removing that install first lets this package
take over.

Supported hosts: macOS, Linux, and Windows on both Intel and Apple Silicon (or
their ARM equivalents). State lives in `$RELO_HOME`, `~/.relo` by default, so
an npm install and a `.deb` install share the same configuration.

## Updates

The postinstall stops a running relo before replacing the binary: it asks the
old build to drain, ends the processes of this home that still hold a
configured port, and checks the ports are free before it writes the new one. A
port another program holds fails the install rather than killing that program:

```
relo: install failed -- something else is listening on port 10101
```

Free the port, or point `$RELO_HOME/config.toml` at another one, and install
again. The data is never touched by an update.

## Uninstalling

```console
$ npm uninstall -g relo
```

This keeps your data in `$RELO_HOME`: the preuninstall script stops the daemon,
frees its ports, and turns off start-at-login. To remove the data as well, use
the daemon itself, which also removes the program:

```console
$ relo uninstall --wipe
```

Without a flag, `relo uninstall` asks whether the data goes.

## Install options

The installer downloads the daemon binary from a GitHub release: the latest
one by default, or the tag `--relo-version` names when the installer is run
by hand:

```console
$ node "$(npm root -g)/relo/lib/install.mjs" --relo-version 0.2.0
```

## Troubleshooting

An install that printed a platform or checksum error left nothing behind, so
fix the cause and run the installer again:

```console
$ node "$(npm root -g)/relo/lib/install.mjs"
```

Downloads are verified against `checksums.txt` from the release. A mismatch
fails the install rather than writing an executable.

## Links

- [Source](https://github.com/jonaskahn/relo)
- [Releases](https://github.com/jonaskahn/relo/releases)
- [Documentation](https://github.com/jonaskahn/relo#readme)

MIT

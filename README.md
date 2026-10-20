<div align="center">
<img src="assets/logo.svg" alt="Relo" width="96">

<h2>Relo</h2>

<code>▚▚▚▚▚▚▚ Light • Fast • Reliable ▚▚▚▚▚▚▚</code>

<a href="https://github.com/jonaskahn/relo/actions/workflows/ci.yml"><img src="https://github.com/jonaskahn/relo/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
<a href="https://codecov.io/gh/jonaskahn/relo"><img src="https://codecov.io/gh/jonaskahn/relo/branch/main/graph/badge.svg" alt="Coverage"></a>
<a href="https://github.com/jonaskahn/relo/releases"><img src="https://img.shields.io/badge/version-0.1.5-blue" alt="Version"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo"><img src="https://img.shields.io/npm/v/@jonaskahn/relo?label=npm%40latest" alt="npm latest"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo?activeTab=versions"><img src="https://img.shields.io/npm/v/@jonaskahn/relo/beta" alt="npm beta"></a>

<p><i>Let your harness agents talk to any model, any provider with no config changes.</i></p>

</div>

---

## ▓ CONSOLE

<div align="center">

https://github.com/user-attachments/assets/cbda11c8-b60c-413b-8a08-03144dd06e7a

<sub>Demo: using Relo in Codex</sub>
</div>

|                                                                                                                             |                                                                                                                           |                                                                                                                      |
|-----------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| Dashboard                                                                                                                   | Connections                                                                                                               | Groups                                                                                                               |
| <img src="https://github.com/user-attachments/assets/ba44719e-f176-4d02-9e92-10cf7055a6e8" alt="Dashboard" width="320">     | <img src="https://github.com/user-attachments/assets/b27d9948-1f30-4aac-a0f5-d98109c5522e" alt="Connections" width="320"> | <img src="https://github.com/user-attachments/assets/29ce8e23-925f-4d22-b571-a49d8bc70841" alt="Groups" width="320"> |
| Agent Harness                                                                                                               | Add agent                                                                                                                 | Usage                                                                                                                |
| <img src="https://github.com/user-attachments/assets/3e5bfe37-ad48-4c12-a32a-f4901f8f982a" alt="Agent Harness" width="320"> | <img src="https://github.com/user-attachments/assets/f4730cb7-7177-40c0-8d48-905dfde29ba3" alt="Add agent" width="320">   | <img src="https://github.com/user-attachments/assets/42f329be-4d1b-408c-bc8e-87a1237eccf6" alt="Usage" width="320">  |

## ▓ INSTALL

Pick one. Every installer refuses to stack a second Relo.

- Linux & MacOS

```shell
curl -fsSL https://relo.ifelse.one/install.sh | sh
```

- Windows

```shell
irm https://relo.ifelse.one/install.ps1 | iex
```

| Channel   | Command                                                              |
|-----------|----------------------------------------------------------------------|
| Script    | the two lines above; `install.sh --help` lists the flags             |
| Homebrew  | ```brew install jonaskahn/apps/relo```                               |
| npm       | ```npm i -g @jonaskahn/relo[@beta] --foreground-scripts``` (Node 22+)  |
| macOS app | `Relo-<version>-universal.zip`                                       |
| Linux     | `Relo-Setup-<version>-amd64.deb` or `Relo-Setup-<version>-amd64.rpm` |
| Windows   | `Relo-Setup-<version>-amd64.exe`                                     |

The setup files come from the [latest release](https://github.com/jonaskahn/relo/releases/latest); the app then
updates itself. The script installs the daemon binary: it verifies the download against `checksums.txt`, frees the
ports Relo serves, and starts the daemon. The npm install downloads the same 13 MB binary and prints each step, with a
download bar showing the rate and the time left; `--foreground-scripts` keeps that output under the install instead of
hiding it.

## ▓ START

```shell
relo daemon start                 # console at http://127.0.0.1:10101
```

`daemon start` runs the proxy in the background and shows the tray icon when a desktop session is available.
`daemon run` keeps the same process in the foreground, which is what a terminal, a login item, and Relo.app use.

Loopback only. `[admin] login = true` puts a sign-in in front, using `$RELO_HOME/admin-token`.

## ▓ SURFACES

| Port    | Speaks                                |
|---------|---------------------------------------|
| `10101` | console + management API + data plane |

| Prefix              | Speaks                      |
|---------------------|-----------------------------|
| `/inference`        | OpenAI-compatible (`/v1/*`) |
| `/inference-claude` | Anthropic (`/v1/*`)         |

Client keys hit the data plane. The admin token hits the console. Provider credentials go upstream only.

## ▓ COMMANDS

```console
$ relo daemon run|start|stop|restart|status
$ relo autostart enable|disable
$ relo uninstall [--keep-data|--wipe]
$ relo version
```

`--home` and `--lang` override one run. Everything else (connections, keys, routes) is a console page.

`daemon stop --force` stops the daemon and frees the ports Relo serves, which is what an installer runs before it
replaces the binaries. A port another program holds is reported rather than ended.

## ▓ UNINSTALL

```console
$ relo uninstall            # asks whether the data goes, then removes the program
$ relo uninstall --keep-data # stops Relo, unregisters the login items, keeps the data
$ relo uninstall --wipe      # the same, and deletes $RELO_HOME and its keychain entries
```

`relo uninstall` stops the running daemon, frees every configured port, turns off start-at-login, settles the data,
and hands the program to whatever installed it: the Windows uninstaller, `apt`/`dnf` on Linux, `npm uninstall -g` on
npm, or deleting `Relo.app` on macOS. What each native path keeps:

| Path                               | Data                                   |
|------------------------------------|----------------------------------------|
| `relo uninstall`                   | asks; `y` deletes, anything else keeps |
| Windows: Settings → Apps           | asks in a dialog; No keeps             |
| `apt remove relo`                  | keeps (`apt purge relo` deletes)       |
| `rpm -e relo`                      | keeps                                  |
| `npm uninstall -g @jonaskahn/relo` | keeps                                  |

An update or reinstall always keeps the data and stops the running service first, so nothing is lost by installing
over a working install.

## ▓ CONFIG

`$RELO_HOME` (`~/.relo`) is the only env var. Settings live in `config.toml`. See [
`config.example.toml`](config.example.toml).

Secrets sit in `$RELO_HOME/secrets.enc`, sealed by `$RELO_HOME/secret.key`. `[secrets] keychain = true` uses the OS
keychain instead.

## ▓ DEV

```console
$ make build          # console + daemon → ./relo
$ make dev-setup      # writes .dev/relo, leaves ~/.relo alone
$ make ci
```

Go 1.26+, Node 22+. No CGO. See [CONTRIBUTING.md](CONTRIBUTING.md) and [docs/](docs/README.md) for the
rest, [SECURITY.md](SECURITY.md) to report a vulnerability, and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for
bundled licenses.

## ▓ LICENSE

Relo is licensed under the [GNU Affero General Public License v3.0](LICENSE). Bundled third-party software keeps its own
licenses; see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

<div align="center">
<img src="https://raw.githubusercontent.com/jonaskahn/relo/main/assets/logo.svg" alt="Relo" width="96">

<h2>Relo</h2>

<code>▚▚▚▚▚▚▚ Light • Fast • Reliable ▚▚▚▚▚▚▚</code>

<a href="https://www.npmjs.com/package/@jonaskahn/relo"><img src="https://img.shields.io/npm/v/@jonaskahn/relo?label=npm%40latest" alt="npm latest"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo?activeTab=versions"><img src="https://img.shields.io/npm/v/@jonaskahn/relo/beta" alt="npm beta"></a>

<p><i>Let your harness agents talk to any model, any provider with no config changes.</i></p>

</div>

---

## ▓ CONSOLE

| Dashboard                                                                                                                   | Connections                                                                                                               | Agents  Harness                                                                                                             |
|-----------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| <img src="https://github.com/user-attachments/assets/664efc9a-cba4-4621-a63b-6aa6972ca751" alt="Dashboard" width="320">     | <img src="https://github.com/user-attachments/assets/66da3c81-3657-4d5e-867f-f670fa76187d" alt="Connections" width="320"> | <img src="https://github.com/user-attachments/assets/87ef4d68-c7b0-4847-afb3-f35f47e45685" alt="Groups" width="320"> |

## Install

- Latest, Stable

```bash
npm i -g @jonaskahn/relo --foreground-scripts
```

- Beta, Risk

```bash
npm i -g @jonaskahn/relo@beta --foreground-scripts
```

Installs the `relo` binary, verified against the release checksums. The
install downloads the ~13 MB daemon binary and prints each step, with a
download bar showing the rate and the time left. `--foreground-scripts` keeps
that output under the install: npm otherwise runs it in the background and
shows nothing until the command ends, which makes a slow download look stuck.

State lives in `~/.relo` (`$RELO_HOME`), shared with every other installer.
On Windows the install also adds a Desktop "Relo" shortcut that restarts the
daemon without opening a terminal.
Never installed twice: if another Relo is on your PATH, this one refuses and
tells you where it found it.

## Start

```bash
relo daemon start                 # console at http://127.0.0.1:10101
```

One port serves everything: the console, the management API, and the data
plane (`/inference` speaks OpenAI, `/inference-claude` speaks Anthropic).

## Update

The installer stops the running daemon, swaps the binary, and checks the port
is free. Your data is never touched. If another program holds the port, the
install fails instead of killing it:

```
✗ INSTALL FAILED
something else is listening on port 10101
  free the port, or point relo at another one, and install again
```

## Uninstall

- Remove and Keep Data

```bash
npm uninstall -g @jonaskahn/relo
```

- Remove and Wipe Data

```bash
relo uninstall --wipe 
```

## Links

[Source](https://github.com/jonaskahn/relo) · [Releases](https://github.com/jonaskahn/relo/releases) · [Docs](https://github.com/jonaskahn/relo#readme)

AGPL-3.0-only

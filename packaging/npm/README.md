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

<div align="center">

https://github.com/user-attachments/assets/cbda11c8-b60c-413b-8a08-03144dd06e7a

<sub>Demo: using Relo in Codex</sub>
</div>

|                                                                              |                                                                        |                                                                     |
|------------------------------------------------------------------------------|------------------------------------------------------------------------|---------------------------------------------------------------------|
| Dashboard                                                                    | Connections                                                            | Groups                                                              |
| <img src="https://github.com/user-attachments/assets/ba44719e-f176-4d02-9e92-10cf7055a6e8" alt="Dashboard" width="320">        | <img src="https://github.com/user-attachments/assets/b27d9948-1f30-4aac-a0f5-d98109c5522e" alt="Connections" width="320"> | <img src="https://github.com/user-attachments/assets/29ce8e23-925f-4d22-b571-a49d8bc70841" alt="Groups" width="320">    |
| Agent Harness                                                                | Add agent                                                              | Usage                                                               |
| <img src="https://github.com/user-attachments/assets/3e5bfe37-ad48-4c12-a32a-f4901f8f982a" alt="Agent Harness" width="320"> | <img src="https://github.com/user-attachments/assets/f4730cb7-7177-40c0-8d48-905dfde29ba3" alt="Add agent" width="320">    | <img src="https://github.com/user-attachments/assets/42f329be-4d1b-408c-bc8e-87a1237eccf6" alt="Usage" width="320"> |

## Install

- Latest, Stable

```bash
npm i -g @jonaskahn/relo
```

- Beta, Risk
```bash
npm i -g @jonaskahn/relo@beta
```

Installs the `relo` binary, verified against the release checksums. State
lives in `~/.relo` (`$RELO_HOME`), shared with every other installer. Never
installed twice: if another Relo is on your PATH, this one refuses and tells
you where it found it.

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
relo: install failed -- something else is listening on port 10101
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

<div align="center">
<img src="https://raw.githubusercontent.com/jonaskahn/relo/main/assets/logo.svg" alt="Relo" width="96">

<h2>Relo</h2>

<code>▚▚▚▚▚▚▚ Light • Fast • Reliable ▚▚▚▚▚▚▚</code>

<a href="https://www.npmjs.com/package/@jonaskahn/relo"><img src="https://img.shields.io/npm/v/@jonaskahn/relo?label=npm%40latest" alt="npm latest"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo?activeTab=versions"><img src="https://img.shields.io/npm/v/@jonaskahn/relo/beta" alt="npm beta"></a>

<p><i>Let your harness agents talk to any model, any provider with no config changes.</i></p>

</div>

---

## Install

```console
$ npm i -g @jonaskahn/relo        # latest stable (Node 22+)
$ npm i -g @jonaskahn/relo@beta   # fresh from main
```

Installs the `relo` binary, verified against the release checksums. State
lives in `~/.relo` (`$RELO_HOME`), shared with every other installer. Never
installed twice: if another Relo is on your PATH, this one refuses and tells
you where it found it.

## Start

```console
$ relo daemon start                 # console at http://127.0.0.1:10101
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

```console
$ npm uninstall -g @jonaskahn/relo   # keeps your data
$ relo uninstall --wipe               # removes data too
```

## Links

[Source](https://github.com/jonaskahn/relo) · [Releases](https://github.com/jonaskahn/relo/releases) · [Docs](https://github.com/jonaskahn/relo#readme)

AGPL-3.0-only

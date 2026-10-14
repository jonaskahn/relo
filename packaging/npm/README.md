<div align="center">

```
┌───┬───┬───┬───┐
│ R │ E │ L │ O │
└───┴───┴───┴───┘
```

`npm i -g @jonaskahn/relo`

</div>

One local proxy for Codex, Claude Code, Cursor and friends. Point them at
Relo, use any model from any provider.

## Install

```console
$ npm i -g @jonaskahn/relo        # latest stable
$ npm i -g @jonaskahn/relo@beta   # fresh from main
$ relo daemon start
```

Needs Node 22+. Installs the `relo` binary, verified against the release
checksums. State lives in `~/.relo` (`$RELO_HOME`), shared with every other
installer. Never installed twice: if another Relo is on your PATH, this one
refuses and tells you where it found it.

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

## Stuck?

```console
$ node "$(npm root -g)/relo/lib/install.mjs"                         # retry
$ node "$(npm root -g)/relo/lib/install.mjs" --relo-version 0.2.0     # pin one
```

## Links

[Source](https://github.com/jonaskahn/relo) · [Releases](https://github.com/jonaskahn/relo/releases) · [Docs](https://github.com/jonaskahn/relo#readme)

AGPL-3.0-only

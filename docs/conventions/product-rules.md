# Product Rules

Rules about user-facing text, artwork, data sources, settings, and
how a request reaches a provider.

**Every user-facing string comes from a catalog.** Go text lives in
`daemon/internal/i18n/catalogs/active.<tag>.toml` and is rendered through the injected
translator; console text lives in `console/src/lib/i18n/locales/<tag>.json` and is
rendered through svelte-i18n. A literal an operator can read in two places will
eventually say two different things. Catalogs stay key-parallel: a message added to one
language is added to all of them in the same change, which the parity tests enforce.
Diagnostics are the exception — error codes, doctor check names, log fields,
and refusal details written by application use cases stay in one language so
operators, scripts, and support threads all match on the same text.

**Brand artwork has two sources, divide by their use.** `assets/logo.svg` is the
logo — the squircle plate carrying the mark — and `assets/logo-mark.svg` is the
bare mark. Everything the Dock, the installers, the launcher tiles, the favicon
every listener serves and the repo logo show the plated logo, generated from
`assets/logo.svg` by `make icons`; the tray icons derive from
`assets/logo-mark.svg`. The console copy is synced by `make sync-assets`, and
the favicon is written both into the server package that embeds it and into
`console/static`, so an inference port and the console show the same artwork.
Never edit a derived file by hand: `make icons-check` fails CI when the
committed artwork and the sources disagree.

**models.dev is network-first, with a build seed and a saved copy.** A model-facing read
asks https://models.dev/api.json, saves a validated answer to `modelsdev.json` under the
state directory, and falls back to the newest local copy when the network fails. Release
artifacts embed a dataset downloaded at build time; the local order is a valid cache at
least as new as the seed, then the seed, then an older cache. A custom `modelsdev_url`
never uses the embedded seed. `make seed` downloads the seed only when it is missing or
invalid and `make seed-latest` forces a fresh one; the generated
`daemon/internal/adapters/modelsdev/seeddata/modelsdev.json.gz` is ignored by git, and a
direct `go build` or `go test` without it still compiles, fetches, and caches normally.
Beta and release jobs run `make seed-latest` before compiling, so a download or validation
failure stops the job before artifacts exist. Pricing still resolves as user override,
then the provider, then models.dev. Protected capabilities use provider authority instead:
the selected account's discovered facts, then the connection definition, then models.dev,
and a manual completion only where all of those are silent. A manual capability can never
supersede a known definition, and stored legacy completions remain visible but inactive.
Context capacity, maximum input, and maximum output stay exact and separate; a display label
may round, but routing, admission, listings, and wire requests use the authoritative count.
An oversized client output budget is clamped silently to the provider's output ceiling and
the context left after the prompt; only a prompt that cannot fit at all is refused.

**A provider's own model list decides its models.** A connection serves the ids its model list
publishes and nothing else: a catalog entry for a model the provider does not list never
becomes a model, and neither do the ids a template used to declare. Azure OpenAI, Vertex AI,
and Kiro publish no list, so their connections take their ids from the operator and refuse
hand-written ids everywhere else. A refresh that fails keeps the roster the connection had,
and one that answers with an empty list switches the former ids off rather than deleting them.

**Settings live in the configuration file.** `$RELO_HOME/config.toml` holds every
startup setting, and `--home`, `--port`, and `--lang` override one for a single run. The
environment carries `RELO_HOME` and credentials only, so a new setting is a TOML field
with validation, never an environment variable.

**Retries live inside a connection; failover moves between connections.** A refused
account is retried `retry_attempts` times — 1, 3, or 5, stored globally with a
per-connection override — with a random 1–3s wait between tries. A wait the provider
names that is longer than that window is left to the account's own cooldown, so the
request moves on at once instead of sitting on the provider's clock. Only when those are
spent does the request reach the next account of the connection, and only when every
account has answered does it leave the connection for the next route member. A route
never retries a member of its own: one member is the connection's whole walk. A refusal
that says something about the account or the model — a 401, a 403, a model the account
may not use, or a status the connection's own switch made retryable — is not cured by
asking the same account again, so it moves on at once.

**The account and the member that served stay first.** A success pins its connection to
that account and its route to that member, so the next request starts where the last one
worked instead of rotating onto another. A conversation that named itself still wins over
both, and a pool strategy the operator picked to spread load — round-robin, random — is
left to spread it.

**A connection that failed everywhere cools down.** When every account of a connection has
failed one request, the connection waits out its next failover step before it is asked
again; a route whose every member failed waits the same way. While it waits, the request
is answered at once with 503 and a `Retry-After` naming when to come back, and a route
skips the cooling connection for its next member. Both the pins and the cooldowns live in
memory: a restart starts every connection available.

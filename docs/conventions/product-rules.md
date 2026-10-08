# Product Rules

Rules about user-facing text, artwork, data sources, and settings.

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

**models.dev is fetched, never embedded.** The daemon holds no provider catalog of its own:
it reads https://models.dev/api.json when a page or a probe needs it, saves the result to the
modelsdev.json file under the state directory, and falls back to that copy when the fetch
fails. Every model field resolves as user override, then the provider, then models.dev, so a
rate the operator set by hand is never overwritten by a price update.

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

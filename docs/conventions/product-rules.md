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

**models.dev is downloaded on a schedule, and every read serves a saved copy.** One
download asks https://models.dev/api.json, validates the answer, and saves it to
`modelsdev.json` under the state directory. Three things download: daemon start, the
24-hour discovery pass, and the connections page's update button. Nothing else does —
adding a connection, probing a key, signing in, refreshing a roster, and opening the
add picker all read the copy Relo already holds, so a slow or unreachable models.dev
can never refuse a connection or stall a page. Release artifacts embed a dataset
downloaded at build time; the local order is a valid cache at least as new as the
seed, then the seed, then an older cache. A custom `modelsdev_url`
never uses the embedded seed. `make seed` downloads the seed only when it is missing or
invalid and `make seed-latest` forces a fresh one; the generated
`daemon/internal/adapters/modelsdev/seeddata/modelsdev.json.gz` is ignored by git, and a
direct `go build` or `go test` without it still compiles, fetches, and caches normally.
Beta and release jobs run `make seed-latest` before compiling, so a download or validation
failure stops the job before artifacts exist. Pricing still resolves as user override,
then the provider, then models.dev. Protected capabilities follow the connection's Allow
overrides switch: when on, the operator's saved manual values come first, then the selected
account's discovered facts, the connection definition, and models.dev. When off, manual values
remain stored and editable but are ignored entirely; the detected layers decide. Name,
description, category, and prices keep their own always-active override behavior.
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

**A route keeps a rate limit to itself.** A 429 from a route member never reaches the
client: the route moves on to its next member, and one attempt stays reserved for every
member the walk has not reached. Only when every member is limited does the client read
a Relo-authored 429 — `rate_limit_error` on the Messages surface, `group_exhausted`
elsewhere — naming the route and, when a member named one, the wait. No provider
message travels with it.

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

**Keyless free pools require an operator opt-in on fresh installs.**
The connections.free setting in config.toml gates OpenCode Zen Free and Kilo Code Free.
A new home writes false; an existing file without the key reads as true for
upgrade compatibility. The add-connection modal explains the privacy, service,
and legal risks before enabling. Free rows stay hidden until the operator turns
on the switch in the Free heading and accepts that notice. Disabling hides the
connections from management and every published model list without deleting
their stored connections, models, or usage. Their rows remain in the routing
snapshot so a configured free model reaches the consent refusal
(`403 free_connections_disabled`) instead of `model_not_found`; a mixed route
skips that member and may still serve from a paid one.

**The OpenCode free lane authenticates the client's transport and the prompt it
carries.** It accepts only OpenCode's own Bun `fetch` TLS/HTTP fingerprint and
answers `FreeUsageLimitError` to every other client, even when the headers and
body are byte-identical. The control experiment is the same capture replayed
twice: Bun gets 200, Go and curl get 429. Relo therefore speaks the lane on a
dedicated transport that reproduces Bun's client hello (uTLS), confined to
opencode.ai; every other provider keeps the stock transport. The executor routes
only the `opencode-free` template over it, the dialer refuses any other host.
The lane reads the request as well: a request that does not carry OpenCode's own
system prompt is refused with the same 429, whatever its headers and transport
say. Relo does not reproduce that prompt. It forwards the inbound messages
unchanged, so a genuine OpenCode client passes the gate with the prompt it
brought, and a request without one — Relo's own integration test turn, for
instance — does not. Such a refusal is answered `429 free_lane_refused`, which
names the restriction instead of a quota the request cannot wait out. The
fingerprint is brittle and belongs to a deliberate access control, not a public
contract: if the gateway changes it, the lane quietly answers 429 again until
the fingerprint is relearned, and the honest long-term fix is a documented
free-lane contract upstream rather than emulation.

**A connection's quota is read the way the connection serves.** A limit decides
whether an account can serve only when it measures that account's own
allowance: a balance or a spend is a fact, a pool inside the allowance (the
meters Cursor reports beside its monthly total, the model-scoped weekly limit
Claude reports beside the weekly one) is one part spent, and a window whose
period has ended describes a period that is over. A connection serves as long
as one account can, so a connection reads Ready while any account is left, and
Limit reached only when every account of it is spent; a spent account is marked
on its own, never on the connection. The same reading decides routing: a full
pool or an ended window constrains nothing, so an account that can still serve
keeps being asked.

**A tool-free turn is forwarded without tools.** The lane used to append four
unavailable placeholder tools (`bash`, `glob`, `grep`, `read`) to every request,
on the theory that the gateway inspects declarations before answering. A
captured native title request shows the opposite: the client sent no tools, the
same body with the appended declarations was refused, and the same body with
only those declarations removed was answered 200 with a real title. The lane
therefore adds declarations only beside a turn that already declares tools, and
a turn that brought none keeps none. This is a request-preservation fix, not a
claim that the gateway accepts Codex's tool dialect.

**A spent lane's quota answer and a refused request look alike.** Both are a
429 carrying `FreeUsageLimitError`, so tell them apart before blaming the
transport. A spent lane names a `Retry-After` measured in hours (a daily
allowance shared by the whole connection). A refused request answers the same
way, and its wait counts down while a native request from the same machine
keeps passing: in the capture of 2026-11-11 the wait fell from 26,486 s to
24,373 s across calls while native opencode kept being answered 200. A moving
wait therefore proves nothing. The decisive test is a control: replay one
captured native request over both transports at the same moment. A 200 over the
fingerprint and a 429 over stock Go means the transport was the discriminator;
the same 429 from both means the allowance is spent or the request was refused
for what it carried. Relo cannot tell a spent allowance from a refused request
without such a control, so it claims neither: a refusal of the free lane fails
the request over to another provider and answers `429 free_lane_refused`, and no
lane is marked spent on a guess. A lane that is marked spent keeps its wait:
once a lane is marked spent, an account refused afterwards may lengthen the
connection's cooldown but never shorten it.

**The free lane's fingerprint is the client hello, read byte for byte.** The
gateway matches the TLS client hello before any application bytes, so the
hello has to be the native client's own, not merely a valid one. The native
hello carries a post-quantum hybrid key share: `X25519MLKEM768` is offered
first and sent as a 1216-byte key share, with X25519 following as the
fallback. A hello that offers only X25519 is a different fingerprint, and the
gateway answers it exactly as it answers an exhausted allowance — the same
`FreeUsageLimitError`, so the refusal is indistinguishable from a spent quota
or from a request without OpenCode's prompt unless the three are compared side
by side. The captured hello is the
expectation: 17 cipher suites in Bun's order, 13 extensions in Bun's order,
ALPN of http/1.1 alone, and the hybrid share first in both the offered groups
and the key shares. `opencode_hello_test.go` reads the hello off the wire and
compares it with the capture, so a drift in the spec, in uTLS, or in the cipher
list is caught locally rather than by the gateway refusing a request hours
later. The fingerprint is brittle on purpose and belongs to a deliberate access
control, not a public contract: if the gateway changes it, the lane quietly
answers 429 again until the fingerprint is relearned, and the honest long-term
fix is a documented free-lane contract upstream rather than emulation.


**Free availability is derived, not configured.** A conversational model is
free when the provider marks it free, the provider listing marks it free, both
effective input and output rates are explicitly zero, or its identifier carries
`free` as a whole token. Missing prices are unknown, not free. Media, embedding,
and unknown categories never count. Provider and listing marks are persisted as
explicit zero token rates, so availability survives a reload without a schema
change. The add picker and Connections page may filter by free availability; the
two keyless lanes are the only single-instance connections.

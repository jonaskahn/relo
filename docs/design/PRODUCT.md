# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Developers and technical operators run Relo on their own machine. They visit
the console briefly to check whether the proxy is healthy, inspect a request,
connect a provider or coding tool, adjust routing, and review usage. The
console should answer the immediate question and make the next action clear.

## Product Purpose

Relo is a local LLM proxy between AI coding clients and upstream providers.
It lets clients keep their native protocol while one local installation routes
requests to configured providers, pools provider accounts, and records usage
and cost. An operator should be able to answer what worked, what failed, and
how it is configured without reading source code or querying SQLite.

## Positioning

Relo accepts the client's own protocol and translates it for the chosen
upstream. The console and HTTP API share a daemon and state. The CLI controls
the daemon lifecycle; connections, accounts, models, routes, and client keys
are configured in the console.

## Operating Context

The management API and console run on a loopback listener at `:10101` by
default. Inference surfaces use their own configured loopback ports; the
default OpenAI-compatible and Anthropic ports are `:10201` and `:10202`.
State lives in `RELO_HOME` (default `~/.relo`), including configuration,
SQLite, the admin token, and runtime files. Provider credentials live in an
encrypted file sealed by a local key, or in the OS keychain when
`[secrets] keychain = true`.

## Capabilities and Constraints

Three credential classes serve separate roles:

- The **admin token** authenticates the management API. With `[admin] login =
  true`, the console asks for it on the sign-in page; otherwise local access
  opens the console directly. With `[admin] allow_external = true`, a
  forwarded request reaches the console only with the token, even while the
  sign-in is off. The token is stored at `$RELO_HOME/admin-token`.
- **Client keys** authenticate inbound inference requests. The console shows
  a newly created key once and stores its digest and a short hint. Keys can
  expire, be rotated, or be revoked, and identify traffic in the request log.
- **Provider credentials** are OAuth logins or API keys used upstream. They
  are never accepted as inbound client keys.

The console is a SvelteKit 2 / Svelte 5 static SPA (Vite + `adapter-static`),
embedded in the Go binary. It uses Tailwind CSS v4, Bits UI, and svelte-i18n.
The daemon is authoritative for accounts, routes, settings, and usage.

## Evidence on Hand

The repository defines the product surface. models.dev metadata is fetched and
cached; a provider's own model list determines the models its connection
serves. Do not invent customers, benchmarks, or claims about upstream behavior.

## Product Principles

- Show the machine's real state; never present an absence as success.
- Put the operator's next action close to the evidence for it.
- Make destructive actions explicit and reversible where possible.
- Keep one coherent palette and type system, so emphasis has meaning.
- Prefer a local mechanism already in the product over a new dependency.

## Accessibility & Inclusion

The console ships in 22 languages — English, German, Simplified and
Traditional Chinese, French, Spanish, Italian, Brazilian Portuguese, Dutch,
Swedish, Polish, Czech, Romanian, Russian, Ukrainian, Japanese, Korean,
Hindi, Indonesian, Vietnamese, Thai, and Turkish — each named in the
picker the way its own speakers write it.
Controls must work with a keyboard and accommodate longer translated labels.
Bits UI dialogs trap focus while open, close on Escape when dismissible, and
return focus to the invoking control. Destructive actions require
confirmation; status uses text or icons as well as color. Motion is restrained
and reduced under `prefers-reduced-motion`.

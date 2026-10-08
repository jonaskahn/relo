# Architecture

This is the required backend organization. When refactoring code that has not
moved yet, place the finished work in its owner below and remove the obsolete
package after its callers move.

```text
daemon/                       Go module
  cmd/relo/                    executable entry point
  internal/
    access/ account/ activity/ catalog/ inference/ routing/  feature core
    application/
      access/                   client-key lifecycle
      account/                  credentials, model context, pool changes
      activity/                 usage, quotas, retention, capture reads
      catalog/                  connections, models, discovery, pricing, refresh
      routing/                  route writes and previews
      integration/              coding-client setup, repair, rotation, restore
      settings/                 appearance, language, retention settings
      status/                   health and doctor views across features
    adapters/
      sqlite/ secrets/ oauth/ modelsdev/ discovery/ upstream/ wire/
      awsauth/ gcpauth/        persistence, authentication, transport
      quota/                   vendor HTTP quota probes and header parsing
      templates/               curated provider registry and sign-in metadata
      codingclients/           client config files and launchers
      antigravity/             shared vendor protocol details
    server/                    management and inference HTTP transport
    platform/                  dependency wiring, workers, daemon lifecycle
    cli/ desktop/              command line and system tray surface
    config/ i18n/ dashboard/   startup settings, messages, embedded console
    clock/                     time helpers
  tests/                       unit, integration, end-to-end, performance,
                               fixtures, and shared testkit
  tools/genicons/              icon generator
  go.mod go.sum .golangci.yml  Go dependencies and lint configuration
  .gitignore                   generated and local-file exclusions
console/                      SvelteKit console
  scripts/                     provider-logo fetcher
  src/
    routes/                    pages and redirects
    lib/                       API client, state, view logic, components, i18n
    app.css app.html app.d.ts   theme tokens, HTML shell, app types
  static/                      fonts, provider logos, favicon, synced logo
  package.json package-lock.json  npm dependencies and scripts
  svelte.config.js vite.config.ts vitest.config.ts tsconfig.json
  eslint.config.js .prettierrc .prettierignore  lint and format rules
  components.json             UI component configuration
  .npmrc .gitignore            npm policy and ignored output
docs/                          conventions and product/design context
assets/                        source artwork
scripts/                       build, asset, and verification scripts
packaging/                     release packaging assets
Makefile                       development, build, and CI targets
```

`console/build/`, `console/.svelte-kit/`, and `console/node_modules/` are
generated locally; they are not source directories.

## Architecture Rules

The feature packages own product rules and canonical types. Keep the existing
feature packages rather than adding a `features/` wrapper. Application packages
own workflows by feature and declare small interfaces beside the use cases
that need external data or effects. Adapters implement those interfaces and
translate to feature types. `server` owns request parsing, authentication,
HTTP status, and response DTOs. `platform` wires concrete implementations once.

```text
server -> application/<feature> -> feature core
adapters -> feature contracts; application and feature core never import adapters
platform -> application, adapters, server, and feature core for wiring
cli, desktop -> platform for lifecycle and application APIs for operator actions
```

| Package group                                                      | Responsibility and import boundary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
|--------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `catalog`, `account`, `routing`, `inference`, `activity`, `access` | Feature rules and types. May use the standard library, `clock`, and other feature types in an acyclic direction; never import application, adapters, server, platform, CLI, desktop, config, `database/sql`, provider SDKs, or HTTP clients.                                                                                                                                                                                                                                                                                                                                     |
| `application/<feature>`                                            | One feature's use cases and consumer-owned ports. May import feature packages and another feature's narrow contract when coordinating a workflow; never import concrete adapters, HTTP handlers, `database/sql`, or `*sql.Tx`.                                                                                                                                                                                                                                                                                                                                                   |
| `adapters/*`                                                       | Persistence, vendor protocols, provider metadata, secrets, and local client files. May import feature contracts; never decide product policy or call an application use case.                                                                                                                                                                                                                                                                                                                                                                                                    |
| `server`                                                           | HTTP listeners, handlers, sessions, middleware, API DTOs, and inference transport. Depends on application interfaces and feature types; never opens SQLite or constructs a credential store, provider client, or adapter. It reads every adapter through a port the composition root injects: `SchemaVersionSource`, `SecretModeSource`, `CallbackBroker`, `CallbackPorts`, `FormatRegistry` (extended with the `InboundRegistry` codecs), `Relay`, `CaptureRedactor`, and `Templates`. Listing refusals match on the `catalog` vocabulary, which the discovery adapter aliases. |
| `platform`                                                         | Composition root. Builds and injects dependencies, starts workers, and owns lifecycle; never imports `cli` or `desktop`.                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `cli`, `desktop`                                                   | User-facing entry points. Start the daemon through `platform` and use its application APIs or HTTP surface for operator actions.                                                                                                                                                                                                                                                                                                                                                                                                                                                 |

The core feature dependency graph must stay acyclic. `routing` may use
`catalog` and `account`; `access` may use the inference vocabulary. Cross-feature
workflows belong to an application use case that consumes a narrow contract
from its neighbor. Do not make two application packages depend on each other
in both directions.

| Rule                                        | Meaning                                                                                                                                                                                                                   |
|---------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `adapters/wire/` translates protocols       | It never opens a database or decides an HTTP response.                                                                                                                                                                    |
| `adapters/sqlite/` owns persistence         | No other package holds a `*sql.DB`, executes SQL, or exposes `*sql.Tx`.                                                                                                                                                   |
| `adapters/secrets/` owns credential storage | No other package calls an OS keychain API or reads the encrypted fallback file.                                                                                                                                           |
| `server/` owns API serialization            | Request and response DTOs, JSON tags for the management API, status codes, and error envelopes stay at the HTTP boundary.                                                                                                 |
| A consumer declares its port                | Keep interfaces small and specific to the operation needed; adapters satisfy them through Go's structural typing.                                                                                                         |
| Registration is explicit                    | No `init()` or mutable package registry; `app` constructs a registry once and passes it to consumers.                                                                                                                     |
| Abstractions need a real use                | Keep account-selection and routing strategies and the wire-format registry, which have multiple implementations. Do not add a generic repository, command bus, service locator, or interface for a single implementation. |
| Helpers have an owner                       | No grab-bag `util` package. A helper belongs beside the feature or adapter whose concept it expresses.                                                                                                                    |
| Protocol changes need fixtures              | A wire change edits the fixture deliberately, in the same commit as the wire change. `git diff --exit-code -- daemon/tests/fixtures/contract` stays clean for every compatibility-preserving change.                      |
| OAuth speaks for the vendor                 | Client identifiers, scopes, redirect addresses, and endpoints change only when the vendor changes.                                                                                                                        |

Group files by the responsibility they implement. Do not impose `ports.go`,
`commands.go`, or `queries.go` on every feature. For a write spanning several
SQLite tables, let the SQLite adapter provide one transaction-scoped operation
without leaking its transaction type. Secrets and client files cannot join a
SQLite transaction; the coordinating application use case defines the
compensation, snapshot/restore, and retry behavior.

Tighten `depguard` as each package reaches this structure: feature core rejects
application, adapter, and transport imports; application rejects concrete
adapters and transport; adapters reject application use cases and transport;
server rejects direct persistence imports. The enforced rules live in
`daemon/.golangci.yml`: feature packages, the composition root, the
transport, adapters, the adapter-free use cases, plus `server-no-adapters`
(the codec interfaces in `adapters/wire` itself stay allowed; every concrete
family is denied), `config-no-adapters`, `desktop-no-sqlite`,
`application-integration-no-adapters`, and
`application-templatesettings-no-adapters`. The tray reads activity through
the source the command line injects, and autostart lives under `platform/`,
so no `//nolint:depguard` remains. Keep the `app` rule against
importing `cli` or `desktop`. Do not mark a boundary enforced before its lint
rule exists and passes.

## Where New Code Goes

| Adding                      | Goes to                                                                                                                                                                                                                         |
|-----------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| A provider template         | `daemon/internal/adapters/templates/`, using `catalog` vocabulary                                                                                                                                                               |
| A sign-in flow              | `daemon/internal/adapters/oauth/`, plus its provider template                                                                                                                                                                   |
| An upstream wire format     | A family under `daemon/internal/adapters/wire/` and its format registry                                                                                                                                                         |
| A model listing dialect     | `catalog.ModelsFormat`, a lister in `daemon/internal/adapters/discovery/`, and the declaring template                                                                                                                           |
| A native client surface     | An inbound codec under `daemon/internal/adapters/wire/`, with its listener and HTTP mapping in `daemon/internal/server/` registered in `daemon/internal/adapters/wire/formats/` and resolved through the `InboundRegistry` port |
| A quota source              | `daemon/internal/adapters/quota/`, implementing the port owned by `activity/`                                                                                                                                                   |
| A management operation      | The owning `daemon/internal/application/<feature>/` package, with an HTTP DTO and handler in `daemon/internal/server/`                                                                                                          |
| A coding client integration | Orchestration in `daemon/internal/application/integration/`; file and launcher behavior in `daemon/internal/adapters/codingclients/` joined by the client ports the use cases declare                                           |
| A background worker         | `daemon/internal/platform/`, started with the daemon lifecycle                                                                                                                                                                  |
| A console page              | A route in `console/src/routes/`, components in `console/src/lib/components/<area>/`, and view logic with its test in `console/src/lib/`                                                                                        |

Refactor one feature at a time. Move its behavior and tests together, remove
forwarding packages once their callers move, and keep API paths, JSON fields,
SQLite migrations, credential formats, and native client protocols compatible.
Update `daemon/tests/coverage.tsv` for each new package and add a `depguard`
rule when the corresponding boundary can pass. Run `make ci-backend` after each
slice, then `make ci-coverage` and `make ci-cross-build` when the structure
changes across packages or platforms.

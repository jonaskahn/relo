# Management API Conventions

The management API lives under `/api/v1` and is read by the console, the tray
app, and scripts.

**The resources are `connections`, `routes`, `clients/keys`, and
`activity`**; `accounts`, `models`, `modelsdev`, `catalog`, `templates`,
`settings`, `status`, and `doctor` answer on paths of their own. `POST
/api/v1/catalog/refresh` is the one update the console offers: it downloads
the catalog and reads every connected provider's model list again.

**JSON fields are snake_case.** Timestamps are RFC 3339 UTC strings. Money is
an integer number of USD micros per million tokens.

**A collection answers with `{"items": [...]}`**, plus `next_cursor` when the
read is paginated. A write answers with the object it changed.

**Errors carry a stable code and a localized message:**

```json
{ "error": { "code": "not_found", "message": "no such connection" } }
```

The code never changes; the message is rendered from the catalogs in the
language the request asked for, falling back to the operator's language.

**Every write needs `X-CSRF-Token`** matching the `relo_csrf` cookie, and a
request body is a single JSON object of at most 1 MiB with no unknown fields.

**`server` owns the API wire shape.** Application use cases return typed
results without management API JSON tags; handlers map them to request and
response DTOs, choose HTTP status codes, and write error envelopes. Adding a
domain field does not expose it on the API until the handler maps it.

**Free availability is reported where a picker chooses a connection.** Each
`GET /api/v1/templates` item carries `free_models`, and every connection from
`GET /api/v1/connections` (or its detail read) carries
`counts.free_models`. Probe models carry `is_free`. These counts cover
conversational models only. The values are derived; they do not add config
fields or stored rows.

**Connections carry their curated dashboard.** Every `GET /api/v1/connections`
item, its detail read, and each `GET /api/v1/templates` item carries
`dashboard_url`. The value comes from the curated template, so an unknown,
custom, or local connection reports an empty string and a changed curated URL
applies without a migration.

**Activity request rows carry routing history.** `GET /api/v1/activity/requests`
accepts a `group` query parameter and returns `GroupID`, `Attempts`, and
`Retried` on each row. The group filter reads retained request rows; archived
daily totals have no group dimension and refuse the filter.

**The daemon log can be exported.** `GET /api/v1/logs/daemon/export` streams a
ZIP containing the current `daemon.log` as `relo-daemon-log-<UTC timestamp>.zip`.

**The event stream is long-lived.** `GET
/api/v1/events/logs,quota,status` stays open for as long as the console is
visible. A browser allows about six HTTP/1.1 connections per origin, so every
open stream counts against the same pool as the console's bundles and API
reads. Keep one stream per browser, close it while the tab is hidden, and do
not open one per tab. Outside debugging, leave `[system.logging] traffic` at
`false`: `true` writes every agent and provider exchange in full and can grow
the traffic log by gigabytes per day.

**Two free refusals are stable.** A probe or commit that would create a second
keyless free connection answers `409 free_connection_exists` and writes
nothing. Inference that reaches a configured free connection while the opt-in is
off answers `403 free_connections_disabled`; the OpenAI-compatible and Responses
surfaces use the management error envelope, while Anthropic uses
`permission_error` and the same localized message. A route may skip that member
and continue to a paid one.

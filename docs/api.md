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

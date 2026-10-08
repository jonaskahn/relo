-- 0012_request_capture.sql
-- Request capture keeps the agent's own HTTP request, every request Relo sent
-- to a provider account, and what the provider answered, beside the usage row
-- they belong to. Bodies are stored as BLOBs so a binary body or a partial
-- stream survives a round trip unchanged.
--
-- Daily aggregates hold whole UTC days of usage, keyed by the dimensions the
-- console filters and groups by, so totals survive the deletion of the
-- request rows they were computed from.

ALTER TABLE usage_attempts ADD COLUMN credential_id TEXT;

-- The attempt count and whether a request retried are sums a rollup reads, so
-- they live on the event rather than being counted per read.
ALTER TABLE usage_events ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_events ADD COLUMN retried INTEGER NOT NULL DEFAULT 0;

UPDATE usage_events SET attempts = (
    SELECT count(*) FROM usage_attempts WHERE usage_attempts.event_id = usage_events.id
);
UPDATE usage_events SET retried = CASE WHEN attempts > 1 THEN 1 ELSE 0 END;

CREATE TABLE usage_captures (
    id              INTEGER PRIMARY KEY,
    event_id        INTEGER NOT NULL REFERENCES usage_events(id) ON DELETE CASCADE,
    ordinal         INTEGER,
    kind            TEXT NOT NULL,
    method          TEXT NOT NULL DEFAULT '',
    url             TEXT NOT NULL DEFAULT '',
    status          INTEGER NOT NULL DEFAULT 0,
    headers         TEXT NOT NULL DEFAULT '{}',
    body            BLOB,
    body_bytes      INTEGER NOT NULL DEFAULT 0,
    truncated       INTEGER NOT NULL DEFAULT 0,
    created_at      INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_captures_event ON usage_captures(event_id, kind, ordinal);

-- usage_days names every day that holds an aggregate and whether the day is
-- closed: a finalized day is read from usage_daily, and its request rows may
-- be deleted. A day that is not finalized is read from the request log.
CREATE TABLE usage_days (
    day         TEXT NOT NULL PRIMARY KEY,
    finalized   INTEGER NOT NULL DEFAULT 0,
    computed_at INTEGER NOT NULL DEFAULT 0
) STRICT;

-- usage_daily is one aggregate per UTC day and per combination of the
-- dimensions the console filters and groups by. Status is the exact code, so
-- a class filter (4xx) and a named code (429) both stay exact.
CREATE TABLE usage_daily (
    day                TEXT NOT NULL,
    provider           TEXT NOT NULL DEFAULT '',
    model              TEXT NOT NULL DEFAULT '',
    account_id         TEXT NOT NULL DEFAULT '',
    account_label      TEXT NOT NULL DEFAULT '',
    client_id          TEXT NOT NULL DEFAULT '',
    client_name        TEXT NOT NULL DEFAULT '',
    origin             TEXT NOT NULL DEFAULT '',
    surface            TEXT NOT NULL DEFAULT '',
    status             INTEGER NOT NULL DEFAULT 0,
    requests           INTEGER NOT NULL DEFAULT 0,
    input_tokens       INTEGER NOT NULL DEFAULT 0,
    output_tokens      INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    cost_micros        INTEGER NOT NULL DEFAULT 0,
    unpriced_requests  INTEGER NOT NULL DEFAULT 0,
    duration_ms        INTEGER NOT NULL DEFAULT 0,
    duration_max_ms    INTEGER NOT NULL DEFAULT 0,
    attempts           INTEGER NOT NULL DEFAULT 0,
    retried_requests   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, provider, model, account_id, account_label, client_id, client_name, origin, surface, status)
) STRICT;

CREATE INDEX idx_usage_daily_day ON usage_daily(day);

-- Outbound proxy pool. Passwords live in the secret store under proxy/<id>;
-- this table deliberately excludes the canonical URL and any secret value.
CREATE TABLE IF NOT EXISTS proxies (
	id          TEXT    NOT NULL PRIMARY KEY,
	label       TEXT    NOT NULL,
	scheme      TEXT    NOT NULL CHECK (scheme IN ('http', 'https', 'socks5')),
	host        TEXT    NOT NULL,
	port        INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
	username    TEXT    NOT NULL DEFAULT '',
	secret_ref  TEXT    NOT NULL DEFAULT '',
	enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
	rank        INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
) STRICT;

CREATE INDEX IF NOT EXISTS proxies_enabled_rank ON proxies(enabled, rank);

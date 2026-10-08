-- A client key an integration owns. The integration mints it, keeps its raw
-- value in a mode-0600 file so a launcher can read it while the daemon is
-- down, and is the only surface that may rotate or revoke it. An empty owner
-- is an operator's own key, which behaves exactly as before.
ALTER TABLE access_keys ADD COLUMN owner TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_access_keys_owner ON access_keys(owner) WHERE owner <> '';

-- One row per coding agent Relo knows how to wire. enabled is the operator's
-- intent, state is what the last write actually achieved, and key_id names
-- the client key the integration owns.
CREATE TABLE integrations (
    id         TEXT PRIMARY KEY,
    enabled    INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0,1)),
    key_id     TEXT NOT NULL DEFAULT '',
    state      TEXT NOT NULL DEFAULT 'off' CHECK (state IN ('off','on','drifted','error')),
    last_error TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
) STRICT;

-- Every file Relo wrote for an integration, what it put there, and the
-- snapshot taken before the first write, which is what makes a drift check
-- and a restore answerable.
CREATE TABLE integration_files (
    integration_id TEXT NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL,
    path           TEXT NOT NULL DEFAULT '',
    digest         TEXT NOT NULL DEFAULT '',
    snapshot_path  TEXT NOT NULL DEFAULT '',
    updated_at     INTEGER NOT NULL,
    PRIMARY KEY (integration_id, kind)
) STRICT;

-- What each action did, in order, so an operator can see the history of a
-- machine's wiring and undo the last change.
CREATE TABLE integration_ops (
    id             TEXT PRIMARY KEY,
    integration_id TEXT NOT NULL,
    action         TEXT NOT NULL,
    detail         TEXT NOT NULL DEFAULT '{}',
    created_at     INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_integration_ops ON integration_ops(integration_id, created_at);

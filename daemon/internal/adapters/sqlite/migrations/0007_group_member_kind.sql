-- A route member now names either one connection's model, as it always did,
-- or a bare model identifier the router resolves against every connection
-- that serves it. The bare form carries no provider, so the foreign key to
-- models can no longer hold for every row; SQLite cannot drop a foreign key
-- in place, so the table is rebuilt without it. Model deletion stays blocked
-- by the service, which refuses to remove a model a route still names.
PRAGMA defer_foreign_keys = ON;

CREATE TABLE model_group_members_new (
    group_id    TEXT NOT NULL REFERENCES model_groups(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL CHECK (position >= 0),
    provider_id TEXT NOT NULL,
    model_id    TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT 'model' CHECK (kind IN ('model','auto')),
    weight      INTEGER NOT NULL DEFAULT 1 CHECK (weight BETWEEN 1 AND 1000),
    enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    PRIMARY KEY (group_id, position),
    UNIQUE (group_id, provider_id, model_id)
) STRICT;

INSERT INTO model_group_members_new (group_id, position, provider_id, model_id, kind, weight, enabled)
SELECT group_id, position, provider_id, model_id, 'model', weight, enabled FROM model_group_members;

DROP TABLE model_group_members;

ALTER TABLE model_group_members_new RENAME TO model_group_members;

CREATE INDEX idx_group_members_model ON model_group_members(provider_id, model_id);

-- Which models one account can serve. A provider publishes one roster per
-- connection, but an account's own entitlement can be narrower: a free
-- ChatGPT login does not reach every model a Pro login does. A row here is
-- written only for a pair Relo actually observed, from the account's own
-- listing or from an upstream refusal that named the model.
CREATE TABLE credential_models (
    credential_id TEXT NOT NULL,
    model_id      TEXT NOT NULL,
    source        TEXT NOT NULL CHECK (source IN ('listing','learned')),
    available     INTEGER NOT NULL DEFAULT 1 CHECK (available IN (0,1)),
    observed_at   INTEGER NOT NULL,
    PRIMARY KEY (credential_id, model_id)
) STRICT;

CREATE INDEX idx_credential_models_model ON credential_models(model_id);

-- The marker that says an account's roster is known. Without it, the absence
-- of a credential_models row means "never observed" rather than "not
-- entitled", and an account whose provider publishes no per-account roster
-- keeps serving every model its connection lists.
CREATE TABLE credential_rosters (
    credential_id TEXT PRIMARY KEY,
    source        TEXT NOT NULL CHECK (source IN ('listing','learned')),
    models_count  INTEGER NOT NULL DEFAULT 0,
    observed_at   INTEGER NOT NULL
) STRICT;

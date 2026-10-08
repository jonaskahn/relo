-- The one schema Relo ships. Providers, models, and groups are the runtime
-- catalog: the operator adds providers and models, models.dev enriches details
-- and pricing on demand, and the daemon reads the whole thing into memory at
-- startup. Timestamps are Unix milliseconds.
CREATE TABLE providers (
    id                    TEXT PRIMARY KEY,
    template_id           TEXT NOT NULL DEFAULT '',
    origin                TEXT NOT NULL CHECK (origin IN ('template','signin','custom')),
    label                 TEXT NOT NULL,
    auth                  TEXT NOT NULL CHECK (auth IN ('oauth','api_key','aws','gcp','none')),
    api_format            TEXT NOT NULL DEFAULT ''
                          CHECK (api_format IN ('','openai-chat','openai-responses','anthropic','vertex-anthropic','gemini','vertex','cloud-code-assist','bedrock-converse','kiro')),
    key_header            TEXT NOT NULL DEFAULT 'bearer'
                          CHECK (key_header IN ('bearer','x-api-key','x-goog-api-key','api-key','none')),
    base_url              TEXT NOT NULL DEFAULT '',
    models_source         TEXT NOT NULL DEFAULT 'listing'
                          CHECK (models_source IN ('listing','modelsdev','manual')),
    models_format         TEXT NOT NULL DEFAULT 'none'
                          CHECK (models_format IN ('openai','anthropic','gemini','antigravity','bedrock','kiro','none')),
    modelsdev_provider_id TEXT NOT NULL DEFAULT '',
    headers               TEXT NOT NULL DEFAULT '{}',
    doc_url               TEXT NOT NULL DEFAULT '',
    key_env               TEXT NOT NULL DEFAULT '[]',
    login_flows           TEXT NOT NULL DEFAULT '[]',
    variables             TEXT NOT NULL DEFAULT '{}',
    enabled               INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    rank                  INTEGER NOT NULL DEFAULT 100 CHECK (rank BETWEEN 0 AND 1000),
    pool_strategy         TEXT NOT NULL DEFAULT 'least-loaded'
                          CHECK (pool_strategy IN ('round-robin','least-loaded','random')),
    last_refreshed_at     INTEGER,
    last_refresh_error    TEXT NOT NULL DEFAULT '',
    created_at            INTEGER NOT NULL,
    updated_at            INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_providers_origin ON providers(origin);

CREATE TABLE models (
    provider_id           TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    model_id              TEXT NOT NULL,
    source                TEXT NOT NULL DEFAULT 'listing'
                          CHECK (source IN ('listing','modelsdev','manual')),
    api_format            TEXT NOT NULL DEFAULT ''
                          CHECK (api_format IN ('','openai-chat','openai-responses','anthropic','vertex-anthropic','gemini','vertex','cloud-code-assist','bedrock-converse','kiro')),
    base_url              TEXT NOT NULL DEFAULT '',
    modelsdev_ref         TEXT NOT NULL DEFAULT '',
    match                 TEXT NOT NULL DEFAULT 'none'
                          CHECK (match IN ('exact','normalized','vendor','manual','none')),
    enabled               INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    available             INTEGER CHECK (available IN (0,1)),
    listed_at             INTEGER,
    updated_at            INTEGER NOT NULL,
    PRIMARY KEY (provider_id, model_id)
) STRICT;

CREATE INDEX idx_models_model_id ON models(model_id);

CREATE TABLE model_facts (
    provider_id           TEXT NOT NULL,
    model_id              TEXT NOT NULL,
    layer                 TEXT NOT NULL CHECK (layer IN ('override','provider','modelsdev')),
    name                  TEXT,
    description           TEXT,
    family                TEXT,
    category              TEXT,
    context_window        INTEGER,
    max_input             INTEGER,
    max_output            INTEGER,
    supports_tools        INTEGER CHECK (supports_tools IN (0,1)),
    supports_reasoning    INTEGER CHECK (supports_reasoning IN (0,1)),
    supports_vision       INTEGER CHECK (supports_vision IN (0,1)),
    status                TEXT,
    release_date          TEXT,
    input_price           INTEGER,
    output_price          INTEGER,
    cache_read_price      INTEGER,
    cache_write_price     INTEGER,
    ext_threshold         INTEGER,
    ext_input_price       INTEGER,
    ext_output_price      INTEGER,
    ext_cache_read_price  INTEGER,
    ext_cache_write_price INTEGER,
    PRIMARY KEY (provider_id, model_id, layer),
    FOREIGN KEY (provider_id, model_id) REFERENCES models(provider_id, model_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX idx_model_facts_model ON model_facts(provider_id, model_id);

CREATE TABLE model_groups (
    id         TEXT PRIMARY KEY,
    label      TEXT NOT NULL DEFAULT '',
    strategy   TEXT NOT NULL CHECK (strategy IN ('priority','round-robin','weighted','cheapest','fastest')),
    enabled    INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    listed     INTEGER NOT NULL DEFAULT 1 CHECK (listed IN (0,1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE model_group_members (
    group_id    TEXT NOT NULL REFERENCES model_groups(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL CHECK (position >= 0),
    provider_id TEXT NOT NULL,
    model_id    TEXT NOT NULL,
    weight      INTEGER NOT NULL DEFAULT 1 CHECK (weight BETWEEN 1 AND 1000),
    enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    PRIMARY KEY (group_id, position),
    UNIQUE (group_id, provider_id, model_id),
    FOREIGN KEY (provider_id, model_id) REFERENCES models(provider_id, model_id) ON DELETE RESTRICT
) STRICT;

CREATE INDEX idx_group_members_model ON model_group_members(provider_id, model_id);

CREATE TABLE modelsdev_state (
    id                    INTEGER PRIMARY KEY CHECK (id = 1),
    source_url            TEXT NOT NULL,
    fetched_at            INTEGER NOT NULL,
    etag                  TEXT NOT NULL DEFAULT '',
    last_modified         TEXT NOT NULL DEFAULT '',
    providers_count       INTEGER NOT NULL DEFAULT 0,
    models_count          INTEGER NOT NULL DEFAULT 0,
    last_attempt_at       INTEGER NOT NULL DEFAULT 0,
    last_error            TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE credentials (
    id                TEXT PRIMARY KEY,
    provider_id       TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    kind              TEXT NOT NULL CHECK (kind IN ('oauth','api_key','aws_keys','gcp_service_account')),
    label             TEXT NOT NULL DEFAULT 'default',
    email             TEXT,
    plan              TEXT,
    secret_ref        TEXT,
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','paused','needs_reauth')),
    priority          INTEGER NOT NULL DEFAULT 0,
    quota_history_id  TEXT,
    generation        INTEGER NOT NULL DEFAULT 0,
    deleted_at        INTEGER,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_creds_provider ON credentials(provider_id) WHERE deleted_at IS NULL;

CREATE TABLE usage_events (
    id                   INTEGER PRIMARY KEY,
    request_id           TEXT,
    timestamp            INTEGER NOT NULL,
    provider             TEXT NOT NULL,
    model                TEXT NOT NULL,
    requested_model      TEXT NOT NULL DEFAULT '',
    group_id             TEXT,
    credential_label     TEXT,
    surface              TEXT,
    status               INTEGER NOT NULL,
    duration_ms          INTEGER NOT NULL,
    input_tokens         INTEGER NOT NULL DEFAULT 0,
    output_tokens        INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens    INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens   INTEGER NOT NULL DEFAULT 0,
    estimated_cost_micros INTEGER,
    route_provider       TEXT,
    route_reason         TEXT,
    client_key_id        TEXT,
    client_key_name      TEXT,
    extra                TEXT NOT NULL DEFAULT '{}'
) STRICT;

CREATE INDEX idx_usage_ts ON usage_events(timestamp);
CREATE INDEX idx_usage_provider ON usage_events(provider, timestamp);
CREATE INDEX idx_usage_client ON usage_events(client_key_id, timestamp);

CREATE TABLE usage_attempts (
    id               INTEGER PRIMARY KEY,
    event_id         INTEGER NOT NULL REFERENCES usage_events(id) ON DELETE CASCADE,
    ordinal          INTEGER NOT NULL,
    provider         TEXT NOT NULL,
    model            TEXT NOT NULL,
    credential_label TEXT NOT NULL DEFAULT '',
    status           INTEGER NOT NULL,
    error_code       TEXT NOT NULL DEFAULT '',
    duration_ms      INTEGER NOT NULL,
    input_tokens     INTEGER NOT NULL DEFAULT 0,
    output_tokens    INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE INDEX idx_attempts_event ON usage_attempts(event_id, ordinal);

CREATE TABLE quota_snapshots (
    credential_id TEXT NOT NULL,
    window        TEXT NOT NULL,
    used_percent  REAL,
    reset_at      INTEGER,
    source        TEXT,
    updated_at    INTEGER NOT NULL,
    PRIMARY KEY (credential_id, window)
) STRICT;

CREATE TABLE quota_history (
    credential_id TEXT NOT NULL,
    window        TEXT NOT NULL,
    observed_at   INTEGER NOT NULL,
    percent       REAL NOT NULL,
    PRIMARY KEY (credential_id, window, observed_at)
) STRICT;

CREATE INDEX idx_quota_history_age ON quota_history(observed_at);

CREATE TABLE retention_config (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    usage_days INTEGER NOT NULL DEFAULT 0 CHECK (usage_days >= 0),
    max_events INTEGER NOT NULL DEFAULT 0 CHECK (max_events >= 0),
    max_bytes  INTEGER NOT NULL DEFAULT 0 CHECK (max_bytes >= 0),
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE access_keys (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('agent','shared')),
    client        TEXT NOT NULL DEFAULT '',
    token_digest  TEXT NOT NULL,
    token_hint    TEXT NOT NULL,
    generation    INTEGER NOT NULL DEFAULT 1,
    expires_at    INTEGER,
    revoked_at    INTEGER,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    last_used_at  INTEGER
) STRICT;

CREATE UNIQUE INDEX idx_access_keys_name
    ON access_keys(lower(name)) WHERE revoked_at IS NULL;

CREATE UNIQUE INDEX idx_access_keys_digest
    ON access_keys(token_digest);

CREATE INDEX idx_access_keys_created ON access_keys(created_at);

CREATE TABLE ui_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    language   TEXT NOT NULL DEFAULT 'auto',
    updated_at INTEGER NOT NULL
) STRICT;

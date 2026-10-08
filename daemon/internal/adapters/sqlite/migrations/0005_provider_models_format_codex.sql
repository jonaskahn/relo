-- The Codex sign-in connection publishes its model roster in a dialect of
-- its own, which the providers CHECK did not admit, so the row could never be
-- written and the console reported a provider its own template offered as
-- missing. SQLite cannot alter a CHECK, so the table is rebuilt with the one
-- value added.
--
-- DROP TABLE on a parent fires the children's ON DELETE actions, which would
-- take every model, credential, and route member with it. The children are
-- therefore copied aside, emptied, and put back once the new table holds the
-- same ids; model_group_members is cleared first because its foreign key to
-- models is RESTRICT rather than CASCADE.
PRAGMA defer_foreign_keys = ON;

CREATE TABLE providers_new (
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
                          CHECK (models_format IN ('openai','anthropic','gemini','antigravity','bedrock','kiro','codex','none')),
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

INSERT INTO providers_new SELECT * FROM providers;

CREATE TABLE models_migration_backup AS SELECT * FROM models;
CREATE TABLE model_facts_migration_backup AS SELECT * FROM model_facts;
CREATE TABLE credentials_migration_backup AS SELECT * FROM credentials;
CREATE TABLE group_members_migration_backup AS SELECT * FROM model_group_members;

DELETE FROM model_group_members;
DELETE FROM model_facts;
DELETE FROM models;
DELETE FROM credentials;

DROP TABLE providers;

ALTER TABLE providers_new RENAME TO providers;

CREATE INDEX idx_providers_origin ON providers(origin);

INSERT INTO models SELECT * FROM models_migration_backup;
INSERT INTO model_facts SELECT * FROM model_facts_migration_backup;
INSERT INTO credentials SELECT * FROM credentials_migration_backup;
INSERT INTO model_group_members SELECT * FROM group_members_migration_backup;

DROP TABLE models_migration_backup;
DROP TABLE model_facts_migration_backup;
DROP TABLE credentials_migration_backup;
DROP TABLE group_members_migration_backup;

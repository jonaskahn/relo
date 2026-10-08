-- The interface language now lives in config.toml (ui.language), so the
-- table keeps only what the console stores in the database: how a quota
-- chart reads.
DROP TABLE IF EXISTS ui_settings;

CREATE TABLE ui_settings (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    quota_display TEXT NOT NULL CHECK (quota_display IN ('used', 'remaining')),
    updated_at    INTEGER NOT NULL
) STRICT;

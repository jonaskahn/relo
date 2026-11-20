-- How long a continued Claude thread stays recoverable.
CREATE TABLE IF NOT EXISTS thread_retention_config (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    days       INTEGER NOT NULL DEFAULT 30 CHECK (days >= 0),
    updated_at INTEGER NOT NULL
);


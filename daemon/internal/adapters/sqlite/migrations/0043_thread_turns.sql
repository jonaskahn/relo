-- Thread turns: the conversations a client leaves on the server.
CREATE TABLE IF NOT EXISTS thread_turns (
    client_key TEXT NOT NULL,
    id TEXT NOT NULL,
    parent_id TEXT NOT NULL DEFAULT '',
    system TEXT NOT NULL DEFAULT '',
    tools TEXT NOT NULL DEFAULT '',
    delta TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (client_key, id)
);

CREATE INDEX IF NOT EXISTS thread_turns_updated_at ON thread_turns (updated_at);


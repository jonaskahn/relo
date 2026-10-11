-- Cursor request configurations pin one account's exact request tuple per
-- group and selection, so account selection checks the requested effort
-- rather than only the grouped model id.
CREATE TABLE IF NOT EXISTS cursor_model_configs (
	provider_id TEXT NOT NULL,
	credential_id TEXT NOT NULL,
	group_id TEXT NOT NULL,
	selection TEXT NOT NULL,
	effort TEXT NOT NULL DEFAULT '',
	is_default INTEGER NOT NULL DEFAULT 0,
	model_id TEXT NOT NULL,
	details_id TEXT NOT NULL,
	max_mode INTEGER NOT NULL DEFAULT 0,
	params TEXT NOT NULL DEFAULT '[]',
	context_window INTEGER NOT NULL DEFAULT 0,
	images INTEGER NOT NULL DEFAULT 0,
	reasoning INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (provider_id, credential_id, group_id, selection)
);
CREATE INDEX IF NOT EXISTS idx_cursor_model_configs_credential
	ON cursor_model_configs (credential_id);

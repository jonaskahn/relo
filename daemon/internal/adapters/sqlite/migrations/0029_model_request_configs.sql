-- Account request configurations hold one account's exact request tuple per
-- grouped model and selection, so account selection checks the requested
-- selection rather than only the grouped model id. The wire payload belongs
-- to the owning codec; storage never reads inside it.
CREATE TABLE IF NOT EXISTS model_request_configs (
	provider_id TEXT NOT NULL,
	credential_id TEXT NOT NULL,
	model_id TEXT NOT NULL,
	selection TEXT NOT NULL,
	effort TEXT NOT NULL DEFAULT '',
	is_default INTEGER NOT NULL DEFAULT 0,
	api_format TEXT NOT NULL DEFAULT '',
	wire_config TEXT NOT NULL DEFAULT '{}',
	context_window INTEGER NOT NULL DEFAULT 0,
	images INTEGER NOT NULL DEFAULT 0,
	reasoning INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (provider_id, credential_id, model_id, selection)
);
CREATE INDEX IF NOT EXISTS idx_model_request_configs_credential
	ON model_request_configs (credential_id);

-- Cursor tuples move into the shared shape: the exact triple becomes the
-- cursor-agent wire payload, and the params array is re-embedded as JSON
-- rather than as text.
INSERT OR IGNORE INTO model_request_configs
	(provider_id, credential_id, model_id, selection, effort, is_default,
	 api_format, wire_config, context_window, images, reasoning)
SELECT provider_id, credential_id, group_id, selection, effort, is_default,
	'cursor-agent',
	json_object('model_id', model_id, 'details_id', details_id,
		'max_mode', CASE WHEN max_mode = 1 THEN json('true') ELSE json('false') END,
		'params', json(CASE WHEN params = '' THEN '[]' ELSE params END)),
	context_window, images, reasoning
FROM cursor_model_configs;

DROP TABLE IF EXISTS cursor_model_configs;

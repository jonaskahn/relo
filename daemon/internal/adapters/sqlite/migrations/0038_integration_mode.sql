-- How one integration points its client at Relo. env is the settings-file
-- mode every existing row was wired in: the client's own configuration names
-- Relo's data plane. proxy is written by an operator who wants the client to
-- keep talking to the vendor's host through Relo's intercept instead.
ALTER TABLE integrations ADD COLUMN mode TEXT NOT NULL DEFAULT 'env';

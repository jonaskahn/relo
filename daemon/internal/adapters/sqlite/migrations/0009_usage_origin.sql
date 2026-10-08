-- usage_events gains the origin of a request: "external" for traffic a
-- client sent through the data plane, and "internal" for a test the console
-- ran from the Integrations page. Every row written before this column
-- existed is external traffic, and the default keeps a writer that never set
-- it on the same side.
ALTER TABLE usage_events ADD COLUMN origin TEXT NOT NULL DEFAULT 'external';

-- The console filters the request log by origin, newest first, so the index
-- leads with the column the filter names.
CREATE INDEX idx_usage_origin ON usage_events(origin, timestamp);

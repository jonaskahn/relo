-- A connection overrides the relay's call wait and retry windows; NULL means
-- it uses the global values stored in config.toml. timeout_seconds is the
-- wait for the next byte, retry_backoff the three low–high second ranges as
-- JSON, for example [[1,3],[3,5],[5,10]].
ALTER TABLE providers ADD COLUMN timeout_seconds INTEGER;
ALTER TABLE providers ADD COLUMN retry_backoff TEXT;

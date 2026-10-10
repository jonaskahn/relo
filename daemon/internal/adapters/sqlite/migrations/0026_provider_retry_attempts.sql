-- A connection overrides how many times one account is retried after a
-- retryable failure; NULL means it uses the global value stored in
-- config.toml. The random retry waits are no longer configurable, so the
-- per-connection retry windows go.
ALTER TABLE providers ADD COLUMN retry_attempts INTEGER;
ALTER TABLE providers DROP COLUMN retry_backoff;

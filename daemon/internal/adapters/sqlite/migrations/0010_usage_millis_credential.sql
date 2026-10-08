-- usage_events.timestamp was written as Unix SECONDS by the runtime writer
-- while every reader treats the column as milliseconds: the age retention
-- cutoff, the since/until filters, and the per-day grouping. The mismatch made
-- a date filter match nothing and an age budget delete every row. The rows
-- written before the writer was corrected carry a second-scale value, which is
-- always below 1e11, so those are converted and every later write stays in
-- milliseconds.
UPDATE usage_events SET timestamp = timestamp * 1000 WHERE timestamp > 0 AND timestamp < 100000000000;

-- The account that served a request is stored by its own identifier so two
-- accounts that share a display label stay distinct in a rollup. Rows written
-- before this column existed keep a NULL, which reads as unattributed rather
-- than as some other account.
ALTER TABLE usage_events ADD COLUMN credential_id TEXT;

CREATE INDEX idx_usage_credential ON usage_events(credential_id, timestamp);

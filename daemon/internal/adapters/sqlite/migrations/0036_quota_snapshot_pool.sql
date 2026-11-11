-- A quota window now says whether it measures one part of an account's
-- allowance or the allowance itself. A provider that reports an overall
-- limit beside the pools it draws from states the pools this way, so a full
-- pool is not a spent account.
ALTER TABLE quota_snapshots ADD COLUMN pool INTEGER NOT NULL DEFAULT 0;

-- A quota window now carries how long it lasts. A provider that reports only
-- a duration still gets a window an operator recognises, and zero keeps a
-- reading whose length the provider never stated.
ALTER TABLE quota_snapshots ADD COLUMN window_seconds INTEGER NOT NULL DEFAULT 0;

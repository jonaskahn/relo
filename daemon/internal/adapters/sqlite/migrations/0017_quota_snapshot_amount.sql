ALTER TABLE quota_snapshots ADD COLUMN amount REAL;
ALTER TABLE quota_snapshots ADD COLUMN currency TEXT NOT NULL DEFAULT '';

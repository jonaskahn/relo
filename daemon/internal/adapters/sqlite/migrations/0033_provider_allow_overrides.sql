-- Connections start with manual capability overrides enabled, which keeps the
-- behavior existing installations already had.
ALTER TABLE providers ADD COLUMN allow_overrides INTEGER NOT NULL DEFAULT 1
    CHECK (allow_overrides IN (0, 1));

-- A free-only connection serves only models that cost nothing. The flag is
-- set at commit and enforced on every roster update and model listing.
ALTER TABLE providers ADD COLUMN free_only INTEGER NOT NULL DEFAULT 0 CHECK (free_only IN (0, 1));

-- The fingerprint of the model list Relo last wrote for one integration.
-- Inspect compares it to the list Relo would write now, so a catalog change
-- can offer Repair on a client whose files Relo does not rewrite.
ALTER TABLE integrations ADD COLUMN models_digest TEXT NOT NULL DEFAULT '';

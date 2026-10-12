-- Template model prefixes hold the families one template's roster keeps, so a
-- connection serves only the ids an operator routes to. Empty keeps the
-- template default.
ALTER TABLE template_settings ADD COLUMN model_prefixes TEXT NOT NULL DEFAULT '';

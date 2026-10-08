-- A connection and a route may fail over on an unlisted 4xx or 5xx as well as
-- the statuses the relay always retries. NULL is the shipped default: it does.
ALTER TABLE providers ADD COLUMN switch_on_4xx INTEGER CHECK (switch_on_4xx IN (0,1));
ALTER TABLE providers ADD COLUMN switch_on_5xx INTEGER CHECK (switch_on_5xx IN (0,1));
ALTER TABLE model_groups ADD COLUMN switch_on_4xx INTEGER CHECK (switch_on_4xx IN (0,1));
ALTER TABLE model_groups ADD COLUMN switch_on_5xx INTEGER CHECK (switch_on_5xx IN (0,1));

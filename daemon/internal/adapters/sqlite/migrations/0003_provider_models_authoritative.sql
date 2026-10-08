-- A connection's own model list is what decides which models it serves, so a
-- row that arrived from the model catalog or from a built-in preset rather
-- than from the provider is unverified: it keeps its history and any route
-- that points at it, and stops routing until the provider's own listing
-- claims the id again. A connection that publishes no list of its own keeps
-- the ids an operator typed, which are the only models it has.
UPDATE models SET available = 0
WHERE source = 'modelsdev'
   OR (source = 'manual' AND provider_id IN (
        SELECT id FROM providers WHERE models_format <> 'none'));

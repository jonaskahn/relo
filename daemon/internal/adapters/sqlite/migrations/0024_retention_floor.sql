-- Retention keeps at least three days of request detail now: no rule
-- deletes a row inside the floor, whatever budget a stored row names. A
-- budget below the floor is lifted to it; 0 still keeps everything.
UPDATE retention_config SET usage_days = 3 WHERE usage_days BETWEEN 1 AND 2;

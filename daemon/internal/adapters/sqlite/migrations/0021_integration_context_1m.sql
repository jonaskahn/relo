-- The Codex agent's 1M context button. Existing integrations stay on, which
-- is the setup default: the window lines are written when those keys are absent.
ALTER TABLE integrations ADD COLUMN context_1m INTEGER NOT NULL DEFAULT 1;

-- Tool cutoff decides whether Relo trims a tool list the OpenCode Free
-- gateway would refuse, rather than sending a request it cannot accept.
ALTER TABLE template_settings ADD COLUMN tool_cutoff INTEGER NOT NULL DEFAULT 1 CHECK (tool_cutoff IN (0, 1));

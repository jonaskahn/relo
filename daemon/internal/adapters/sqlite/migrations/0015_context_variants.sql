CREATE TABLE context_variants (
    template_id TEXT NOT NULL PRIMARY KEY,
    enabled     INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    updated_at  INTEGER NOT NULL
) STRICT;

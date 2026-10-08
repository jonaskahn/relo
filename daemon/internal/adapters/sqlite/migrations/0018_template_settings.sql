CREATE TABLE template_settings (
    template_id  TEXT NOT NULL PRIMARY KEY,
    long_context INTEGER NOT NULL DEFAULT 1 CHECK (long_context IN (0, 1)),
    auto_refresh INTEGER NOT NULL DEFAULT 1 CHECK (auto_refresh IN (0, 1)),
    updated_at   INTEGER NOT NULL
) STRICT;

INSERT INTO template_settings (template_id, long_context, auto_refresh, updated_at)
SELECT template_id, enabled, 1, updated_at FROM context_variants;

DROP TABLE context_variants;

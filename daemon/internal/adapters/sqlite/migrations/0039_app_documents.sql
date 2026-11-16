-- Application documents: small JSON documents one package owns, validates and
-- loads whole into memory, addressed by (namespace, id). The storage never
-- reads inside a body; the owning package defines its schema, stamps
-- schema_version, and migrates an older version when it reads one back.
--
-- A document belongs here only when it is small, read-mostly, replaced whole
-- on every write, looked up by its address rather than by a field inside it,
-- not a secret, not high volume, and free of foreign keys (it tolerates the
-- thing it names disappearing). Anything else gets a table of its own.
CREATE TABLE IF NOT EXISTS app_documents (
	namespace TEXT NOT NULL,
	id TEXT NOT NULL,
	schema_version INTEGER NOT NULL,
	body TEXT NOT NULL CHECK (json_valid(body) AND length(body) <= 262144),
	updated_at INTEGER NOT NULL,
	PRIMARY KEY (namespace, id)
);

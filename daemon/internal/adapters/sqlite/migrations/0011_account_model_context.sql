-- An operator can size one connection's model for a single account, so two
-- accounts of the same connection may advertise different context windows for
-- the same model. The provider-level override lives in model_facts; this table
-- holds the account-level one, which takes precedence when a request is served
-- by that account. Only the context window is stored: an account sizes a model
-- rather than restating every fact about it.
CREATE TABLE account_model_facts (
    credential_id  TEXT NOT NULL,
    provider_id    TEXT NOT NULL,
    model_id       TEXT NOT NULL,
    context_window INTEGER,
    updated_at     INTEGER NOT NULL,
    PRIMARY KEY (credential_id, provider_id, model_id)
) STRICT;

-- A read resolves one model's account overrides across every account of a
-- connection, which is what publishes the smallest eligible window.
CREATE INDEX idx_account_model_facts_model ON account_model_facts(provider_id, model_id);

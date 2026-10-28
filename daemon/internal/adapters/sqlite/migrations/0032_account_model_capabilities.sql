-- Capabilities a per-account model roster reported for one credential. These
-- facts are discovered, not operator overrides: account_model_facts keeps the
-- manual context completions an operator owns. NULL means the roster was
-- silent; an empty reasoning_efforts JSON array means it explicitly listed no
-- effort choices.
CREATE TABLE account_model_capabilities (
    credential_id        TEXT NOT NULL,
    provider_id          TEXT NOT NULL,
    model_id             TEXT NOT NULL,
    source               TEXT NOT NULL,
    context_window       INTEGER,
    max_input            INTEGER,
    max_output           INTEGER,
    supports_tools       INTEGER,
    supports_reasoning   INTEGER,
    supports_vision      INTEGER,
    reasoning_efforts    TEXT,
    reasoning_toggle     INTEGER,
    reasoning_budget     INTEGER,
    reasoning_budget_min INTEGER,
    reasoning_budget_max INTEGER,
    observed_at          INTEGER NOT NULL,
    PRIMARY KEY (credential_id, provider_id, model_id)
) STRICT;

CREATE INDEX idx_account_model_capabilities_model
    ON account_model_capabilities (provider_id, model_id);

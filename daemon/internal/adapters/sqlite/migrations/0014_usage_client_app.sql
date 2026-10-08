-- The coding client a request was made with is stored on the event, so a
-- log filter still finds the row after the key that authenticated it is
-- removed.

ALTER TABLE usage_events ADD COLUMN client_app TEXT NOT NULL DEFAULT '';

UPDATE usage_events SET client_app = COALESCE(
    (SELECT access_keys.client FROM access_keys WHERE access_keys.id = usage_events.client_key_id),
    ''
);

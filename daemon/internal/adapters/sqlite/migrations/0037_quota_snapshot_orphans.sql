-- A quota row whose credential is gone describes no live account, so the
-- rows go with it. A tombstoned credential is gone too: the account it stood
-- for is not there to be measured.
DELETE FROM quota_snapshots
WHERE credential_id NOT IN (SELECT id FROM credentials WHERE deleted_at IS NULL);

DELETE FROM quota_history
WHERE credential_id NOT IN (SELECT id FROM credentials WHERE deleted_at IS NULL);

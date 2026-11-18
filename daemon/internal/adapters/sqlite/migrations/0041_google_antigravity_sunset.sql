-- Google has disabled Cloud Code Assist for accounts it finds behind a
-- third-party client, so a turn from this connection is refused with
-- TOS_VIOLATION whatever the account's quota is. Turn the connection off for
-- every install that already carries it. The row, its accounts, its models and
-- its usage stay, so switching it back on remains the operator's choice.
UPDATE providers SET enabled = 0 WHERE template_id = 'google-antigravity';

-- Discard only denials attributable to one retained lost-thread response.
-- Capture persistence, not request duration, bounds asynchronous bookkeeping.
WITH denials AS MATERIALIZED (
    SELECT cm.*, c.provider_id
    FROM credential_models cm
    JOIN credentials c ON c.id = cm.credential_id
    JOIN providers p ON p.id = c.provider_id
    WHERE cm.source = 'learned' AND cm.available = 0
      AND p.template_id = 'claude'
      AND NOT EXISTS (
          SELECT 1 FROM credential_rosters r WHERE r.credential_id = cm.credential_id
      )
), history AS MATERIALIZED (
    SELECT d.credential_id, d.model_id, d.observed_at, a.id AS attempt_id,
           e.timestamp, a.status,
           MAX(cap.created_at) AS persisted_at,
           CASE WHEN a.credential_id = d.credential_id AND a.status = 404
                AND instr(lower(a.error_code), 'no thread state was found') > 0
                AND instr(lower(a.error_code), 'previous_message_id') > 0
                AND COUNT(cap.id) = 1
                AND (SELECT COUNT(*) FROM usage_attempts sibling
                     WHERE sibling.event_id = a.event_id AND sibling.ordinal = a.ordinal) = 1
                AND MAX(cap.status) = 404 AND MAX(cap.truncated) = 0
                AND MAX(length(cap.body)) = MAX(cap.body_bytes)
                AND MAX(CASE WHEN a.status = 404 AND cap.status = 404 AND cap.truncated = 0
                             AND instr(lower(a.error_code), 'no thread state was found') > 0
                         THEN CASE WHEN json_valid(CAST(cap.body AS TEXT)) THEN
                    json_extract(CAST(cap.body AS TEXT), '$.error.details.error_code') = 'thread_not_found'
                    AND instr(lower(json_extract(CAST(cap.body AS TEXT), '$.error.message')),
                              'no thread state was found') > 0
                    AND instr(lower(json_extract(CAST(cap.body AS TEXT), '$.error.message')),
                              'previous_message_id') > 0
                    ELSE 0 END ELSE 0 END) = 1
                THEN 1 ELSE 0 END AS lost_thread
    FROM denials d
    JOIN usage_attempts a ON (a.credential_id = d.credential_id OR COALESCE(a.credential_id, '') = '')
        AND a.provider = d.provider_id AND a.model = d.model_id
    JOIN usage_events e ON e.id = a.event_id
    LEFT JOIN usage_captures cap ON cap.event_id = a.event_id
        AND cap.ordinal = a.ordinal AND cap.kind = 'provider_response'
    GROUP BY d.credential_id, d.model_id, a.id
), attributable AS (
    SELECT credential_id, model_id, MIN(timestamp) AS request_start
    FROM history
    WHERE timestamp <= observed_at AND (persisted_at IS NULL OR observed_at <= persisted_at)
    GROUP BY credential_id, model_id
    HAVING COUNT(*) = 1 AND MIN(lost_thread) = 1
)
DELETE FROM credential_models
WHERE (credential_id, model_id) IN (
    SELECT candidate.credential_id, candidate.model_id
    FROM attributable candidate
    WHERE NOT EXISTS (
        -- Unresolved or contradictory later refusals are not evidence to erase.
        SELECT 1 FROM history h
        WHERE h.credential_id = candidate.credential_id AND h.model_id = candidate.model_id
          AND (h.persisted_at IS NULL OR h.persisted_at >= candidate.request_start)
          AND h.status IN (403, 404) AND h.lost_thread = 0
    )
);

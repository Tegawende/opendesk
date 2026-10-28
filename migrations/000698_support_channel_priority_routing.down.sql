DROP INDEX support_channel_destinations_team_any_priority_idx;
DROP INDEX support_channel_destinations_team_priority_idx;
DROP INDEX support_channel_destinations_priority_idx;
DROP INDEX support_channel_destinations_fallback_idx;

-- Keep the prior catch-all route when it exists, otherwise retain the oldest route.
-- The legacy schema supports only one destination per team and one fallback.
DELETE FROM support_channel_destinations d
USING (
    SELECT id FROM (
        SELECT id, row_number() OVER (
            PARTITION BY connection_id, team_id
            ORDER BY (priority IS NULL) DESC, created_at, id
        ) AS route_order
        FROM support_channel_destinations
    ) ranked
    WHERE route_order > 1
) duplicates
WHERE d.id = duplicates.id;

ALTER TABLE support_channel_destinations DROP COLUMN priority;
ALTER TABLE support_channel_destinations
    ADD CONSTRAINT support_channel_destinations_connection_id_team_id_key UNIQUE (connection_id, team_id);

CREATE UNIQUE INDEX support_channel_destinations_fallback_idx
    ON support_channel_destinations(connection_id)
    WHERE team_id IS NULL;

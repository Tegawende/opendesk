ALTER TABLE support_channel_destinations
    ADD COLUMN priority text CHECK (priority IS NULL OR priority IN ('low', 'normal', 'high', 'urgent'));

ALTER TABLE support_channel_destinations
    DROP CONSTRAINT support_channel_destinations_connection_id_team_id_key;

DROP INDEX support_channel_destinations_fallback_idx;

CREATE UNIQUE INDEX support_channel_destinations_team_any_priority_idx
    ON support_channel_destinations(connection_id, team_id)
    WHERE team_id IS NOT NULL AND priority IS NULL;

CREATE UNIQUE INDEX support_channel_destinations_team_priority_idx
    ON support_channel_destinations(connection_id, team_id, priority)
    WHERE team_id IS NOT NULL AND priority IS NOT NULL;

CREATE UNIQUE INDEX support_channel_destinations_priority_idx
    ON support_channel_destinations(connection_id, priority)
    WHERE team_id IS NULL AND priority IS NOT NULL;

CREATE UNIQUE INDEX support_channel_destinations_fallback_idx
    ON support_channel_destinations(connection_id)
    WHERE team_id IS NULL AND priority IS NULL;

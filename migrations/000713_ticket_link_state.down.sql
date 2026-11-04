ALTER TABLE ticket_links
    DROP COLUMN synced_at,
    DROP COLUMN external_state_name,
    DROP COLUMN external_state,
    DROP COLUMN external_id;

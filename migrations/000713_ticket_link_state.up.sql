ALTER TABLE ticket_links
    ADD COLUMN external_id text,
    ADD COLUMN external_state text CHECK (external_state IN ('backlog', 'todo', 'in_progress', 'done', 'canceled')),
    ADD COLUMN external_state_name text,
    ADD COLUMN synced_at timestamptz;

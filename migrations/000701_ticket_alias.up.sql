ALTER TABLE tickets ADD COLUMN alias text;

CREATE UNIQUE INDEX tickets_ws_alias_key ON tickets (workspace_id, lower(alias)) WHERE alias IS NOT NULL;

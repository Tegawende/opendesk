-- Sessions without a contact are dropped (their visitors start a new session), then the column is
-- required again.
DELETE FROM widget_sessions WHERE contact_id IS NULL;
ALTER TABLE widget_sessions DROP COLUMN IF EXISTS identity;
ALTER TABLE widget_sessions ALTER COLUMN contact_id SET NOT NULL;

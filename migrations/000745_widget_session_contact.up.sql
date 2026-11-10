-- A widget session has no contact until the visitor starts a conversation: the widget starts a
-- session on every page of the host site, so a contact per session was one per visitor. Until
-- then the session remembers what the host page told it (a verified user id, an email, a name,
-- a picture, a company, widget_ fields) in identity, which the contact takes when it is made.
ALTER TABLE widget_sessions ALTER COLUMN contact_id DROP NOT NULL;
ALTER TABLE widget_sessions ADD COLUMN IF NOT EXISTS identity jsonb NOT NULL DEFAULT '{}'::jsonb;

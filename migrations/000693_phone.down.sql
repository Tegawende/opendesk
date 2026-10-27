DELETE FROM automations WHERE trigger = 'call_ended';
ALTER TABLE automations DROP CONSTRAINT IF EXISTS automations_trigger_check;
ALTER TABLE automations ADD CONSTRAINT automations_trigger_check
    CHECK (trigger IN ('ticket_created', 'customer_replied', 'agent_replied', 'ticket_updated'));

UPDATE tickets SET channel = 'api' WHERE channel = 'phone';
ALTER TABLE tickets DROP CONSTRAINT IF EXISTS tickets_channel_check;
ALTER TABLE tickets ADD CONSTRAINT tickets_channel_check CHECK (channel IN ('email', 'chat', 'api', 'portal'));

DROP INDEX IF EXISTS contacts_phone_digits_idx;
DROP TABLE IF EXISTS phone_calls;
DROP TABLE IF EXISTS phone_inbound_events;
DROP TABLE IF EXISTS phone_connections;

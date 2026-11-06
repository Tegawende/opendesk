-- A side conversation can start by forwarding one email of the ticket's conversation in full
-- (contracts/api.md "Threads"): its layout, and its inline images as copies on the first message.
ALTER TABLE ticket_threads ADD COLUMN forward_message_id uuid REFERENCES messages(id) ON DELETE SET NULL;
CREATE INDEX ticket_threads_forward_idx ON ticket_threads(forward_message_id) WHERE forward_message_id IS NOT NULL;

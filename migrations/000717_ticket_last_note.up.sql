-- A ticket's newest internal note (Ticket.last_note) is read with every ticket: this keeps
-- that one row a short index read, however long the conversation.
CREATE INDEX IF NOT EXISTS messages_ticket_last_note_idx ON messages(ticket_id, created_at DESC, id DESC)
    WHERE kind = 'note' AND thread_id IS NULL AND deleted_at IS NULL AND sent_as_reply_id IS NULL
        AND author_type IN ('user', 'api') AND NOT ai;

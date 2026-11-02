-- An API key whose admins turned "Record requests" on keeps a record of every request it makes
-- (contracts/api.md "API keys"), for 90 days (core.cleanup).
ALTER TABLE api_keys ADD COLUMN record_requests boolean NOT NULL DEFAULT false;

CREATE TABLE api_key_requests (
    id                 uuid PRIMARY KEY,
    workspace_id       uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    api_key_id         uuid NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    created_at         timestamptz NOT NULL DEFAULT now(),
    method             text NOT NULL,
    path               text NOT NULL,
    query              text NOT NULL DEFAULT '',
    status             integer NOT NULL,
    duration_ms        integer NOT NULL DEFAULT 0,
    request_id         text NOT NULL DEFAULT '',
    idempotency_key    text NOT NULL DEFAULT '',
    replayed           boolean NOT NULL DEFAULT false,
    ip                 text NOT NULL DEFAULT '',
    user_agent         text NOT NULL DEFAULT '',
    content_type       text NOT NULL DEFAULT '',
    request_size       bigint NOT NULL DEFAULT 0,
    -- The JSON a write sent, secrets masked, up to 64 KB; null for reads and non-JSON bodies.
    request_body       text,
    request_truncated  boolean NOT NULL DEFAULT false,
    -- The answer to a failed request (status 400 and up), up to 8 KB.
    response_body      text,
    response_truncated boolean NOT NULL DEFAULT false,
    -- The ticket the request named in its path, as written (an id, a number, gorgias:<id>), and
    -- the records it touched. No foreign keys: the record outlives a deleted ticket.
    ticket_ref         text NOT NULL DEFAULT '',
    ticket_id          uuid,
    message_id         uuid,
    contact_id         uuid
);
CREATE INDEX api_key_requests_key_idx ON api_key_requests (api_key_id, created_at DESC, id DESC);
CREATE INDEX api_key_requests_ticket_idx ON api_key_requests (ticket_id) WHERE ticket_id IS NOT NULL;
CREATE INDEX api_key_requests_created_idx ON api_key_requests (created_at);

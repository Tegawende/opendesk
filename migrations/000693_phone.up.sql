-- Phone connections (contracts/phone.md): a cloud phone system (Aircall first) whose
-- calls land on the caller's ticket as a call entry in its timeline.

-- One connection per provider per workspace.
CREATE TABLE phone_connections (
    id                 uuid PRIMARY KEY,
    workspace_id       uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    provider           text NOT NULL,
    name               text NOT NULL,
    -- The provider account's name (Aircall: the company), refreshed on every test.
    account_name       text NOT NULL DEFAULT '',
    -- The API credentials, sealed with d.Box.
    credentials_enc    bytea NOT NULL,
    -- The webhook opendesk registered at the provider, and the token its events carry,
    -- sealed with d.Box.
    webhook_id         text NOT NULL DEFAULT '',
    webhook_secret_enc bytea,
    status             text NOT NULL DEFAULT 'connected' CHECK (status IN ('connected', 'error')),
    last_error         text NOT NULL DEFAULT '',
    health_status      text NOT NULL DEFAULT 'unknown' CHECK (health_status IN ('unknown', 'healthy', 'action_required', 'error')),
    health_detail      text NOT NULL DEFAULT '',
    health_checked_at  timestamptz,
    -- Placement: a call goes on the caller's open ticket, else on their latest ticket
    -- solved within reuse_days (0: never), else on a new phone ticket.
    reuse_days         integer NOT NULL DEFAULT 7 CHECK (reuse_days BETWEEN 0 AND 90),
    -- An answered call assigns an unassigned ticket to the member who took it.
    assign_answered    boolean NOT NULL DEFAULT false,
    -- A ringing call shows the caller's tickets in the provider's app.
    insight_cards      boolean NOT NULL DEFAULT true,
    last_event_at      timestamptz,
    created_by         uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, provider)
);
CREATE INDEX phone_connections_created_by_idx ON phone_connections (created_by);

-- Verified provider events, normalized (never the raw payload), waiting for or done by
-- the worker. The id deduplicates the provider's retries; rows go after seven days.
CREATE TABLE phone_inbound_events (
    connection_id     uuid NOT NULL REFERENCES phone_connections(id) ON DELETE CASCADE,
    external_event_id text NOT NULL,
    workspace_id      uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    payload           jsonb NOT NULL,
    received_at       timestamptz NOT NULL DEFAULT now(),
    processed_at      timestamptz,
    PRIMARY KEY (connection_id, external_event_id)
);
CREATE INDEX phone_inbound_events_workspace_idx ON phone_inbound_events (workspace_id);
CREATE INDEX phone_inbound_events_received_idx ON phone_inbound_events (received_at);

-- One row per provider call: its latest known state, and the ticket and timeline entry
-- it is documented on. Provider events only ever move it forward.
CREATE TABLE phone_calls (
    id               uuid PRIMARY KEY,
    workspace_id     uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    connection_id    uuid REFERENCES phone_connections(id) ON DELETE SET NULL,
    provider         text NOT NULL,
    external_call_id text NOT NULL,
    ticket_id        uuid REFERENCES tickets(id) ON DELETE SET NULL,
    activity_id      uuid REFERENCES activities(id) ON DELETE SET NULL,
    contact_id       uuid REFERENCES contacts(id) ON DELETE SET NULL,
    direction        text NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    stage            text NOT NULL DEFAULT 'ringing' CHECK (stage IN ('ringing', 'answered', 'ended')),
    outcome          text NOT NULL DEFAULT '' CHECK (outcome IN ('', 'answered', 'missed', 'voicemail')),
    missed_reason    text NOT NULL DEFAULT '',
    -- The customer's number in E.164, '' when it was withheld.
    number           text NOT NULL DEFAULT '',
    -- The provider number (line) the call came in on or went out from.
    line_id          text NOT NULL DEFAULT '',
    line_name        text NOT NULL DEFAULT '',
    agent_user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    agent_name       text NOT NULL DEFAULT '',
    caller_name      text NOT NULL DEFAULT '',
    started_at       timestamptz,
    answered_at      timestamptz,
    ended_at         timestamptz,
    duration_seconds integer NOT NULL DEFAULT 0,
    has_recording    boolean NOT NULL DEFAULT false,
    has_voicemail    boolean NOT NULL DEFAULT false,
    -- Links only, never media: the provider's lasting page for the recording or voicemail
    -- (asset_url), and its direct links, which stop working at links_expire_at
    -- (recording_url, voicemail_url).
    asset_url        text NOT NULL DEFAULT '',
    recording_url    text NOT NULL DEFAULT '',
    voicemail_url    text NOT NULL DEFAULT '',
    links_expire_at  timestamptz,
    -- The provider's labels on the call and its users' comments on it, each with the time
    -- of the event they came with: an older event never replaces a newer list.
    tags             text[] NOT NULL DEFAULT '{}',
    tags_at          timestamptz,
    comments         jsonb NOT NULL DEFAULT '[]',
    comments_at      timestamptz,
    summary          text NOT NULL DEFAULT '',
    transcript       text NOT NULL DEFAULT '',
    outside_hours    boolean NOT NULL DEFAULT false,
    -- When the call_ended automations were queued (once per call), and the insight card.
    ended_trigger_at timestamptz,
    card_queued_at   timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, provider, external_call_id)
);
CREATE INDEX phone_calls_connection_idx ON phone_calls (connection_id);
CREATE INDEX phone_calls_ticket_idx ON phone_calls (ticket_id);
CREATE INDEX phone_calls_activity_idx ON phone_calls (activity_id);
CREATE INDEX phone_calls_contact_idx ON phone_calls (contact_id);
CREATE INDEX phone_calls_agent_idx ON phone_calls (agent_user_id);

-- Calls find their contact by the digits of the number.
CREATE INDEX contacts_phone_digits_idx ON contacts (workspace_id, (regexp_replace(phone, '[^0-9]', '', 'g'))) WHERE phone <> '';

-- Tickets opened by a call are on the phone channel.
ALTER TABLE tickets DROP CONSTRAINT IF EXISTS tickets_channel_check;
ALTER TABLE tickets ADD CONSTRAINT tickets_channel_check CHECK (channel IN ('email', 'chat', 'api', 'portal', 'phone'));

-- Automations can run when a call ends.
ALTER TABLE automations DROP CONSTRAINT IF EXISTS automations_trigger_check;
ALTER TABLE automations ADD CONSTRAINT automations_trigger_check
    CHECK (trigger IN ('ticket_created', 'customer_replied', 'agent_replied', 'ticket_updated', 'call_ended'));

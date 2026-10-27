# Phone (`internal/phone`)

A phone connection links a workspace to a cloud phone system. Every call, inbound or
outbound, is documented on the caller's ticket as one call entry in its timeline, kept
current while the call rings, is answered, ends and gets its recording, comments and AI
summary. The end of a call runs the `call_ended` automations, so a team decides with
its own rules what a missed call outside opening hours does. Version 1 ships Aircall.

This is not a support channel (`contracts/support-channels.md`): those mirror tickets
into chat tools where agents work them, and their engine is built around threads, cards
and commands. A phone system is a way customers reach the team, like a mailbox. It is
not an integration (`contracts/integrations.md`) either: those give Ollie tools over
MCP. The phone engine reuses their patterns (sealed credentials, a verified webhook
stored and queued in one transaction, health and repair, admin-only connect routes) in a
small package of its own.

The cloud and self-host use the same code. There is no plan gate.

## Model and API

```ts
type PhoneProvider = { key: string; name: string; fields: { name: string; label: string; secret: boolean }[] };
type PhoneConnection = { id: string; provider: string; name: string; account_name: string;
  status: "connected" | "error"; last_error: string;
  health_status: "unknown" | "healthy" | "action_required" | "error";
  health_code: "" | "receiving" | "webhook_missing" | "credentials_refused" | "provider_unreachable";
  health_detail: string; health_checked_at: string | null;
  reuse_days: number;          // 0..90, default 7 (Placement)
  assign_answered: boolean;    // default false (Agents)
  insight_cards: boolean;      // default true (Insight cards)
  last_event_at: string | null;
  webhook_url?: string;        // admins only: people with the admin role, API keys and AI clients with settings:write
  created_at: string };
type PhoneLine = { id: string; name: string; provider: string };
```

| method | path | role | body → response |
| --- | --- | --- | --- |
| GET | `/v1/phone/providers` | viewer, `settings:read` | → `{data: PhoneProvider[]}` |
| GET | `/v1/phone/connections` | viewer, `settings:read` | → `{data: PhoneConnection[]}`; never credentials, `webhook_url` for admins only |
| POST | `/v1/phone/connections` | admin user | `{provider, name?, credentials}` → 201 `PhoneConnection`; 422 `phone_credentials_refused`; 409 when the workspace already has that provider |
| PATCH | `/v1/phone/connections/{id}` | admin user | `{name?, reuse_days?, assign_answered?, insight_cards?, credentials?}` → `PhoneConnection` |
| POST | `/v1/phone/connections/{id}/test` | admin user | checks credentials and webhook, registers the webhook again when it is gone or disabled → `PhoneConnection` |
| DELETE | `/v1/phone/connections/{id}` | admin user | removes opendesk's webhook at the provider, then the connection → 204. Calls already on tickets stay |
| GET | `/v1/phone/lines` | admin, `settings:read` | → `{data: PhoneLine[]}`: the provider's numbers (asked at most every 10 minutes per connection, and again after new credentials) and the lines calls came in on, for `call_line` conditions |
| POST | `/v1/phone/hooks/{provider}/{connectionId}` | provider | the provider's webhook (Security) |

"Admin user" routes need a person signed in as an admin: connecting a phone system hands
it a way into tickets, so API keys get `403`. One connection per provider per workspace.

Events, to admins only (`events.Event.Admins`): `phone_connection.created` / `.updated`
`{phone_connection}` (also on every new health state), `phone_connection.deleted`
`{phone_connection_id}`. Audit:
`phone.connected`, `phone.updated`, `phone.credentials_rotated`, `phone.disconnected`.

## Provider adapter boundary

The engine owns connections, sealed credentials, contact matching, placement, the
timeline, automations and every ticket write. An adapter (`internal/phone/provider`) owns
its provider's protocol only, never imports a helpdesk package and never reads a table:

```go
type Adapter interface {
    Describe() Descriptor
    Validate(ctx, *http.Client, credentials) (Account, error)
    RegisterWebhook(ctx, *http.Client, credentials, url string) (Webhook, error) // {ID, Secret}
    WebhookActive(ctx, *http.Client, credentials, id string) (bool, error)
    DeleteWebhook(ctx, *http.Client, credentials, id string) error
    Verify(body []byte, secret string) error
    Parse(body []byte) (Event, error) // ErrIgnored for events of no use
    Lines(ctx, *http.Client, credentials) ([]Line, error)
    PushCard(ctx, *http.Client, credentials, callID string, Card) error
}
```

`Event` is normalized (`kind` call, summary or transcript; the provider's call id; `Final`
for the event after which the call no longer changes) and carries a `Call` snapshot:
direction, stage (ringing, answered, ended), the number, the line, the times, the missed
reason in the provider's words, whether there is a recording or voicemail, links, tags,
comments, the agent (name, email) and the provider's own name for the caller. Adding a
provider means implementing this interface and passing it to `phone.New` in `For`; provider
keys are data, not a database enum. Adapters are compiled in, like support channels'.

## Calls

`phone_calls` holds one row per provider call, unique by `(workspace, provider, call id)`,
with its latest state and the ticket, contact and timeline entry it is documented on.
`phone_inbound_events` holds verified events, normalized, until a week after they were
applied (30 days at most); the raw payload is never stored.

A call's content (number, names, links, tags, comments, summary, transcript) lives on its
ticket, in the call's row and its timeline entry, and goes when the ticket's content goes:
the row cascades with the ticket (`ON DELETE CASCADE`, like `messages` and `activities`),
and a deleted ticket keeps its calls as it keeps its messages. A deleted contact's calls stay
on their tickets, as their messages do (`contact_id` is cleared). The only rows without a
ticket are a blocked contact's, which keep no content (Contacts).

Every verified event is applied by the `phone.inbound` job in one transaction that locks
its event row and its call row, so events of one call apply one at a time and each once.
Providers deliver late, twice and out of order, so a call only moves forward: its stage
never goes back, a time once known stays (the start is the earliest seen), the outcome is
computed from the merged state (voicemail, else answered when someone answered, else
missed, once the call ended), tags and comments are the latest event's. The same events in
any order leave the same call.

### Contacts

A call's number is normalized to E.164 when it says its country (`+`, `00`; spaces, dots,
dashes, brackets and a trunk `(0)` dropped). Contacts match on the number's digits, written
with or without `00` and with a trunk `(0)` dropped as on the call's side (an index on
`contacts.phone`'s digits); the most recently updated wins.
A new number creates a contact named after the provider's contact for it, else the number,
and publishes `contact.created`. Calls from one new number are serialized, so they make one
contact. A withheld number has no contact.

A blocked contact's calls reach no ticket, as their email doesn't (`internal/email` ignores a
blocked sender): the call's row is kept, with its contact, stage and times but without its
number, names, links, tags, comments, summary or transcript, so its later events are known;
it writes no timeline entry, sends no insight card and runs no automation. A call already on
a ticket when its contact is blocked stays there.

### Placement

A call goes on the contact's open ticket (open, pending or on hold, most recently updated);
else on their latest ticket solved (`solved_at`) within the connection's `reuse_days` (0: never);
else on a new ticket on the `phone` channel, routed like any new ticket
(`routing.Resolve` with channel `phone`, so routing rules can send phone tickets to a team),
named "Call from …" or "Call to …" and running the `ticket_created` automations. Like an
`api` ticket, a phone ticket has no reply address: replies there are recorded, not emailed. Merged,
deleted, spam and closed tickets are never reused. A call is placed once, on its first event,
so the insight card can show its ticket; the ticket moves up (`updated_at`, `ticket.updated`
with `changes: ["calls"]`) when the call lands on it and when it ends.

### Timeline

A call is one activity of type `call`, actor `system`, created on its first event and
updated in place as the call moves on (`activity.updated`, only when something it shows
changed):

```ts
type CallData = { call_id: string; provider: string; direction: "inbound" | "outbound";
  stage: "ringing" | "answered" | "ended"; outcome: "" | "answered" | "missed" | "voicemail";
  missed_reason: string; number: string; line: string; line_id: string; caller_name: string;
  agent: UserRef | null; agent_name: string;
  started_at: string | null; answered_at: string | null; ended_at: string | null; duration: number;
  recording: boolean; voicemail: boolean;
  asset_url: string; recording_url: string; voicemail_url: string; links_expire_at: string | null;
  tags: string[]; comments: { id: string; text: string; author?: string; at?: string }[];
  summary: string; transcript: string; outside_hours: boolean };
```

opendesk stores links, never media. `recording_url` and `voicemail_url` are the provider's
direct links and stop working at `links_expire_at`; the app then plays through `asset_url`,
the provider's lasting page, which is also what "Open in Aircall" opens, shown only when
there is one. The provider's API link to the call is not kept, as it is no page for people.
Only https links are kept. The summary is bounded to 4,000 characters, the transcript to 20,000, comments to 20.

### Agents

The provider user who took or made a call maps to the member (owner, admin or agent) with
the same email; the entry shows them (`agent`), else the provider's name for them. With
`assign_answered`, an answered call assigns a ticket nobody has, isn't solved or closed and
whose team the member is on, as the system (`assigned` with `via: "phone"`).

### Automations

When a call ends (the provider's final event, once per call) the `call_ended` automations
of its ticket run a minute later, so the tags and comments the team adds right after hanging
up are in the call's facts, which are read when they run. Their conditions (only that
trigger takes them; another trigger with one is refused):

| field | ops | values |
| --- | --- | --- |
| `call_outcome` | equals, not_equals | answered, missed, voicemail |
| `call_direction` | equals, not_equals | inbound, outbound |
| `call_missed_reason` | equals, not_equals | the provider's reason, `[a-z0-9_]` (Aircall: `out_of_opening_hours`, `short_abandoned`, `abandoned_in_ivr`, `abandoned_in_classic`, `no_available_agent`, `agents_did_not_answer`) |
| `call_time` | equals, not_equals | business_hours, outside_business_hours: the call's start against the business hours of the ticket's team, else the workspace's; hours that are off are never outside |
| `call_line` | equals, not_equals, contains, not_contains | the line's name |
| `call_tag` | equals, not_equals, contains, not_contains | the call's tags, like `tag` |

Every other condition and every action works as on other triggers, so "a missed inbound
call outside business hours reopens the ticket, adds a note and raises its priority" is an
automation, not code. Tags and comments added after the call ended update its entry but
don't run the automations again.

### Insight cards

With `insight_cards`, a ringing inbound call shows the caller's name (linked to their
contact), the ticket the call went on and their three latest other tickets in the
provider's app (Aircall: one card per call). Anyone in the provider's app reads it, so it
names no ticket of a private team and no spam, merged or deleted one. The card is sent by
the `phone.insight_card` job and dropped when it could not go out within three minutes.

### Click to call

With a connection, phone numbers on contacts and in the ticket sidebar are `tel:` links
(a `00` prefix becomes `+`). The operating system hands them to the default phone app:
Aircall Workspace, once it is set as the handler for phone links.

## Aircall

Docs: [API reference](https://developers.aircall.io/api-references),
[webhooks](https://developer.aircall.io/docs/webhooks-overview.md),
[insight cards](https://developer.aircall.io/docs/insight-cards.md),
[call summaries](https://developer.aircall.io/docs/ai-call-summaries.md),
[click to dial](https://developer.aircall.io/docs/implement-click-to-dial.md).

- Credentials: an API ID and API token (Basic authentication), checked with
  `GET /v1/company`, whose name names the connection.
- Webhook: `POST /v1/webhooks` with `custom_name: "opendesk"`, the connection's
  `webhook_url` and the events `call.created`, `call.ringing_on_agent`, `call.answered`,
  `call.hungup`, `call.ended` (final), `call.voicemail_left`, `call.comm_assets_generated`,
  `call.tagged`, `call.untagged`, `call.commented`, `summary.created` and
  `transcription.created`. A company Aircall refuses the AI events for (no AI add-on) gets
  the call events alone. The answer's `webhook_id` is stored, and its `token` sealed.
  opendesk only ever reads, replaces or deletes the webhook it created, by that id; it never
  lists the account's webhooks, so other integrations' webhooks stay untouched.
- New credentials that can't see that webhook are another account's: the webhook is removed
  from the old account with the old credentials before they are replaced, so the old account
  stops sending calls, and one is registered in the new account. A new key of the same
  account keeps the webhook. Everything that talks to the provider about one connection
  (connect, test, a credentials change, disconnect) runs one at a time per connection, under
  a transaction-scoped advisory lock, so two at once never register two webhooks; it is not a
  row lock, so incoming events never wait on it.
- Aircall puts the token in every webhook body; opendesk compares it in constant time.
  Aircall waits five seconds and retries a failed delivery up to 50 times before disabling
  the webhook, so opendesk answers 200 at once (also to events it ignores or doesn't
  understand) and a test registers a disabled webhook again.
- A call's `raw_digits` is its number (`anonymous` when withheld), `number` its line,
  `user` who took or made it, `missed_call_reason` why it was missed, `asset` the lasting
  page for its media, `recording_short_url`/`voicemail_short_url` (three hours) else
  `recording`/`voicemail` (one hour) its direct links, `tags` and `comments` the team's.
  `direct_link` is an API URL (it needs the API key), so it is not kept.
- Insight card: `POST /v1/calls/:id/insight_cards` with a `title` and `shortText` lines.
- Rate limit: Aircall allows a company a set number of requests a minute; a 429 snoozes the
  job until `X-AircallApi-Reset`. opendesk calls the API on connect, test, disconnect, the
  lines list and one card per ringing inbound call.

## Security and operations

- Credentials and the webhook token are AES-GCM sealed with `d.Box`
  (`phone_connections.credentials_enc`, `.webhook_secret_enc`, in `internal/sealed`).
  Responses, events, errors and logs never include them; adapter errors name the
  operation and the status only.
- The webhook is found by its opaque connection id, body-capped at 1 MiB and verified
  before parsing. Its URL reaches the connection, so only admins read it (`webhook_url`,
  `phone_connection.*` events). A request that names no connection or fails verification
  counts against its address (30 a minute, then that address gets `429` before any work);
  a verified event counts against its connection (600 a minute). The two are apart, so
  forged requests never use up the provider's own allowance, and ids that name no
  connection add no limiter keys. Nothing in a payload
  decides the workspace, the ticket or anyone's access: the connection does. The stored
  event and its job are written in one transaction, and the event id (a hash of the body)
  makes a provider's retry a no-op.
- Provider API clients have a 15-second timeout and do not follow redirects. Their base
  URL is an adapter constant, never user input.
- Logs carry ids, event names and errors, never numbers, names or call content.
- A suspended workspace's events are acknowledged and dropped.

## Not yet

Aircall SMS and WhatsApp, two-way contact sync, an OAuth app (Aircall Marketplace) instead of
API keys, Aircall's real-time transcription, other providers.

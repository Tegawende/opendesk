---
title: API keys and webhooks
description: Create API keys and webhook endpoints, inspect deliveries and send test events.
---

Both are settings: an `admin` user, or an API key with `settings:read` and `settings:write`. Creating, changing and deleting either is audit-logged.

| Method | Path | Description |
| --- | --- | --- |
| `GET`, `POST` | `/v1/api-keys` | List or create API keys |
| `GET`, `PATCH`, `DELETE` | `/v1/api-keys/{id}` | |
| `GET`, `POST` | `/v1/webhooks` | List or create webhooks |
| `GET`, `PATCH`, `DELETE` | `/v1/webhooks/{id}` | |
| `GET` | `/v1/webhooks/{id}/deliveries` | Recent deliveries |
| `POST` | `/v1/webhooks/{id}/test` | Send a `webhook.test` event |

## API keys

```ts
type ApiKey = {
  id: string;
  name: string;
  prefix: string;
  scopes: string[];
  last_used_at: string | null;
  expires_at: string | null;
  created_at: string;
  created_by: UserRef | null;
  record_requests: boolean; // every request the key makes is recorded, kept 90 days
};
```

```sh
curl https://api.opendesk.sh/v1/api-keys \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "OD-Workspace: $WORKSPACE_ID" \
  -H "Content-Type: application/json" \
  -d '{ "name": "CRM sync", "scopes": ["tickets:read", "contacts:read", "contacts:write"], "expires_at": "2027-01-01T00:00:00Z" }'
```

The response is the key plus `secret`, shown only this once. See [scopes](/api/authentication/#scopes).

### Record a key's requests

To check afterwards what an integration did, turn on `record_requests` when you create the key, or later:

```sh
curl -X PATCH https://api.opendesk.sh/v1/api-keys/$KEY_ID \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "OD-Workspace: $WORKSPACE_ID" \
  -H "Content-Type: application/json" \
  -d '{ "record_requests": true }'
```

From the key's next request, each request it makes is kept for 90 days:

- when, the method, path and query, the status and how long it took, the `Idempotency-Key` and whether the answer was a replay;
- for single-ticket create/update and message create/edit, complete JSON up to 64 KB with secret fields masked; other writes keep type and size;
- for a failure (status 400 and up), the HTTP status, a known diagnostic code and validation-error count;
- the ticket, message and contact it touched.

Private mail, drafts, bulk operations and account/provider settings keep metadata only. Incomplete or malformed JSON is replaced by `***`. Path parameters use the route template unless they are UUIDs or numeric references. Queries keep valid pagination limits and `expand=messages`; other values are omitted or masked. Failure diagnostics omit rejected values and free-form error text.

Fields named like secrets (`secret`, `token`, `password`, `api_key`, `credential`…) are masked as `***`, and the `Authorization` header is never kept. Recording happens after the answer and never slows down or fails a request.

Read the record with `GET /v1/api-keys/{id}/requests`, newest first. Filter with `status=4xx` (or `2xx`, `5xx`, or `errors` for every failure, 400 and up), `method`, `since` and `until` (RFC 3339), and `ticket` (its id, its number, or the reference your path used). `GET /v1/api-keys/{id}/requests/{requestId}` gives one request with what it sent and the answer. Admins only, from the app; an API key can't read it. A revoked key's record stays readable. Ticket records follow current ticket visibility: admins outside a private team cannot read its records, and deleted tickets' records are unavailable.

```sh
curl "https://api.opendesk.sh/v1/api-keys/$KEY_ID/requests?status=4xx" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "OD-Workspace: $WORKSPACE_ID"
```

In the app: **Settings → API keys**, the key's menu, **Record requests**. The list's **Log** column then shows **On** and the key's failures of the last 24 hours: **On** opens the request log, the failure count opens it on those failures. The key's `error_count_24h` gives the same count (`null` while it does not record).

### Change a key's scopes

Send the new list with `PATCH /v1/api-keys/{id}`: it replaces the key's scopes, and the key's next request already has them. The secret stays the same, so nothing changes where the key is used.

```sh
curl -X PATCH https://api.opendesk.sh/v1/api-keys/$KEY_ID   -H "Authorization: Bearer $ACCESS_TOKEN"   -H "OD-Workspace: $WORKSPACE_ID"   -H "Content-Type: application/json"   -d '{ "scopes": ["tickets:read", "tickets:write"] }'
```

- **Removing a scope** applies at once.
- **Adding a scope** gives a live credential more access, so it asks for what creating a key asks for: a sign-in within the last 12 hours, and the second factor when it is on (`403 reauth_required` / `mfa_required`).
- `collab:draft` drafts replies as the member who created the key: only that member can add it.
- Each change is in the audit log as `api_key.scopes_changed`, with the scopes before and after, and what was added and removed.

In the app: **Settings → API keys**, the key's menu, **Edit permissions**.

## Webhooks

```ts
type Webhook = { id: string; url: string; description: string; events: string[]; enabled: boolean; created_at: string };
type WebhookDelivery = {
  id: string;
  event_id: string;
  event_type: string;
  status: "pending" | "succeeded" | "failed";
  attempts: number;
  response_code: number | null;
  duration_ms: number | null;
  created_at: string;
};
```

```sh
curl https://api.opendesk.sh/v1/webhooks \
  -H "Authorization: Bearer od_live_..." \
  -H "Content-Type: application/json" \
  -d '{ "url": "https://example.com/hooks/opendesk", "description": "Sync to warehouse", "events": ["ticket.created", "message.created"], "enabled": true }'
```

The response includes the signing `secret`, shown once. `events` lists event types, or `["*"]` for all. The URL must be HTTPS and must not point at a private, loopback or link-local address.

See [Webhooks](/api/webhooks/) for payloads, signatures and retries.

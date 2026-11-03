---
title: API overview
description: The opendesk REST API, its base URL, versioning, conventions and workspace header.
---

The opendesk web app uses exactly this public API. Anything you can do in the app, you can do with a request.

## Base URL

| | Base URL |
| --- | --- |
| Cloud | `https://api.opendesk.sh/v1` |
| Self-hosted | `https://<your-domain>/v1` (your `API_URL` followed by `/v1`) |

Examples in these docs use the cloud URL.

## Versioning

The version is in the path. `/v1` changes only in backward-compatible ways: new endpoints, new optional fields and new event types can appear at any time, so ignore fields you do not recognize. A breaking change would ship as a new version.

## Conventions

- Requests and responses are JSON, with `snake_case` keys.
- Timestamps are RFC 3339 in UTC, such as `2026-10-11T09:00:00Z`.
- Ids are UUID strings. Tickets can also be addressed by alias or number: `/v1/tickets/ORDER-42`, `/v1/tickets/1042` or `/v1/tickets/%231042`.
- Errors are [problem details](/api/errors/) with a stable `code`.
- Lists are [cursor paginated](/api/pagination/).
- Create routes accept an [`Idempotency-Key`](/api/idempotency/).
- Responses carry [rate limit](/api/rate-limits/) headers.

## Authentication and the workspace header

Every request, other than the public and widget endpoints, carries a bearer credential. See [Authentication](/api/authentication/).

Most routes act on one **workspace**. Users can belong to several, so agent requests name it with the `OD-Workspace` header:

```sh
curl https://api.opendesk.sh/v1/tickets \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "OD-Workspace: 0192f0c4-7c1e-7b7a-9a4e-3c1d2e5f6a7b"
```

An **API key** belongs to exactly one workspace, so `OD-Workspace` is ignored for API keys:

```sh
curl https://api.opendesk.sh/v1/tickets \
  -H "Authorization: Bearer od_live_..."
```

## Roles

Requests made as a user are limited by their role in the workspace: `owner` > `admin` > `agent` > `viewer`. Viewers read only. Settings routes (mailboxes, members, API keys, webhooks, automations, SLA policies and widget settings) need `admin`. API keys are limited by their [scopes](/api/authentication/#scopes).

## A first request

```sh
curl https://api.opendesk.sh/v1/tickets?status=open,pending&limit=10 \
  -H "Authorization: Bearer od_live_..."
```

```json
{
  "data": [
    {
      "id": "0192f0c4-...",
      "number": 1042,
      "subject": "Can't sign in",
      "status": "open",
      "priority": "normal",
      "channel": "email",
      "contact": { "id": "...", "email": "ana@example.com", "name": "Ana", "avatar_url": "" },
      "assignee": null,
      "tags": [],
      "last_message_at": "2026-10-11T09:00:00Z",
      "last_message_preview": "I reset my password but..."
    }
  ],
  "next_cursor": "eyJ..."
}
```

## Next

- [Resources](/api/resources/tickets/): every endpoint, by resource
- [Webhooks](/api/webhooks/) and [Realtime](/api/realtime/) for changes as they happen
- [OpenAPI reference](/api/openapi/)

---
title: Contacts and companies
description: Manage the people who write in and the companies they belong to.
---

Scopes: `contacts:read` to read, `contacts:write` to change.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/v1/contacts` | List contacts. Filters: `q`, `company_id`, `tag`, `field.<key>` |
| `POST` | `/v1/contacts` | Create a contact |
| `GET` | `/v1/contacts/{id}` | Get a contact |
| `PATCH` | `/v1/contacts/{id}` | Update a contact |
| `DELETE` | `/v1/contacts/{id}` | Delete a contact |
| `GET` | `/v1/contacts/{id}/tickets` | The contact's tickets |
| `GET`, `POST` | `/v1/companies` | List or create companies. Filters: `q`, `tag`, `field.<key>` |
| `GET`, `PATCH`, `DELETE` | `/v1/companies/{id}` | Get, update or delete a company |

## Objects

```ts
type Contact = {
  id: string;
  email: string | null;
  emails: string[];            // extra addresses beside the primary email
  name: string;
  phone: string;
  avatar_url: string;
  company: { id: string; name: string; domain: string } | null;
  external_id: string | null;
  notes: string;
  custom: Record<string, unknown>;
  tags: { id: string; name: string; color: LabelColor; group_id: string | null }[];
  fields: Record<string, FieldValue>;
  blocked: boolean;
  ticket_count: number;
  created_at: string;
  updated_at: string;
};

type Company = {
  id: string;
  name: string;
  domain: string;
  notes: string;
  custom: Record<string, unknown>;
  tags: { id: string; name: string; color: LabelColor; group_id: string | null }[];
  fields: Record<string, FieldValue>;
  contact_count: number;
  created_at: string;
  updated_at: string;
};
```

A contact's email is unique within the workspace, and so is its `external_id`, your own id for the person (set by [verified widget sessions](/guides/live-chat/identity-verification/)). A company's domain is unique within the workspace.

## Several email addresses

A person who writes from more than one address stays one contact. Besides the primary `email`, a contact has extra addresses in `emails`. Mail from any of them lands on the contact, and a search finds it by any. An address belongs to one contact, primary or extra: adding one that another contact (or this one) already has is `409`.

| Method | Path | |
| --- | --- | --- |
| `POST` | `/v1/contacts/{id}/emails` | `{ "email": "..." }` adds an extra address (`201`, at most 20) |
| `DELETE` | `/v1/contacts/{id}/emails/{email}` | takes an extra address off (the primary changes with `PATCH`) |
| `POST` | `/v1/contacts/{id}/emails/{email}/make-primary` | swaps an extra address with the primary; the old primary becomes an extra |

Each answers with the contact. Blocking a contact blocks every address. Customer-portal sign-in uses the primary address only, so making another address the primary ends the contact's portal sessions.

Writes to contacts and companies take `tags` (replaces), `add_tags`, `remove_tags` and `fields` (merged, `null` clears), with the same rules as tickets. See [Labels and custom fields](/api/resources/labels-fields/).

## Examples

```sh
# Search
curl "https://api.opendesk.sh/v1/contacts?q=ana" -H "Authorization: Bearer od_live_..."

# Create
curl https://api.opendesk.sh/v1/contacts \
  -H "Authorization: Bearer od_live_..." \
  -H "Content-Type: application/json" \
  -d '{ "email": "ana@example.com", "name": "Ana Silva", "custom": { "plan": "pro" } }'

# Block
curl -X PATCH https://api.opendesk.sh/v1/contacts/0192f0c4-... \
  -H "Authorization: Bearer od_live_..." \
  -H "Content-Type: application/json" \
  -d '{ "blocked": true }'
```

## Events

`contact.created`, `contact.updated`, `contact.deleted`, and the same for `company.*`.

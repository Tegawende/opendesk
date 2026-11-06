---
title: Templates and email design
description: The template variables, each brand's email layout and satisfaction email, previews and test sends, ticket values, and member profiles.
---

How templates work, and every variable, is described in [Template variables and syntax](/guides/email-templates/variables/).

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/v1/templates/variables` | The variables, slots and filters, with your custom fields |
| `GET` | `/v1/email-templates/{brand_id}` | A brand's layout and satisfaction email |
| `PUT` | `/v1/email-templates/{brand_id}/{kind}` | Admin: save the layout or the satisfaction email |
| `DELETE` | `/v1/email-templates/{brand_id}/{kind}` | Admin: go back to the default |
| `POST` | `/v1/templates/preview` | Render a template without saving it |
| `POST` | `/v1/templates/test` | Users only: send the rendered email to yourself |
| `GET` | `/v1/tickets/{id}/variables` | Every variable's value for a ticket |
| `POST` | `/v1/tickets/{id}/render` | Fill a template for a ticket |
| `PATCH` | `/v1/members/{user_id}/profile` | Users only: change a member's profile |

Scopes: `settings:read` for the variables and a brand's templates, `settings:write` to save or reset them, and `tickets:read` (an agent or above) for previews, test sends and the ticket endpoints.

## Variables

```ts
type TemplateCatalog = { variables: TemplateVariable[]; slots: TemplateSlot[]; filters: string[] };

type TemplateVariable = {
  key: string; // "contact.first_name", "ticket.fields.order_number"
  group: "contact" | "company" | "ticket" | "agent" | "team" | "brand" | "mailbox" | "workspace" | "date";
  label: string;
  description: string;
  example: string;
  html: boolean; // true for agent.signature, which inserts HTML
};
type TemplateSlot = {
  key: "content" | "signature" | "history" | "survey";
  label: string;
  description: string;
  required_in: TemplateKind[];
};
type TemplateKind = "reply" | "signature" | "personal_signature" | "auto_reply" | "layout" | "csat" | "subject";
```

`GET /v1/templates/variables` returns the built-in variables followed by one entry per custom field that is not archived. Build pickers from it rather than keeping your own list.

## Email templates

Each brand has two: its `layout` and its satisfaction (`csat`) email.

```ts
type EmailTemplate = {
  brand_id: string;
  kind: "layout" | "csat";
  subject: string;                  // csat only; "" = the default
  body_html: string;                // layout: the custom layout (mode "custom"); csat: the email body
  options: EmailLayoutOptions | {}; // layout only
  is_default: boolean;              // true when the brand has not changed it; the fields carry the default
  updated_at: string | null;
};
type EmailLayoutOptions = {
  mode: "simple" | "custom";
  style: "plain" | "branded";
  logo: boolean;                    // the brand's logo on top
  accent: string;                   // "#rrggbb", or "" for the brand colour
  font: "system" | "sans" | "serif";
  footer_html: string;              // takes variables; "" = none
  history: "none" | "last" | "all"; // quote the previous message, or the whole conversation (up to 10)
  history_header: string;           // {{ message.author }}, {{ message.email }} and {{ message.date }}; "" = the default
};
```

`GET /v1/email-templates/{brand_id}` returns `{ layout: EmailTemplate, csat: EmailTemplate }`.

### Save

`PUT /v1/email-templates/{brand_id}/{kind}` takes any of `subject`, `body_html` and `options`; what you leave out keeps its current value. A layout is up to 128 KB and its footer up to 16 KB; a satisfaction email's body is up to 128 KB and its subject up to 200 characters, as is the quote line.

```sh
curl -X PUT https://api.opendesk.sh/v1/email-templates/$BRAND_ID/layout \
  -H "Authorization: Bearer od_live_..." \
  -H "Content-Type: application/json" \
  -d '{ "options": { "mode": "simple", "style": "branded", "logo": true, "accent": "", "font": "system",
        "footer_html": "<p>{{ brand.name }}, 1 Main Street, Springfield</p>", "history": "last", "history_header": "" } }'
```

A custom layout (`mode: "custom"`) must contain `{{ content }}` and may use `{{ signature }}` and `{{ history }}`. A satisfaction email body must contain `{{ survey }}`. Templates are sanitized with the design allowlist: email HTML plus tables, inline `style` with safe properties, `width`, `height`, `bgcolor`, `valign`, and `mailto:` and `tel:` links.

A template with an unknown variable, a tag that does not parse, a slot it may not use or a missing required slot is `422 validation_failed`, with one entry in `errors` per problem. `value` is the tag at fault:

```json
{
  "status": 422,
  "code": "validation_failed",
  "detail": "There is no variable called contact.frist_name.",
  "errors": [
    { "message": "There is no variable called contact.frist_name.", "location": "body.body_html", "value": "{{ contact.frist_name }}" }
  ]
}
```

`DELETE /v1/email-templates/{brand_id}/{kind}` puts the template back to the default and answers `204`.

## Preview and test

`POST /v1/templates/preview` renders a template without saving it:

```ts
type PreviewRequest = {
  kind: TemplateKind;
  body_html?: string;
  subject?: string;
  options?: EmailLayoutOptions;
  brand_id?: string;
  team_id?: string;
  mailbox_id?: string;
  ticket_id?: string; // fill from this ticket, else sample data
};
type TemplatePreview = { subject: string; html: string; text: string; problems: { message: string; token: string }[] };
```

`html` is the whole email as the customer would see it: a signature or auto-reply inside its brand's layout, a layout or satisfaction email around sample content. Values come from `ticket_id` (a ticket you can see) or sample data, with you as the agent. Problems are returned, not refused, so an editor can show them as you type.

`POST /v1/templates/test` takes the same body and sends the rendered email to your own login address, with `[Test]` before the subject. It needs a user, not an API key, and allows 10 an hour per user (`429` after that).

## Ticket values

`GET /v1/tickets/{id}/variables?mailbox_id=` returns every variable's value for the ticket, with you as the agent, and the signature your email reply from `mailbox_id` (else the ticket's address) would carry:

```json
{
  "values": { "contact.first_name": "Jane", "ticket.number": "1042", "team.name": "Billing" },
  "signature_html": "<p>Alex Morgan, Support engineer</p>"
}
```

`POST /v1/tickets/{id}/render` takes `{ body_html }` and returns `{ body_html }`, filled for the ticket and sanitized like a reply. The web app uses it to insert saved replies.

```sh
curl https://api.opendesk.sh/v1/tickets/$TICKET_ID/render \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "OD-Workspace: $WORKSPACE_ID" \
  -H "Content-Type: application/json" \
  -d '{ "body_html": "<p>Hi {{ contact.first_name | default: \"there\" }},</p>" }'
```

Both need the ticket to be visible to you; a ticket of a private team you are not on is `404`. You do not need these to send a reply: a reply posted to [`POST /v1/tickets/{id}/messages`](/api/resources/messages/#send-a-reply) is filled when it is created.

## Member profiles

```ts
type MemberProfile = {
  title: string;          // up to 80 characters
  phone: string;          // up to 40
  pronouns: string;       // up to 40
  signature_html: string; // the personal signature; "" = none, up to 64 KB
};
```

Every `Member` carries its `profile`. `PATCH /v1/members/{user_id}/profile` takes any of the four fields and returns the `Member`. `user_id` may be `me`; members edit their own profile and admins anyone's. Title, phone and pronouns are one line of plain text. The personal signature takes every variable except `{{ agent.signature }}`.

```sh
curl -X PATCH https://api.opendesk.sh/v1/members/me/profile \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "OD-Workspace: $WORKSPACE_ID" \
  -H "Content-Type: application/json" \
  -d '{ "title": "Support engineer", "pronouns": "they/them" }'
```

## Events

`email_template.updated` (`{ brand_id, kind }`) when a brand's layout or satisfaction email is saved or reset; refetch it. A profile change publishes `member.updated`.

// Phone connections (contracts/phone.md). The demo Aircall account takes the API ID
// "demo" with any token; anything else is refused like a wrong key.

import type { PhoneProvider } from "@/lib/api/types";
import { phoneConnection } from "../phone";
import { del, get, HttpError, invalid, notFound, patch, post, respond } from "../router";
import { nowIso } from "../util";

const providers: PhoneProvider[] = [
  {
    key: "aircall",
    name: "Aircall",
    fields: [
      { name: "api_id", label: "API ID", secret: false },
      { name: "api_token", label: "API token", secret: true },
    ],
  },
];

const lines = [
  { id: "501", name: "DE - Support", provider: "aircall" },
  { id: "502", name: "EN - Support", provider: "aircall" },
  { id: "503", name: "EN - Sales", provider: "aircall" },
];

export function registerPhone() {
  get("/v1/phone/providers", () => ({ data: providers }));
  get("/v1/phone/connections", (ctx) => ({ data: ctx.ws.phone }));
  get("/v1/phone/lines", (ctx) => ({ data: ctx.ws.phone.length ? lines : [] }));
  post("/v1/phone/connections", (ctx) => {
    const b = ctx.body as { provider?: string; credentials?: Record<string, string> };
    if (b.provider !== "aircall") throw invalid("Unknown phone provider.");
    if (ctx.ws.phone.some((c) => c.provider === b.provider)) throw new HttpError(409, "conflict", "This workspace is already connected to that provider.");
    if (!b.credentials?.api_id?.trim() || !b.credentials?.api_token?.trim()) throw invalid("Provide every required credential.");
    if (b.credentials.api_id !== "demo") throw new HttpError(422, "phone_credentials_refused", "The provider refused these credentials.");
    const row = phoneConnection({ name: "Margin", created_at: nowIso(), last_event_at: null, health_checked_at: nowIso() });
    ctx.ws.phone.push(row);
    return respond(201, row);
  });
  patch("/v1/phone/connections/:id", (ctx) => {
    const row = ctx.ws.phone.find((c) => c.id === ctx.params.id);
    if (!row) throw notFound("Phone connection");
    const b = ctx.body as { reuse_days?: number; assign_answered?: boolean; insight_cards?: boolean; name?: string };
    if (b.reuse_days !== undefined) row.reuse_days = b.reuse_days;
    if (b.assign_answered !== undefined) row.assign_answered = b.assign_answered;
    if (b.insight_cards !== undefined) row.insight_cards = b.insight_cards;
    if (b.name) row.name = b.name;
    return row;
  });
  post("/v1/phone/connections/:id/test", (ctx) => {
    const row = ctx.ws.phone.find((c) => c.id === ctx.params.id);
    if (!row) throw notFound("Phone connection");
    Object.assign(row, { health_status: "healthy", health_code: "receiving", status: "connected", health_checked_at: nowIso() });
    return row;
  });
  del("/v1/phone/connections/:id", (ctx) => {
    const before = ctx.ws.phone.length;
    ctx.ws.phone = ctx.ws.phone.filter((c) => c.id !== ctx.params.id);
    if (ctx.ws.phone.length === before) throw notFound("Phone connection");
    return respond(204, null);
  });
}

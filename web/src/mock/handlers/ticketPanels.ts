// Mock ticket panels (contracts/integrations.md "Ticket panels"): a seeded Shopify store
// whose orders are made up from the contact's email, and http panels that answer a few
// sections. Nothing is fetched; credentials are only checked as given, never kept.

import type { PanelOrder, TicketPanel, TicketPanelData } from "@/lib/api/types";
import { db, userRef, type WsData } from "../db";
import { publish } from "../gateway";
import { HttpError, del, forbidden, get, invalid, notFound, patch, post, respond, type Ctx } from "../router";
import { findTicket } from "../tickets";
import { nowIso, uuid } from "../util";

const panelsOf = new Map<string, TicketPanel[]>();

function seed(): TicketPanel[] {
  const at = nowIso();
  return [
    {
      id: uuid(),
      kind: "shopify",
      name: "Margin Store",
      host: "margin-store.myshopify.com",
      auth: "client_credentials",
      client_id: "3f9c0d1e7a2b4c5d8e6f",
      enabled: true,
      position: 0,
      status: "connected",
      last_error: "",
      created_at: at,
      updated_at: at,
    },
  ];
}

export function panelsState(ws: WsData): TicketPanel[] {
  let list = panelsOf.get(ws.workspace.id);
  if (!list) {
    list = seed();
    panelsOf.set(ws.workspace.id, list);
  }
  return list;
}

/** A Shopify panel's credentials, as the server reads them: an app's client ID and secret, or a token. */
function shopifyAuth(b: Record<string, unknown>): Pick<TicketPanel, "auth" | "client_id"> {
  const id = String(b.client_id ?? "").trim();
  const secret = String(b.client_secret ?? "").trim();
  const token = String(b.token ?? "").trim();
  if (token && (id || secret)) throw invalid("Give the app's client ID and secret, or an access token, not both.");
  if (token) return { auth: "access_token", client_id: undefined };
  if (!id || !secret) throw invalid("Paste the app's client ID and client secret.");
  return { auth: "client_credentials", client_id: id };
}

function admin(ws: WsData) {
  const r = ws.members.find((m) => m.user_id === db.meId)?.role;
  if (r !== "owner" && r !== "admin") throw forbidden("Only admins can manage ticket panels.");
}

function find(ctx: Ctx): TicketPanel {
  const p = panelsState(ctx.ws).find((x) => x.id === ctx.params.id);
  if (!p) throw notFound("Ticket panel");
  return p;
}

function audit(ws: WsData, action: string, p: TicketPanel) {
  ws.audit.unshift({ id: uuid(), actor: { type: "user", user: userRef(db.meId), api_key_id: null }, action, entity_type: "ticket_panel", entity_id: p.id,
    data: { kind: p.kind, host: p.host }, ip: "84.152.33.17", created_at: nowIso() });
}

const SHOP = /^[a-z0-9][a-z0-9-]{0,59}\.myshopify\.com$/;

function shopOf(raw: unknown): string {
  let s = String(raw ?? "").trim().toLowerCase();
  if (s.includes("://")) {
    try {
      s = new URL(s).host;
    } catch {
      s = "";
    }
  }
  if (s && !s.includes(".")) s += ".myshopify.com";
  if (!SHOP.test(s)) throw invalid("Use the shop's myshopify.com domain, such as my-store.myshopify.com.");
  return s;
}

function urlOf(raw: unknown): string {
  const s = String(raw ?? "").trim();
  if (!/^https:\/\/[^/\s]+/.test(s)) throw new HttpError(422, "invalid_url", "The URL must use https://.");
  return s;
}

const secret = () => `pnsec_${uuid().replace(/-/g, "")}`;

function hash(s: string) {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
  return Math.abs(h);
}

const PRODUCTS = [
  { title: "Arco floor lamp", variant: "Marble", price: "1890.00" },
  { title: "Tolomeo desk lamp", variant: "Aluminium", price: "289.00" },
  { title: "Flowerpot VP3", variant: "Mint", price: "159.00" },
  { title: "LED bulb E27", price: "12.90" },
];

/** A made-up store history for an email: the same every time, none for some addresses. */
function ordersFor(email: string, shop: string): { orders: PanelOrder[]; spent: number } | null {
  const h = hash(email);
  if (h % 5 === 0) return null;
  const n = 1 + (h % 3);
  const handle = shop.replace(/\.myshopify\.com$/, "");
  const orders: PanelOrder[] = [];
  let spent = 0;
  for (let i = 0; i < n; i++) {
    const p = PRODUCTS[(h + i) % PRODUCTS.length];
    const qty = 1 + ((h >> i) % 2);
    const total = Number(p.price) * qty;
    spent += total;
    const id = 5000 + (h % 900) + i;
    const days = 3 + i * 17 + (h % 5);
    const fulfilled = i > 0 || h % 2 === 0;
    orders.push({
      id: `gid://shopify/Order/${id}`,
      number: `#${1000 + (h % 400) + n - i}`,
      url: `https://admin.shopify.com/store/${handle}/orders/${id}`,
      created_at: new Date(Date.now() - days * 86_400_000).toISOString(),
      total: total.toFixed(2),
      currency: "EUR",
      financial_status: i === 0 && h % 7 === 0 ? "refunded" : "paid",
      fulfillment_status: fulfilled ? "fulfilled" : "unfulfilled",
      cancelled: false,
      items: [{ title: p.title, variant: p.variant, quantity: qty, price: p.price }],
      tracking: fulfilled ? [{ company: "DHL Express", number: `JD01460000${id}`, url: `https://www.dhl.com/track?tracking-id=JD01460000${id}` }] : [],
    });
  }
  return { orders, spent };
}

function dataFor(p: TicketPanel, t: { id: string; number: number; contact: { email: string | null; name: string } | null }): TicketPanelData {
  const base: TicketPanelData = { panel_id: p.id, kind: p.kind, state: "ok", fetched_at: nowIso() };
  const email = t.contact?.email?.toLowerCase() ?? "";
  if (p.kind === "shopify") {
    if (!email) return { ...base, state: "no_contact" };
    const found = ordersFor(email, p.host);
    if (!found) return { ...base, state: "empty" };
    const handle = p.host.replace(/\.myshopify\.com$/, "");
    return {
      ...base,
      customer: {
        name: t.contact?.name || email,
        url: `https://admin.shopify.com/store/${handle}/customers/${7000 + (hash(email) % 900)}`,
        orders_count: found.orders.length + (hash(email) % 4),
        total_spent: found.spent.toFixed(2),
        currency: "EUR",
        tags: hash(email) % 3 === 0 ? ["VIP"] : [],
      },
      orders: found.orders,
    };
  }
  return {
    ...base,
    sections: [
      {
        title: "Sales order",
        items: [
          { label: "Order", value: `SO-${2200 + (t.number % 300)}`, url: `https://erp.example.com/so/${2200 + (t.number % 300)}` },
          { label: "Confirmed delivery", value: new Date(Date.now() + 9 * 86_400_000).toISOString().slice(0, 10) },
          { label: "Supplier", value: "Flos S.p.A." },
        ],
      },
    ],
  };
}

export function registerTicketPanels() {
  get("/v1/ticket-panels", (ctx) => ({ data: [...panelsState(ctx.ws)].sort((a, b) => a.position - b.position) }));

  post("/v1/ticket-panels", (ctx) => {
    admin(ctx.ws);
    const list = panelsState(ctx.ws);
    if (list.length >= 10) throw new HttpError(409, "conflict", "A workspace can have at most 10 ticket panels.");
    const kind = ctx.body.kind;
    const at = nowIso();
    let p: TicketPanel;
    let s: string | undefined;
    if (kind === "shopify") {
      const host = shopOf(ctx.body.shop);
      const auth = shopifyAuth(ctx.body);
      p = { id: uuid(), kind, name: String(ctx.body.name ?? "").trim() || "Margin Store", host, ...auth, enabled: true, position: list.length, status: "connected", last_error: "", created_at: at, updated_at: at };
    } else if (kind === "http") {
      const url = urlOf(ctx.body.url);
      p = { id: uuid(), kind, name: String(ctx.body.name ?? "").trim() || new URL(url).host, host: new URL(url).host, enabled: true, position: list.length, status: "connected", last_error: "", created_at: at, updated_at: at };
      s = secret();
    } else throw invalid("A panel is shopify or http.");
    list.push(p);
    publish(ctx.ws, "ticket_panel.created", { panel: p });
    audit(ctx.ws, "ticket_panel.connected", p);
    return respond(201, s ? { panel: p, secret: s } : { panel: p });
  });

  patch("/v1/ticket-panels/:id", (ctx) => {
    admin(ctx.ws);
    const p = find(ctx);
    const b = ctx.body;
    if (typeof b.name === "string") p.name = b.name.trim().slice(0, 64) || p.name;
    if (typeof b.enabled === "boolean") p.enabled = b.enabled;
    if (typeof b.position === "number") p.position = b.position;
    if (b.token !== undefined || b.client_id !== undefined || b.client_secret !== undefined) {
      if (p.kind !== "shopify") throw invalid("Only a Shopify panel has credentials.");
      // A rotated secret keeps the app's client ID.
      Object.assign(p, shopifyAuth(b.token !== undefined ? b : { client_id: p.client_id, ...b }));
      p.status = "connected";
      p.last_error = "";
    }
    if (b.url !== undefined) {
      if (p.kind !== "http") throw invalid("Only an http panel has a URL.");
      p.host = new URL(urlOf(b.url)).host;
      p.status = "connected";
      p.last_error = "";
    }
    p.updated_at = nowIso();
    publish(ctx.ws, "ticket_panel.updated", { panel: p });
    audit(ctx.ws, "ticket_panel.updated", p);
    return p;
  });

  post("/v1/ticket-panels/:id/rotate-secret", (ctx) => {
    admin(ctx.ws);
    const p = find(ctx);
    if (p.kind !== "http") throw invalid("Only an http panel has a signing secret.");
    p.updated_at = nowIso();
    publish(ctx.ws, "ticket_panel.updated", { panel: p });
    return { panel: p, secret: secret() };
  });

  del("/v1/ticket-panels/:id", (ctx) => {
    admin(ctx.ws);
    const p = find(ctx);
    panelsOf.set(ctx.ws.workspace.id, panelsState(ctx.ws).filter((x) => x !== p));
    publish(ctx.ws, "ticket_panel.deleted", { panel_id: p.id });
    audit(ctx.ws, "ticket_panel.disconnected", p);
    return respond(204);
  });

  get("/v1/tickets/:id/panels/:panelId", (ctx) => {
    const t = findTicket(ctx.ws, ctx.params.id);
    if (!t) throw notFound("Ticket");
    const p = panelsState(ctx.ws).find((x) => x.id === ctx.params.panelId && x.enabled);
    if (!p) throw notFound("Ticket panel");
    return dataFor(p, t);
  });
}

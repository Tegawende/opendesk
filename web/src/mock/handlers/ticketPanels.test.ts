import { describe, expect, it } from "vitest";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined, headers: ws ? { "OD-Workspace": ws } : {} });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

describe("mock ticket panels", () => {
  installMock();

  it("lists the seeded store and reads a ticket's contact from it", async () => {
    const panels = (await call("GET", "/v1/ticket-panels")).body.data as { id: string; kind: string }[];
    const shop = panels.find((p) => p.kind === "shopify")!;
    expect(shop).toBeTruthy();
    const tickets = (await call("GET", "/v1/tickets")).body.data as { id: string; contact: { email: string | null } | null }[];
    const withEmail = tickets.find((t) => t.contact?.email)!;
    const d = (await call("GET", `/v1/tickets/${withEmail.id}/panels/${shop.id}`)).body;
    expect(["ok", "empty"]).toContain(d.state);
    if (d.state === "ok") {
      expect(d.orders.length).toBeGreaterThan(0);
      expect(d.orders[0].items[0].price).toMatch(/^\d+\.\d\d$/);
      expect(d.orders[0].shipping).toMatchObject({ city: expect.any(String), country: expect.stringMatching(/^[A-Z]{2}$/) });
      expect(Array.isArray(d.orders[0].discount_codes)).toBe(true);
    }
  });

  it("leaves the customer's tags out when the panel says so, for Shopify panels only", async () => {
    const panels = (await call("GET", "/v1/ticket-panels")).body.data as { id: string; kind: string; show_tags: boolean }[];
    const shop = panels.find((p) => p.kind === "shopify")!;
    expect(shop.show_tags).toBe(true);
    const tickets = (await call("GET", "/v1/tickets")).body.data as { id: string; contact: { email: string | null } | null }[];
    const ids = tickets.filter((t) => t.contact?.email).map((t) => t.id);
    const read = async () => {
      for (const id of ids) {
        const d = (await call("GET", `/v1/tickets/${id}/panels/${shop.id}`)).body;
        if (d.state === "ok") return d;
      }
      throw new Error("no contact the store knows");
    };
    expect((await read()).customer.tags.length).toBeGreaterThan(0);
    expect((await call("PATCH", `/v1/ticket-panels/${shop.id}`, { show_tags: false })).body.show_tags).toBe(false);
    expect((await read()).customer.tags).toEqual([]);
    await call("PATCH", `/v1/ticket-panels/${shop.id}`, { show_tags: true });
    const made = await call("POST", "/v1/ticket-panels", { kind: "http", url: "https://erp.example.com/tags" });
    expect((await call("PATCH", `/v1/ticket-panels/${made.body.panel.id}`, { show_tags: false })).status).toBe(422);
    await call("DELETE", `/v1/ticket-panels/${made.body.panel.id}`);
  });

  it("adds an http panel with a secret shown once, refuses bad input, and removes it", async () => {
    expect((await call("POST", "/v1/ticket-panels", { kind: "http", url: "ftp://erp.example.com" })).status).toBe(422);
    expect((await call("POST", "/v1/ticket-panels", { kind: "shopify", shop: "evil.example.com", token: "x" })).status).toBe(422);
    const made = await call("POST", "/v1/ticket-panels", { kind: "http", url: "https://erp.example.com/panel", name: "Odoo" });
    expect(made.status).toBe(201);
    expect(made.body.secret).toMatch(/^pnsec_/);
    expect(made.body.panel).toMatchObject({ kind: "http", name: "Odoo", host: "erp.example.com" });
    const tickets = (await call("GET", "/v1/tickets")).body.data as { id: string }[];
    const d = (await call("GET", `/v1/tickets/${tickets[0]!.id}/panels/${made.body.panel.id}`)).body;
    expect(d.sections[0].items.length).toBeGreaterThan(0);
    const off = await call("PATCH", `/v1/ticket-panels/${made.body.panel.id}`, { enabled: false });
    expect(off.body.enabled).toBe(false);
    expect((await call("GET", `/v1/tickets/${tickets[0]!.id}/panels/${made.body.panel.id}`)).status).toBe(404);
    expect((await call("DELETE", `/v1/ticket-panels/${made.body.panel.id}`)).status).toBe(204);
  });

  it("connects a store with an app's client ID and secret, or a token, and keeps the client ID on a new secret", async () => {
    const base = { kind: "shopify", shop: "lumen-store" };
    expect((await call("POST", "/v1/ticket-panels", { ...base, client_id: "app-id" })).status).toBe(422);
    expect((await call("POST", "/v1/ticket-panels", { ...base, client_id: "app-id", client_secret: "s", token: "shpat_x" })).status).toBe(422);
    const made = await call("POST", "/v1/ticket-panels", { ...base, client_id: "app-id", client_secret: "s" });
    expect(made.body.panel).toMatchObject({ auth: "client_credentials", client_id: "app-id" });
    expect(JSON.stringify(made.body)).not.toContain('"s"');
    const id = made.body.panel.id;
    expect((await call("PATCH", `/v1/ticket-panels/${id}`, { client_secret: "s2" })).body).toMatchObject({ auth: "client_credentials", client_id: "app-id" });
    const token = (await call("PATCH", `/v1/ticket-panels/${id}`, { token: "shpat_old" })).body;
    expect(token.auth).toBe("access_token");
    expect(token.client_id).toBeUndefined();
    expect((await call("PATCH", `/v1/ticket-panels/${id}`, { client_secret: "s3" })).status).toBe(422);
    await call("DELETE", `/v1/ticket-panels/${id}`);
  });
});

import { describe, expect, it } from "vitest";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined, headers: ws ? { "OD-Workspace": ws } : {} });
  return { status: r.status, body: await r.json() };
}

describe("mock customer portal", () => {
  installMock();

  it("starts a request that then shows in the customer's list with their words", async () => {
    const slug = db.workspaces[0].brands.find((b) => b.help_center.enabled)!.slug;
    const before = (await call("GET", "/v1/customer/portal/tickets")).body.data.length;
    const made = await call("POST", "/v1/customer/portal/tickets", { workspace_slug: slug, subject: "  Printer   is down ", body: "  It will not print  " });
    expect(made.status).toBe(201);
    expect(made.body).toMatchObject({ subject: "Printer is down", status: "open" });
    const list = (await call("GET", "/v1/customer/portal/tickets")).body.data;
    expect(list.length).toBe(before + 1);
    expect((await call("GET", `/v1/customer/portal/tickets/${made.body.id}/messages`)).body.data).toMatchObject([{ direction: "inbound", body_text: "It will not print" }]);
    expect((await call("POST", "/v1/customer/portal/tickets", { workspace_slug: slug, subject: " ", body: "x" })).status).toBe(422);
    expect((await call("POST", "/v1/customer/portal/tickets", { workspace_slug: "nope", subject: "x", body: "x" })).status).toBe(404);
  });
});

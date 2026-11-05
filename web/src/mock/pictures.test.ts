import { describe, expect, it } from "vitest";
import { installMock } from "./index";
import { dispatch } from "./router";
import { db } from "./db";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, {
    method,
    body: body instanceof FormData ? body : body ? JSON.stringify(body) : undefined,
    headers: ws ? { "OD-Workspace": ws } : {},
  });
  return { status: r.status, body: r.status === 204 || r.status === 202 ? null : await r.json() };
}

const form = (bytes: number, type: string) => {
  const fd = new FormData();
  fd.append("file", new File([new Uint8Array(bytes)], "pic", { type }));
  return fd;
};

describe("mock pictures, assignment and onboarding", () => {
  installMock();

  it("seeds a mix of contacts and companies with and without pictures", async () => {
    const contacts = (await call("GET", "/v1/contacts?limit=100")).body.data as { avatar_url: string | null; avatar_source: string | null }[];
    expect(contacts.some((c) => c.avatar_url)).toBe(true);
    expect(contacts.some((c) => !c.avatar_url && c.avatar_source === null)).toBe(true);
    const companies = (await call("GET", "/v1/companies")).body.data as { avatar_source: string | null }[];
    expect(new Set(companies.map((c) => c.avatar_source))).toEqual(new Set(["upload", "domain", null]));
    const members = (await call("GET", "/v1/members")).body.data as { presence: string; user: { avatar_source: string | null } }[];
    expect(members.some((m) => m.user.avatar_source === "oauth")).toBe(true);
    expect(members.some((m) => m.presence === "offline")).toBe(true);
  });

  it("uploads and removes pictures with the API's checks", async () => {
    expect((await call("PUT", "/v1/me/avatar", form(10, "image/svg+xml"))).body.code).toBe("unsupported_image");
    expect((await call("PUT", "/v1/me/avatar", form(6 << 20, "image/png"))).status).toBe(413);
    const up = await call("PUT", "/v1/me/avatar", form(10, "image/png"));
    expect(up.body.avatar_source).toBe("upload");
    expect(up.body.avatar_url).toMatch(/^data:image\/png;base64,/);
    expect((await call("DELETE", "/v1/me/avatar")).body.avatar_url).toBeNull();

    const contact = (await call("GET", "/v1/contacts?limit=1")).body.data[0];
    expect((await call("PUT", `/v1/contacts/${contact.id}/avatar`, form(10, "image/webp"))).body.avatar_source).toBe("upload");
    expect((await call("POST", `/v1/contacts/${contact.id}/avatar/refresh`)).status).toBe(202);
    const ws = (await call("PUT", "/v1/workspace/images/help_center_logo", form(10, "image/png"))).body;
    expect(ws.images.help_center_logo).toBeTruthy();
    expect((await call("PUT", "/v1/workspace/images/banner", form(10, "image/png"))).status).toBe(422);
  });

  it("validates assignment, availability and business hours", async () => {
    const a = await call("PATCH", "/v1/assignment", { mode: "load_balanced", auto_away_minutes: 15 });
    expect(a.body).toMatchObject({ mode: "load_balanced", auto_away_minutes: 15 });
    expect((await call("PATCH", "/v1/assignment", { auto_away_minutes: 5 })).status).toBe(422);
    expect(a.body.new_ticket_notify).toBe("auto");
    expect((await call("PATCH", "/v1/assignment", { new_ticket_notify: "leads" })).body.new_ticket_notify).toBe("leads");
    expect((await call("PATCH", "/v1/assignment", { new_ticket_notify: "nobody" })).status).toBe(422);
    expect(a.body.followup_keeps_assignee).toBe(false);
    expect((await call("PATCH", "/v1/assignment", { followup_keeps_assignee: true })).body.followup_keeps_assignee).toBe(true);
    expect((await call("PATCH", "/v1/assignment", { followup_keeps_assignee: "yes" })).status).toBe(422);
    expect((await call("PATCH", "/v1/members/me/availability", { availability: "away" })).body.availability).toBe("away");
    const bh = (await call("GET", "/v1/business-hours")).body;
    expect((await call("PUT", "/v1/business-hours", { ...bh, schedule: { ...bh.schedule, mon: [{ start: "09:10", end: "17:00" }] } })).status).toBe(422);
    const ok = await call("PUT", "/v1/business-hours", { ...bh, enabled: true, timezone: "Europe/Budapest" });
    expect(ok.body).toMatchObject({ enabled: true, timezone: "Europe/Budapest" });
    expect((await call("PATCH", "/v1/triage", { fields: ["priority"], assign_after_triage: true })).body).toMatchObject({ fields: ["priority"], assign_after_triage: true });
  });

  it("computes and dismisses the setup checklist", async () => {
    const o = (await call("GET", "/v1/onboarding")).body;
    expect(o.total).toBe(9);
    expect(o.steps.map((s: { id: string }) => s.id)).toContain("try_ollie");
    expect(o.steps.find((s: { id: string }) => s.id === "setup_ai_agent")).toMatchObject({ href: "/settings/ai/agent", requires: "admin" });
    expect(o.steps.find((s: { id: string }) => s.id === "connect_mailbox").done).toBe(true);
    await call("POST", "/v1/onboarding/dismiss");
    expect((await call("GET", "/v1/onboarding")).body.dismissed).toBe(true);
  });
});

import { describe, expect, it } from "vitest";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";
import type { Ticket } from "@/lib/api/types";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, {
    method,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    headers: ws ? { "OD-Workspace": ws } : {},
  });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

describe("mock ticket aliases", () => {
  installMock();

  it.each([
    "ABCDEFAB-CDEF-0000-0000-ABCDEFABCDEF",
    "ABCDEFABCDEF00000000ABCDEFABCDEF",
    "AABCDEFAB-CDEF-0000-0000-ABCDEFABCDEFZ",
    "{ABCDEFAB-CDEF-0000-0000-ABCDEFABCDEF}",
    "urn:uuid:ABCDEFAB-CDEF-0000-0000-ABCDEFABCDEF",
    "00000000-0000-0000-0000-000000000000",
  ])("reserves UUID reference %s", async (alias) => {
    const tickets: Ticket[] = (await call("GET", "/v1/tickets")).body.data;
    const first = tickets[0];
    expect((await call("PATCH", `/v1/tickets/${first.id}`, { alias, subject: "Changed" })).status).toBe(422);
    const unchanged = (await call("GET", `/v1/tickets/${first.id}`)).body;
    expect(unchanged.subject).toBe(first.subject);
    expect(unchanged.alias ?? null).toBe(first.alias ?? null);
  });

  it("sets unique aliases and supports search and ticket lookup", async () => {
    const tickets = (await call("GET", "/v1/tickets")).body.data as Ticket[];
    const [first, second] = tickets;
    expect(first).toBeTruthy();
    expect(second).toBeTruthy();

    expect((await call("PATCH", `/v1/tickets/${first.id}`, { alias: " issue-217 " })).status).toBe(200);
    const search = (await call("GET", "/v1/tickets?q=issue-217")).body.data as Ticket[];
    expect(search.map((ticket) => ticket.id)).toContain(first.id);
    expect((await call("GET", "/v1/tickets/issue-217")).body.id).toBe(first.id);
    for (const reference of [second.id.toUpperCase(), second.id.replaceAll("-", ""), `A${second.id}Z`]) {
      expect((await call("GET", `/v1/tickets/${reference}`)).body.id).toBe(second.id);
      expect((await call("PATCH", `/v1/tickets/${first.id}`, { alias: reference })).status).toBe(422);
    }
    expect((await call("PATCH", `/v1/tickets/${second.id}`, { alias: "ISSUE-217" })).status).toBe(409);
    expect((await call("PATCH", `/v1/tickets/${first.id}`, { alias: "217" })).status).toBe(422);
    await call("PATCH", `/v1/tickets/${first.id}`, { alias: null });
  });
});

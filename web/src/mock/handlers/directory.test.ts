import { describe, expect, it } from "vitest";
import type { Company, Contact } from "@/lib/api/types";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";

async function list<T>(path: string): Promise<T[]> {
  const ws = db.workspaces[0].workspace.id;
  const r = await dispatch(`http://mock${path}`, { method: "GET", headers: { "OD-Workspace": ws } });
  return (await r.json()).data as T[];
}

describe("mock people and companies lists", () => {
  installMock();

  it("sorts people by name and tickets, and filters on blocked", async () => {
    const byName = await list<Contact>("/v1/contacts?sort=name&limit=100");
    const names = byName.map((c) => c.name.toLowerCase());
    expect(names).toEqual([...names].sort((a, b) => a.localeCompare(b)));
    expect((await list<Contact>("/v1/contacts?sort=-name&limit=100")).map((c) => c.id)).toEqual(byName.map((c) => c.id).reverse());

    const tickets = (await list<Contact>("/v1/contacts?sort=-tickets&limit=100")).map((c) => c.ticket_count);
    expect(tickets).toEqual([...tickets].sort((a, b) => b - a));

    db.workspaces[0].contacts[0].blocked = true;
    const blocked = await list<Contact>("/v1/contacts?blocked=true&limit=100");
    expect(blocked.length).toBeGreaterThan(0);
    expect(blocked.every((c) => c.blocked)).toBe(true);
    expect((await list<Contact>("/v1/contacts?blocked=false&limit=100")).some((c) => c.blocked)).toBe(false);
  });

  it("sorts companies by name, size and age", async () => {
    const asc = (await list<Company>("/v1/companies?limit=100")).map((c) => c.name.toLowerCase());
    expect(asc).toEqual([...asc].sort((a, b) => a.localeCompare(b)));
    expect((await list<Company>("/v1/companies?sort=-name&limit=100")).map((c) => c.name.toLowerCase())).toEqual([...asc].reverse());
    const sizes = (await list<Company>("/v1/companies?sort=-contacts&limit=100")).map((c) => c.contact_count);
    expect(sizes).toEqual([...sizes].sort((a, b) => b - a));
  });
});

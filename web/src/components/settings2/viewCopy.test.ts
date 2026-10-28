import { beforeAll, describe, expect, it } from "vitest";
import { i18n, loadAgentCatalogs } from "@/i18n";
import type { Team, View } from "@/lib/api/types";
import { canWriteView, copyPersonalNote, copyView, makeRoom, viewRights } from "./viewCopy";

const view = (id: string, position: number, extra: Partial<View> = {}): View => ({ id, name: `View ${id}`, shared: false, filters: {}, position, team_id: null, ...extra });
const team = (id: string, name: string, members: Team["members"]) => ({ id, name, members }) as Team;

const billing = view("b", 2, { name: "Billing queue", shared: true, filters: { status: ["open", "pending"], tag: "refund", channel_not: ["chat"] } });
const legal = view("l", 3, { name: "Legal", team_id: "t-legal", filters: { priority: ["urgent"] } });
const teams = [team("t-legal", "Legal", [{ user_id: "lea", role: "lead" }, { user_id: "jonas", role: "member" }])];

const admin = viewRights(true, teams, "maya");
const lead = viewRights(false, teams, "lea");
const agent = viewRights(false, teams, "jonas");

beforeAll(() => Promise.all([loadAgentCatalogs("fr"), loadAgentCatalogs("de")]));

describe("copyView", () => {
  it("names the copy in the person's language", () => {
    expect(copyView(billing, i18n.getFixedT("en", "settingsInbox")("views.copyName", { name: billing.name }), admin).name).toBe("Billing queue (copy)");
    expect(i18n.getFixedT("fr", "settingsInbox")("views.copyName", { name: billing.name })).toBe("Billing queue (copie)");
    expect(i18n.getFixedT("de", "settingsInbox")("views.copyName", { name: billing.name })).toBe("Billing queue (Kopie)");
  });

  it("keeps the filters, as a copy of their own", () => {
    const c = copyView(billing, "Billing queue (copy)", admin);
    expect(c.filters).toEqual(billing.filters);
    c.filters.status!.push("solved");
    expect(billing.filters.status).toEqual(["open", "pending"]);
  });

  it("leaves the id and place out and carries every other field", () => {
    const withSection = { ...legal, section: "Escalations" } as View & { section: string };
    const c = copyView(withSection, "Legal (copy)", admin);
    expect(c).toEqual({ name: "Legal (copy)", shared: false, team_id: "t-legal", filters: { priority: ["urgent"] }, section: "Escalations" });
    expect(c).not.toHaveProperty("id");
    expect(c).not.toHaveProperty("position");
  });

  it("keeps who sees it when the person may create it for them", () => {
    expect(copyView(billing, "x", admin)).toMatchObject({ shared: true, team_id: null });
    expect(copyView(legal, "x", admin)).toMatchObject({ shared: false, team_id: "t-legal" });
    expect(copyView(legal, "x", lead)).toMatchObject({ shared: false, team_id: "t-legal" });
    expect(copyView(view("p", 0), "x", agent)).toMatchObject({ shared: false, team_id: null });
  });

  it("makes the copy personal when the person may not share it with the same people", () => {
    expect(copyView(billing, "x", agent)).toMatchObject({ shared: false, team_id: null, filters: billing.filters });
    expect(copyView(billing, "x", lead)).toMatchObject({ shared: false, team_id: null });
    expect(copyView(legal, "x", agent)).toMatchObject({ shared: false, team_id: null, filters: legal.filters });
  });

  it("says why the copy is personal", () => {
    const t = i18n.getFixedT("en", "settingsInbox");
    expect(copyPersonalNote(t, billing, teams)).toBe("This copy is yours alone, since only admins create views for everyone.");
    expect(copyPersonalNote(t, legal, teams)).toBe("This copy is yours alone, since only admins and the leads of Legal create its views.");
    expect(copyPersonalNote(t, legal, [])).toBe("This copy is yours alone, since only admins and the team's leads create its views.");
  });
});

describe("canWriteView", () => {
  it("follows the server's rules for creating views", () => {
    expect(canWriteView(agent, { shared: false })).toBe(true);
    expect(canWriteView(agent, { shared: true })).toBe(false);
    expect(canWriteView(agent, { shared: false, team_id: "t-legal" })).toBe(false);
    expect(canWriteView(lead, { shared: false, team_id: "t-legal" })).toBe(true);
    expect(canWriteView(lead, { shared: false, team_id: "t-other" })).toBe(false);
    expect(canWriteView(admin, { shared: true })).toBe(true);
  });
});

describe("makeRoom", () => {
  const list = [view("a", 0, { shared: true }), view("b", 1), view("c", 2, { shared: true }), view("d", 3), view("e", 9)];

  it("moves the views after the original down one, up to the first gap", () => {
    expect(makeRoom(list, "a", 1, () => true)).toEqual([
      { id: "b", position: 2 },
      { id: "c", position: 3 },
      { id: "d", position: 4 },
    ]);
    expect(makeRoom(list, "d", 4, () => true)).toEqual([]);
    expect(makeRoom(list, "e", 10, () => true)).toEqual([]);
  });

  it("leaves the views the person may not move where they are", () => {
    expect(makeRoom(list, "a", 1, (v) => canWriteView(agent, v))).toEqual([{ id: "b", position: 2 }]);
    const tied = [view("a", 0), view("b", 1, { shared: true }), view("c", 1), view("d", 2)];
    expect(makeRoom(tied, "a", 1, (v) => canWriteView(agent, v))).toEqual([
      { id: "c", position: 2 },
      { id: "d", position: 3 },
    ]);
  });

  it("moves nothing when the original is gone", () => {
    expect(makeRoom(list, "zz", 1, () => true)).toEqual([]);
  });
});

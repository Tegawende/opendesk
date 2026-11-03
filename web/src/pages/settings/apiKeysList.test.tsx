import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n, loadAgentCatalogs } from "@/i18n";
import type { ApiKey } from "@/lib/api/types";
import { ConfirmProvider } from "@/components/ui/confirm";
import { TooltipProvider } from "@/components/ui/tooltip";
import { qk } from "@/lib/queries";
import { useSession } from "@/lib/session";
import ApiKeys from "./ApiKeys";
import { ScopeSummary, groupScopes } from "./apiKeysList";

const requests = vi.fn(async (_id: string, _q: Record<string, unknown>) => ({ data: [], next_cursor: null }));
vi.mock("@/lib/api/endpoints", async (orig) => ({
  ...(await orig<object>()),
  api: {
    apiKeys: { list: async () => ({ data: [] }), requests: (id: string, q: Record<string, unknown>) => requests(id, q) },
    bots: { list: async () => ({ data: [] }) },
  },
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
};

const DAY = 86400_000;
const key = (over: Partial<ApiKey>): ApiKey =>
  ({
    id: over.name,
    prefix: "od_live_abcd1234",
    scopes: ["tickets:read"],
    last_used_at: null,
    expires_at: null,
    created_at: "2026-11-02T10:00:00Z",
    created_by: null,
    bot_id: null,
    record_requests: false,
    error_count_24h: null,
    ...over,
  }) as ApiKey;

const keys = [
  key({ name: "Returns portal", scopes: ["contacts:read", "contacts:write", "tickets:read", "tickets:write"], record_requests: true, error_count_24h: 2 }),
  key({ name: "Claims bot", scopes: ["tickets:read", "tickets:write"], record_requests: true, error_count_24h: 0 }),
  key({ name: "Automation", scopes: ["contacts:read", "contacts:write", "kb:read", "kb:write", "realtime", "settings:read", "settings:write", "tickets:read", "tickets:write"] }),
  key({ name: "Wide", scopes: ["changelog:read", "changelog:write", "contacts:read", "contacts:write", "kb:read", "kb:write", "settings:read", "settings:write", "tickets:read", "tickets:write", "wiki:read", "wiki:write"] }),
  key({ name: "Reader", scopes: ["contacts:read", "kb:read", "changelog:read", "wiki:read", "settings:read", "tickets:read"] }),
  key({ name: "Soon", expires_at: new Date(Date.now() + 3 * DAY).toISOString() }),
  key({ name: "Old export", expires_at: new Date(Date.now() - 3 * DAY).toISOString() }),
];

let root: Root | undefined;
beforeAll(() => loadAgentCatalogs("fr"));
beforeEach(async () => {
  requests.mockClear();
  await i18n.changeLanguage("fr");
  const ws = { id: "w1", name: "Acme", slug: "acme" };
  useSession.setState({ status: "authed", user: { id: "me", email: "owner@example.com" }, workspaceId: ws.id, memberships: [{ workspace: ws, role: "admin" }] } as never);
});
afterEach(async () => {
  await act(async () => root?.unmount());
  root = undefined;
  await i18n.changeLanguage("en");
  document.body.innerHTML = "";
});

async function render() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  qc.setQueryData(qk.apiKeys, keys);
  const host = document.createElement("div");
  document.body.appendChild(host);
  const r = createRoot(host);
  root = r;
  await act(async () =>
    r.render(
      <QueryClientProvider client={qc}>
        <TooltipProvider>
          <ConfirmProvider>
            <ApiKeys />
          </ConfirmProvider>
        </TooltipProvider>
      </QueryClientProvider>,
    ),
  );
}
const settle = () => act(() => new Promise((r) => setTimeout(r, 20)));
const rowOf = (name: string) => [...document.querySelectorAll(".group")].find((r) => r.querySelector("span.truncate")?.textContent === name) as HTMLElement | undefined;

describe("groupScopes", () => {
  it("gives one entry per resource with the permissions granted", () => {
    expect(groupScopes(["tickets:read", "tickets:write", "contacts:read", "realtime", "collab:draft"])).toEqual([
      { resource: "tickets", level: "readWrite" },
      { resource: "contacts", level: "read" },
      { resource: "realtime", level: "subscribe" },
      { resource: "collab", level: "draft" },
    ]);
  });
});

describe("The API keys list", () => {
  const chip = (name: string, label: string) => rowOf(name)!.querySelector(`[aria-label="${label}"]`) as HTMLElement | null;

  it("gives each resource a chip whose tooltip says it in words, three then +N", async () => {
    await render();
    expect(chip("Returns portal", "Tickets : lecture et écriture")).not.toBeNull();
    expect(chip("Returns portal", "Contacts : lecture et écriture")).not.toBeNull();
    // Automation: tickets, contacts, articles, then settings and realtime in the +2.
    expect(chip("Automation", "Articles : lecture et écriture")).not.toBeNull();
    expect(chip("Automation", "Réglages : lecture et écriture")).toBeNull();
    expect(chip("Automation", "Réglages : lecture et écriture, Temps réel : événements en direct (abonnement)")?.textContent).toBe("+2");
    expect(rowOf("Reader")!.textContent).toContain("Lecture seule");
  });

  it("draws only the levels a key has: an eye, then the pencil for write", async () => {
    await render();
    const icons = (el: HTMLElement) => [...el.querySelectorAll("svg")].map((s) => s.getAttribute("class") ?? "");
    const [eye, pencil] = icons(chip("Returns portal", "Tickets : lecture et écriture")!);
    expect(eye).toContain("text-tone-violet");
    expect(pencil).toContain("text-tone-amber");
    expect(icons(chip("Soon", "Tickets : lecture seule")!)).toHaveLength(1);
  });

  it("opens the key's permissions from anywhere in the rights cell, +N included", async () => {
    await render();
    const cell = rowOf("Automation")!.querySelector<HTMLButtonElement>('button[aria-label="Voir et modifier les autorisations: Automation"]')!;
    // A real button: Enter and Space press it.
    expect(cell.tagName).toBe("BUTTON");
    expect(cell.type).toBe("button");
    await act(async () => cell.querySelector<HTMLElement>("[data-more]")!.click());
    await settle();
    expect(document.body.textContent).toContain("Autorisations de Automation");
    await act(async () => root?.unmount());
    root = undefined;
    document.body.innerHTML = "";

    await render();
    await act(async () => rowOf("Returns portal")!.querySelector<HTMLButtonElement>('button[aria-label="Voir et modifier les autorisations: Returns portal"]')!.click());
    await settle();
    expect(document.body.textContent).toContain("Autorisations de Returns portal");
  });

  it("stays plain text with the rest on hover where the reader may not edit", async () => {
    const host = document.createElement("div");
    document.body.appendChild(host);
    const r = createRoot(host);
    root = r;
    await act(async () =>
      r.render(
        <TooltipProvider>
          <ScopeSummary scopes={keys.find((k) => k.name === "Automation")!.scopes} full={false} readOnly={false} />
        </TooltipProvider>,
      ),
    );
    expect(host.querySelector("button")).toBeNull();
    expect(host.querySelector("[data-more]")!.getAttribute("aria-label")).toBe("Réglages : lecture et écriture, Temps réel : événements en direct (abonnement)");
  });

  it("drops the Active pill and shows the expiry as a dash, soon in amber", async () => {
    await render();
    for (const name of ["Returns portal", "Claims bot", "Automation", "Soon"]) expect(rowOf(name)!.textContent).not.toMatch(/Active|Expire bientôt/);
    expect(rowOf("Claims bot")!.querySelector('[aria-label="Sans expiration"]')?.textContent).toBe("—");
    const soon = [...rowOf("Soon")!.querySelectorAll("span")].find((s) => s.className.includes("text-tone-amber"));
    expect(soon).toBeDefined();
  });

  it("puts the failures of the last 24 hours in the Journal column, in red when there are some", async () => {
    await render();
    const red = rowOf("Returns portal")!.querySelector("[data-errors]")!;
    expect(red.textContent).toBe("2 erreurs en 24 h");
    expect(red.tagName).toBe("BUTTON");
    expect(red.className).toContain("text-danger-fg");
    const muted = rowOf("Claims bot")!.querySelector("[data-errors]")!;
    expect(muted.textContent).toBe("0 erreur en 24 h");
    expect(muted.tagName).toBe("SPAN");
    expect(muted.className).toContain("text-fg-4");
    expect(rowOf("Automation")!.querySelector('[aria-label="Requêtes non enregistrées"]')?.textContent).toBe("—");
  });

  it("opens the request log from the Journal column, the errors on their failures of the last 24 hours", async () => {
    await render();
    await act(async () => rowOf("Returns portal")!.querySelector<HTMLButtonElement>('button[aria-label="Ouvrir le journal des requêtes: Returns portal"]')!.click());
    await settle();
    expect(document.body.textContent).toContain("Journal des requêtes de Returns portal");
    expect(requests).toHaveBeenLastCalledWith("Returns portal", { limit: 50, cursor: null });
    await act(async () => root?.unmount());
    root = undefined;
    document.body.innerHTML = "";

    await render();
    const before = Date.now();
    await act(async () => rowOf("Returns portal")!.querySelector<HTMLButtonElement>("button[data-errors]")!.click());
    await settle();
    const q = requests.mock.lastCall![1] as { status: string; since: string };
    expect(q.status).toBe("errors");
    expect(Math.abs(before - 86400_000 - Date.parse(q.since))).toBeLessThan(5_000);
    expect(document.body.textContent).toContain("Depuis le");
  });

  it("folds the expired keys at the bottom", async () => {
    await render();
    expect(rowOf("Old export")).toBeUndefined();
    const fold = [...document.querySelectorAll("button")].find((b) => b.textContent === "Clés expirées (1)")!;
    expect(fold.getAttribute("aria-expanded")).toBe("false");
    await act(async () => fold.click());
    await settle();
    expect(rowOf("Old export")).toBeDefined();
    expect(rowOf("Old export")!.textContent).toContain("Expirée");
  });
});

// Review reproduction: write-only credentials do not have read access.
it("shows only write permission for a write-only key", async () => {
  const host = document.createElement("div"); document.body.appendChild(host);
  const r = createRoot(host); root = r;
  await act(async () => r.render(<TooltipProvider><ScopeSummary scopes={["tickets:write"]} full={false} readOnly={false} /></TooltipProvider>));
  expect(host.querySelectorAll("svg.lucide-eye")).toHaveLength(0);
  expect(host.querySelectorAll("svg.lucide-pencil")).toHaveLength(1);
});

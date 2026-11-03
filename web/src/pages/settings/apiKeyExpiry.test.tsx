import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n, loadAgentCatalogs } from "@/i18n";
import { ApiError } from "@/lib/api/client";
import type { ApiKey } from "@/lib/api/types";
import { ConfirmProvider } from "@/components/ui/confirm";
import { TooltipProvider } from "@/components/ui/tooltip";
import { qk } from "@/lib/queries";
import { useSession } from "@/lib/session";
import ApiKeys from "./ApiKeys";
import { ExpiryDialog, expiryWidens } from "./apiKeyExpiry";

const update = vi.fn<(id: string, b: { expires_at?: string | null }) => Promise<ApiKey>>();
vi.mock("@/lib/api/endpoints", async (orig) => ({
  ...(await orig<object>()),
  api: {
    apiKeys: {
      list: async () => ({ data: [] }),
      requests: async () => ({ data: [], next_cursor: null }),
      update: (id: string, b: { expires_at?: string | null }) => update(id, b),
    },
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
const day = (ms: number) => {
  const d = new Date(ms);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};
const key = (over: Partial<ApiKey> = {}): ApiKey =>
  ({
    id: "k1",
    name: "Quotes board",
    prefix: "od_live_abcd1234",
    scopes: ["tickets:read"],
    last_used_at: null,
    expires_at: new Date(Date.now() + 90 * DAY).toISOString(),
    created_at: "2026-11-02T10:00:00Z",
    created_by: null,
    bot_id: null,
    record_requests: false,
    error_count_24h: null,
    ...over,
  }) as ApiKey;

let root: Root | undefined;
const onClose = vi.fn();

beforeAll(() => loadAgentCatalogs("fr"));
beforeEach(async () => {
  await i18n.changeLanguage("fr");
  update.mockReset();
  onClose.mockReset();
  const ws = { id: "w1", name: "Acme", slug: "acme" };
  useSession.setState({ status: "authed", user: { id: "me", email: "owner@example.com" }, workspaceId: ws.id, memberships: [{ workspace: ws, role: "admin" }] } as never);
});
afterEach(async () => {
  await act(async () => root?.unmount());
  root = undefined;
  await i18n.changeLanguage("en");
  document.body.innerHTML = "";
});

async function mount(node: React.ReactNode, keys: ApiKey[]) {
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
          <ConfirmProvider>{node}</ConfirmProvider>
        </TooltipProvider>
      </QueryClientProvider>,
    ),
  );
}

const settle = () => act(() => new Promise((r) => setTimeout(r, 20)));
const button = (label: string) => [...document.querySelectorAll("button")].find((b) => b.textContent === label)!;
const text = () => document.body.textContent ?? "";
const dateInput = () => document.querySelector<HTMLInputElement>('input[type="date"]')!;
async function pick(value: string) {
  const input = dateInput();
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

describe("Change expiry", () => {
  it("opens on the key's own date with nothing to save", async () => {
    const k = key();
    await mount(<ExpiryDialog apiKey={k} onClose={onClose} />, [k]);
    expect(dateInput().value).toBe(day(Date.parse(k.expires_at!)));
    expect(button("Enregistrer l’expiration").disabled).toBe(true);
  });

  it("shortens at once, without the sign-in note", async () => {
    const k = key();
    update.mockImplementation(async (_id, b) => key({ expires_at: b.expires_at }));
    await mount(<ExpiryDialog apiKey={k} onClose={onClose} />, [k]);
    await pick(day(Date.now() + 10 * DAY));
    expect(text()).not.toContain("connexion récente");
    await act(async () => button("Enregistrer l’expiration").click());
    await settle();
    const sent = update.mock.calls[0][1].expires_at!;
    expect(day(Date.parse(sent))).toBe(day(Date.now() + 10 * DAY));
    expect(onClose).toHaveBeenCalled();
  });

  it("says a later date needs a recent sign-in, and shows the sign-in notice on that refusal", async () => {
    const k = key();
    update.mockRejectedValue(
      new ApiError({ status: 403, code: "reauth_required", title: "Forbidden", detail: "For your security, sign in again to do this. Your last sign-in is more than 12 hours old." }),
    );
    await mount(<ExpiryDialog apiKey={k} onClose={onClose} />, [k]);
    await pick(day(Date.now() + 400 * DAY));
    expect(text()).toContain("demande une connexion récente");
    await act(async () => button("Enregistrer l’expiration").click());
    await settle();
    expect(onClose).not.toHaveBeenCalled();
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("12");
  });

  it("refuses a day that isn't in the future", async () => {
    const k = key();
    await mount(<ExpiryDialog apiKey={k} onClose={onClose} />, [k]);
    await pick(day(Date.now() - DAY));
    expect(text()).toContain("Choisissez un jour après aujourd’hui");
    expect(button("Enregistrer l’expiration").disabled).toBe(true);
  });

  it("tells widening from narrowing", () => {
    const now = new Date().toISOString();
    const later = new Date(Date.now() + DAY).toISOString();
    expect(expiryWidens(now, null)).toBe(true);
    expect(expiryWidens(null, now)).toBe(false);
    expect(expiryWidens(now, later)).toBe(true);
    expect(expiryWidens(later, now)).toBe(false);
    expect(expiryWidens(null, null)).toBe(false);
  });
});

describe("The expiry cell", () => {
  it("opens the dialog for a live key, and not for an expired one", async () => {
    const live = key({ id: "a", name: "Quotes board" });
    const dead = key({ id: "b", name: "Old export", expires_at: new Date(Date.now() - 3 * DAY).toISOString() });
    await mount(<ApiKeys />, [live, dead]);
    expect(document.querySelector('[aria-label="Modifier l’expiration de Old export"]')).toBeNull();
    const cell = document.querySelector<HTMLButtonElement>('[aria-label="Modifier l’expiration de Quotes board"]')!;
    expect(cell).not.toBeNull();
    await act(async () => cell.click());
    await settle();
    expect(text()).toContain("Expiration de Quotes board");
  });
});

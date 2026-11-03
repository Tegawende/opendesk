import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n, loadAgentCatalogs } from "@/i18n";
import type { ApiKey, ApiKeyRequest, ApiKeyRequestDetail, Page, ApiKeyRequestFilter } from "@/lib/api/types";
import { TooltipProvider } from "@/components/ui/tooltip";
import { qk } from "@/lib/queries";
import { useSession } from "@/lib/session";
import { ApiKeyRequestsDrawer, ago, pretty } from "./ApiKeyRequests";

type Query = ApiKeyRequestFilter & { limit?: number; cursor?: string | null };
const requests = vi.fn<(id: string, q: Query) => Promise<Page<ApiKeyRequest>>>();
const request = vi.fn<(id: string, rid: string) => Promise<ApiKeyRequestDetail>>();
const update = vi.fn<(id: string, b: { record_requests?: boolean }) => Promise<ApiKey>>();
vi.mock("@/lib/api/endpoints", () => ({
  api: {
    apiKeys: {
      requests: (id: string, q: Query) => requests(id, q),
      request: (id: string, rid: string) => request(id, rid),
      update: (id: string, b: { record_requests?: boolean }) => update(id, b),
    },
  },
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const key = (over: Partial<ApiKey> = {}): ApiKey =>
  ({
    id: "k1",
    name: "Returny",
    prefix: "od_live_abcd1234",
    scopes: ["tickets:read", "tickets:write"],
    last_used_at: null,
    expires_at: null,
    created_at: "2026-11-02T10:00:00Z",
    created_by: null,
    bot_id: null,
    record_requests: true,
    ...over,
  }) as ApiKey;

const row = (over: Partial<ApiKeyRequest> = {}): ApiKeyRequest => ({
  id: "r1",
  created_at: new Date(Date.now() - 120_000).toISOString(),
  method: "POST",
  path: "/v1/tickets/4242/messages",
  query: "",
  status: 201,
  duration_ms: 80,
  idempotency_key: "returny-R-1",
  replayed: false,
  ticket: { id: "t1", number: 4242 },
  ticket_ref: "4242",
  message_id: "m1",
  contact_id: null,
  ...over,
});

let root: Root;
let qc: QueryClient;
const onClose = vi.fn();

beforeAll(() => loadAgentCatalogs("fr"));
beforeEach(() => {
  requests.mockReset();
  request.mockReset();
  update.mockReset();
  useSession.setState({ user: { id: "me", email: "owner@example.com" } as never, workspaceId: "w1" });
});
afterEach(async () => {
  await act(async () => root.unmount());
  await i18n.changeLanguage("en");
  document.body.innerHTML = "";
});

async function render(k: ApiKey, initial?: Parameters<typeof ApiKeyRequestsDrawer>[0]["initial"]) {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  qc.setQueryData(qk.apiKeys, [k]);
  const host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => root.render(<QueryClientProvider client={qc}>
        <TooltipProvider>
          <ApiKeyRequestsDrawer apiKey={k} onClose={onClose} initial={initial} />
        </TooltipProvider>
      </QueryClientProvider>));
  await settle();
}

const settle = () => act(() => new Promise((r) => setTimeout(r, 30)));
const button = (label: string) => [...document.querySelectorAll("button")].find((b) => b.textContent === label)!;
const click = (el: HTMLElement) => act(async () => el.click());
const text = () => document.body.textContent ?? "";
const rows = () => [...document.querySelectorAll('[role="table"] [role="row"]')].slice(1);

describe("An API key's requests", () => {
  it("filters on every failure with Errors, 4xx and 5xx together", async () => {
    requests.mockResolvedValue({ data: [row({ status: 503 })], next_cursor: null });
    await render(key());
    await click(button("Errors"));
    await settle();
    expect(requests).toHaveBeenLastCalledWith("k1", { limit: 50, cursor: null, status: "errors" });
  });

  it("opens already filtered, and the date filter can be taken off", async () => {
    requests.mockResolvedValue({ data: [row({ status: 422 })], next_cursor: null });
    await render(key(), { status: "errors", since: "2026-11-02T10:00:00.000Z" });
    expect(requests).toHaveBeenLastCalledWith("k1", { limit: 50, cursor: null, status: "errors", since: "2026-11-02T10:00:00.000Z" });
    const since = document.querySelector<HTMLButtonElement>('button[aria-label="Show every date"]')!;
    expect(since.textContent).toContain("Since");
    await click(since);
    await settle();
    expect(requests).toHaveBeenLastCalledWith("k1", { limit: 50, cursor: null, status: "errors" });
  });

  it("keeps the date and error filter while paging, and resets paging when status changes", async () => {
    const since = "2026-11-02T10:00:00.000Z";
    requests.mockImplementation(async (_id, q) => ({
      data: q.cursor ? [row({ id: "r2", status: 503 })] : [row({ status: 422 })],
      next_cursor: q.cursor || q.status !== "errors" ? null : "c1",
    }));
    await render(key(), { status: "errors", since });
    await click(button("Load older requests"));
    await settle();
    expect(requests).toHaveBeenLastCalledWith("k1", { limit: 50, cursor: "c1", status: "errors", since });
    expect(rows()).toHaveLength(2);
    await click(button("5xx"));
    await settle();
    expect(requests).toHaveBeenLastCalledWith("k1", { limit: 50, cursor: null, status: "5xx", since });
    expect(rows()).toHaveLength(1);
    await click(button("All"));
    await settle();
    expect(requests).toHaveBeenLastCalledWith("k1", { limit: 50, cursor: null, since });
  });

  it("shows a filtered empty state when only a date filter is active", async () => {
    requests.mockResolvedValue({ data: [], next_cursor: null });
    await render(key(), { since: "2026-11-02T10:00:00.000Z" });
    expect(text()).toContain(i18n.t("settingsDev:apiKeys.requests.emptyFiltered"));
    expect(text()).not.toContain(i18n.t("settingsDev:apiKeys.requests.emptyHint"));
  });

  it("says when a request was made in the past tense, never as a negative duration", async () => {
    const now = Date.parse("2026-11-03T10:00:00Z");
    // Intl spaces these with no-break spaces.
    const at = (iso: string) => ago(iso, now).replace(/\s/g, " ");
    await i18n.changeLanguage("fr");
    expect(at("2026-11-03T09:58:00Z")).toBe("il y a 2 min");
    expect(at("2026-11-03T07:00:00Z")).not.toMatch(/^-/);
    await i18n.changeLanguage("de");
    expect(at("2026-11-03T09:58:00Z")).toBe("vor 2 Min.");
    await i18n.changeLanguage("en");
    expect(at("2026-11-03T09:58:00Z")).toBe("2m");
    await render(key());
  });

  it("lists them with their status and ticket, and filters by answer and ticket", async () => {
    requests.mockImplementation(async (_id, q) => ({
      data: q.status === "4xx" ? [row({ id: "r2", status: 422, ticket: null, ticket_ref: "999" })] : [row(), row({ id: "r2", status: 422, ticket: null, ticket_ref: "999" })],
      next_cursor: null,
    }));
    await render(key());
    expect(requests).toHaveBeenCalledWith("k1", { limit: 50, cursor: null });
    expect(rows()).toHaveLength(2);
    expect(text()).toContain("/v1/tickets/4242/messages");
    const link = document.querySelector<HTMLAnchorElement>('a[href="/inbox/open/t1"]');
    expect(link?.textContent).toBe("#4242");
    expect(text()).toContain("999");

    await click(button("4xx"));
    await settle();
    expect(requests).toHaveBeenLastCalledWith("k1", { limit: 50, cursor: null, status: "4xx" });
    expect(rows()).toHaveLength(1);

    const input = document.querySelector<HTMLInputElement>('input[aria-label="Filter by ticket"]')!;
    await act(async () => {
      const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
      set.call(input, "ext:555");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => input.form!.requestSubmit());
    await settle();
    expect(requests).toHaveBeenLastCalledWith("k1", { limit: 50, cursor: null, status: "4xx", ticket: "ext:555" });
  });

  it("opens a request with what it sent, indented, and the answer to a failure", async () => {
    requests.mockResolvedValue({ data: [row({ status: 422 })], next_cursor: null });
    request.mockResolvedValue({
      ...row({ status: 422 }),
      request_id: "req_1",
      ip: "203.0.113.7",
      user_agent: "returns-portal/1.4",
      content_type: "application/json",
      request_size: 40,
      request_body: '{"kind":"note","password":"***"}',
      request_truncated: false,
      response_body: '{"code":"validation_failed"}',
      response_truncated: true,
    });
    await render(key());
    await click(rows()[0] as HTMLElement);
    await settle();
    expect(request).toHaveBeenCalledWith("k1", "r1");
    const sent = document.querySelector('pre[aria-label="Sent"]')!;
    expect(sent.textContent).toBe('{\n  "kind": "note",\n  "password": "***"\n}');
    expect(document.querySelector('pre[aria-label="Answer"]')?.textContent).toContain('"code": "validation_failed"');
    expect(text()).toContain("Capture exceeded 8 KB. Only safe diagnostics are kept.");
    expect(text()).toContain("returny-R-1");
    await click(button("All requests"));
    expect(rows()).toHaveLength(1);
  });

  it("offers to turn recording on when it is off", async () => {
    requests.mockResolvedValue({ data: [], next_cursor: null });
    update.mockImplementation(async (_id, b) => key({ record_requests: b.record_requests }));
    await render(key({ record_requests: false }));
    expect(text()).toContain("Recording is off");
    await click(button("Turn on recording"));
    await settle();
    expect(update).toHaveBeenCalledWith("k1", { record_requests: true });
    expect(text()).not.toContain("Recording is off");
    expect(qc.getQueryData<ApiKey[]>(qk.apiKeys)?.[0].record_requests).toBe(true);
  });

  it("loads older pages", async () => {
    requests.mockImplementation(async (_id, q) => (q.cursor ? { data: [row({ id: "r9" })], next_cursor: null } : { data: [row()], next_cursor: "c1" }));
    await render(key());
    await click(button("Load older requests"));
    await settle();
    expect(requests).toHaveBeenLastCalledWith("k1", { limit: 50, cursor: "c1" });
    expect(rows()).toHaveLength(2);
  });

  it("is in French", async () => {
    await i18n.changeLanguage("fr");
    requests.mockResolvedValue({ data: [], next_cursor: null });
    await render(key());
    expect(text()).toContain("Aucune requête pour l’instant");
  });
});

describe("pretty", () => {
  it("indents JSON and leaves anything else", () => {
    expect(pretty('{"a":1}')).toBe('{\n  "a": 1\n}');
    expect(pretty('{"cut')).toBe('{"cut');
  });
});

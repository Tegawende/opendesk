import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot } from "react-dom/client";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { customerPortal } from "@/lib/customerPortal/client";
import { NewRequest } from "./PortalNewRequest";

const navigate = vi.fn();
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, className }: { children: ReactNode; className?: string }) => <a href="#" className={className}>{children}</a>,
  useNavigate: () => navigate,
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
afterEach(() => {
  vi.restoreAllMocks();
  navigate.mockReset();
  document.body.innerHTML = "";
});

async function mount() {
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  await act(async () => root.render(<QueryClientProvider client={client}><NewRequest slug="acme" /></QueryClientProvider>));
  return host;
}
const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
const setText = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
async function fill(host: HTMLElement, subject: string, body: string) {
  const input = host.querySelector<HTMLInputElement>('input[name="subject"]')!;
  const area = host.querySelector<HTMLTextAreaElement>('textarea[name="body"]')!;
  await act(async () => {
    setValue.call(input, subject);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    setText.call(area, body);
    area.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
const sendButton = (host: HTMLElement) => host.querySelector<HTMLButtonElement>('button[type="submit"]')!;
const submit = (host: HTMLElement) => act(async () => host.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));

describe("the portal's new request form", () => {
  it("asks for a subject and a message, and nothing the team sets", async () => {
    const host = await mount();
    expect(host.textContent).toContain("New request");
    expect([...host.querySelectorAll("input, textarea")].map((e) => e.getAttribute("name"))).toEqual(["subject", "body"]);
    expect(sendButton(host).disabled).toBe(true);
    await fill(host, "Order is late", "   ");
    expect(sendButton(host).disabled).toBe(true);
    await fill(host, "Order is late", "It has not arrived");
    expect(sendButton(host).disabled).toBe(false);
    expect(host.querySelector<HTMLInputElement>('input[name="subject"]')!.maxLength).toBe(200);
    expect(host.querySelector<HTMLTextAreaElement>('textarea[name="body"]')!.maxLength).toBe(10000);
  });

  it("sends the trimmed words for this portal and opens the request it made", async () => {
    const create = vi.spyOn(customerPortal, "create").mockResolvedValue({ id: "t-9", number: 9, subject: "Order is late", status: "open", created_at: "", updated_at: "" });
    const host = await mount();
    await fill(host, "  Order is late ", "  It has not arrived\n");
    await submit(host);
    expect(create).toHaveBeenCalledWith("acme", "Order is late", "It has not arrived");
    expect(navigate).toHaveBeenCalledWith({ to: "/portal/$slug/tickets/$ticketId", params: { slug: "acme", ticketId: "t-9" } });
  });

  it("says so when too many requests were sent, and keeps what was written", async () => {
    vi.spyOn(customerPortal, "create").mockRejectedValue(new ApiError({ status: 429, code: "rate_limited" }));
    const host = await mount();
    await fill(host, "Again", "Another one");
    await submit(host);
    await vi.waitFor(() => expect(host.textContent).toContain("several requests in a row"));
    expect(navigate).not.toHaveBeenCalled();
    expect(host.querySelector<HTMLInputElement>('input[name="subject"]')!.value).toBe("Again");
  });

  it("shows the general message for any other failure", async () => {
    vi.spyOn(customerPortal, "create").mockRejectedValue(new ApiError({ status: 500 }));
    const host = await mount();
    await fill(host, "x", "y");
    await submit(host);
    await vi.waitFor(() => expect(host.textContent).toContain("Something went wrong"));
  });
});

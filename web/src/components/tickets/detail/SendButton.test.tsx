import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n, loadAgentCatalogs } from "@/i18n";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { TicketStatus, Workspace, WorkspaceSettings } from "@/lib/api/types";
import { qk } from "@/lib/queries";
import { nextLayout, SendButton, sendChoices } from "./SendButton";

beforeAll(() => loadAgentCatalogs("fr"));

const none = () => false;

describe("sendChoices", () => {
  const base = { side: false, current: "open", finish: "solved", hidden: none, button: false, fits: true } satisfies Parameters<typeof sendChoices>[0];

  it("keeps every status in the menu while the button is off", () => {
    expect(sendChoices(base)).toEqual({ button: null, menu: ["pending", "on_hold", "solved"] });
  });

  it("moves the finishing status to its own button, out of the menu", () => {
    expect(sendChoices({ ...base, button: true })).toEqual({ button: "solved", menu: ["pending", "on_hold"] });
  });

  it("folds the button back into the menu when it does not fit", () => {
    expect(sendChoices({ ...base, button: true, fits: false })).toEqual({ button: null, menu: ["pending", "on_hold", "solved"] });
  });

  it("offers no button when the ticket is already in the finishing status", () => {
    expect(sendChoices({ ...base, button: true, current: "solved" })).toEqual({ button: null, menu: ["pending", "on_hold"] });
  });

  it("leaves hidden statuses out of the menu", () => {
    expect(sendChoices({ ...base, button: true, hidden: (s) => s === "pending" || s === "on_hold" })).toEqual({ button: "solved", menu: [] });
  });

  it("keeps a side conversation to on hold, without a button", () => {
    expect(sendChoices({ ...base, side: true, button: true })).toEqual({ button: null, menu: ["on_hold"] });
  });
});

describe("nextLayout", () => {
  // A 110 px segment; folding the formatting buttons frees 112 px.
  const button = 110;
  const saved = 112;

  it("keeps every formatting button while the segment fits beside them", () => {
    expect(nextLayout("wide", 5, button, saved)).toBe("wide");
    expect(nextLayout("menu", 120, button, saved)).toBe("wide");
  });

  it("folds the formatting buttons before the segment goes back into the menu", () => {
    expect(nextLayout("wide", 0, button, saved)).toBe("folded");
    expect(nextLayout("menu", 40, button, saved)).toBe("folded");
    expect(nextLayout("folded", 30, button, saved)).toBe("folded");
  });

  it("puts the segment in the menu only when even folding leaves no room", () => {
    expect(nextLayout("folded", 0, button, saved)).toBe("menu");
    expect(nextLayout("menu", 0, button, saved)).toBe("menu");
    expect(nextLayout("menu", 40, button, 0)).toBe("menu");
  });

  it("unfolds once there is room for everything, and never retakes a layout found squeezed", () => {
    expect(nextLayout("folded", 120, button, saved)).toBe("wide");
    expect(nextLayout("folded", 120, button, saved, new Set(["wide"]))).toBe("folded");
    expect(nextLayout("menu", 120, button, saved, new Set(["wide", "folded"]))).toBe("menu");
  });
});

describe("SendButton", () => {
  let host: HTMLDivElement;
  let root: Root;

  beforeEach(async () => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    await i18n.changeLanguage("en");
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.innerHTML = "";
    await i18n.changeLanguage("en");
    vi.unstubAllGlobals();
  });

  type Props = Partial<Parameters<typeof SendButton>[0]>;
  async function render(settings: WorkspaceSettings, props: Props = {}) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
    qc.setQueryData<Workspace>(qk.workspace, { id: "w", name: "Acme", settings } as Workspace);
    const onSend = vi.fn<(s?: TicketStatus) => void>();
    await act(async () =>
      root.render(
        <QueryClientProvider client={qc}>
          <TooltipProvider>
            <SendButton note={false} disabled={false} busy={false} onSend={onSend} current="open" phase="idle" {...props} />
          </TooltipProvider>
        </QueryClientProvider>,
      ),
    );
    const buttons = () => [...host.querySelectorAll("button")];
    const named = (text: string) => buttons().find((b) => b.textContent === text);
    const openMenu = async () => {
      const trigger = host.querySelector<HTMLButtonElement>("button[aria-haspopup='menu']")!;
      await act(async () => trigger.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true })));
      return [...document.querySelectorAll("[role='menuitem']")].map((i) => i.querySelector(".flex-1")?.textContent);
    };
    return { onSend, buttons, named, openMenu };
  }

  it("is today's split button while the setting is off", async () => {
    const { named, openMenu } = await render({});
    expect(named("Send")).toBeTruthy();
    expect(named("Send and solve")).toBeUndefined();
    expect(await openMenu()).toEqual(["Send and mark pending", "Send and mark on hold", "Send and mark solved"]);
  });

  it("shows Send and solve next to Send, which sends and solves", async () => {
    const { named, onSend, openMenu } = await render({ send_and_finish_button: true });
    const finish = named("Send and solve")!;
    expect(finish).toBeTruthy();
    expect(finish.disabled).toBe(false);
    await act(async () => finish.click());
    expect(onSend).toHaveBeenCalledWith("solved");
    expect(await openMenu()).toEqual(["Send and mark pending", "Send and mark on hold"]);
  });

  it("labels finishing as Close or a custom name while sending solved", async () => {
    const { named, onSend } = await render({ send_and_finish_button: true, status_labels: { solved: { label: "Closed" }, closed: { hidden: true } } });
    expect(named("Send and close")).toBeTruthy();
    await act(async () => named("Send and close")!.click());
    expect(onSend).toHaveBeenCalledWith("solved");
    await act(async () => root.unmount());
    root = createRoot(host);
    const renamed = await render({ send_and_finish_button: true, status_labels: { solved: { label: "Done" }, closed: { hidden: true } } });
    expect(renamed.named("Send and mark as Done")).toBeTruthy();
    await act(async () => renamed.named("Send and mark as Done")!.click());
    expect(renamed.onSend).toHaveBeenCalledWith("solved");
  });

  it("sends solved even with older settings that hide solved", async () => {
    const { onSend } = await render({
      send_and_finish_button: true,
      status_labels: { solved: { hidden: true }, pending: { hidden: true }, on_hold: { hidden: true } },
    });
    const finish = host.querySelector<HTMLButtonElement>("[data-send-finish]");
    expect(finish).toBeTruthy();
    await act(async () => finish?.click());
    expect(onSend).toHaveBeenCalledWith("solved");
  });

  it("the send menu finishes as solved when terminal Closed is hidden", async () => {
    const { onSend, openMenu } = await render({
      status_labels: { solved: { label: "Closed" }, closed: { hidden: true }, pending: { hidden: true }, on_hold: { hidden: true } },
    });
    expect(await openMenu()).toEqual(["Send and mark closed"]);
    const item = document.querySelector<HTMLElement>("[role='menuitem']");
    expect(item).toBeTruthy();
    await act(async () => item?.click());
    expect(onSend).toHaveBeenCalledWith("solved");
  });

  it("follows the menu for a note, and offers no button in a side conversation", async () => {
    const note = await render({ send_and_finish_button: true }, { note: true });
    expect(note.named("Add note and solve")).toBeTruthy();
    await act(async () => root.unmount());
    root = createRoot(host);
    const side = await render({ send_and_finish_button: true }, { sideTo: ["vendor@example.com"] });
    expect(side.buttons().some((b) => b.textContent?.includes("solve"))).toBe(false);
    expect(await side.openMenu()).toEqual(["Send and mark on hold"]);
  });

  it("is disabled with Send, and offers nothing on a solved ticket", async () => {
    const off = await render({ send_and_finish_button: true }, { disabled: true });
    expect(off.named("Send")!.disabled).toBe(true);
    expect(off.named("Send and solve")!.disabled).toBe(true);
    await act(async () => root.unmount());
    root = createRoot(host);
    const solved = await render({ send_and_finish_button: true }, { current: "solved" });
    expect(solved.named("Send and solve")).toBeUndefined();
  });

  it("asks for room for the segment, and lets the toolbar unfold without it", async () => {
    const onFold = vi.fn<(f: boolean) => void>();
    await render({ send_and_finish_button: true }, { onFold });
    // Nothing is squeezed here (no layout in jsdom), so it never asks to fold.
    expect(onFold).toHaveBeenLastCalledWith(false);
  });

  it("speaks French", async () => {
    await i18n.changeLanguage("fr");
    const { named, onSend } = await render({ send_and_finish_button: true, status_labels: { solved: { label: "Closed" }, closed: { hidden: true } } });
    expect(named("Envoyer")).toBeTruthy();
    expect(named("Envoyer et fermer")).toBeTruthy();
    await act(async () => named("Envoyer et fermer")!.click());
    expect(onSend).toHaveBeenCalledWith("solved");
  });
});


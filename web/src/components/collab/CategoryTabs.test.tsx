import { createMemoryHistory, createRootRoute, createRoute, createRouter, Outlet, RouterProvider } from "@tanstack/react-router";
import { act, type DragEvent } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { i18n } from "@/i18n";
import type { MailCategory } from "@/lib/api/types";
import type { CategoryTab } from "@/lib/collab/categories";
import { startThreadDrag, takeThreadDrops, THREAD_DRAG, type ThreadDropTarget } from "@/lib/collab/threadDrag";
import { CategoryTabs } from "./CategoryTabs";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const tab = (key: string, name: string, color: MailCategory["color"] = "gray"): CategoryTab => ({ key, name, description: "", color, unread: 0 });
const TABS = [tab("primary", "Primary"), tab("codes", "Codes / Logins", "green"), tab("updates", "Updates", "blue")];

const cleanups: (() => void | Promise<void>)[] = [];
beforeEach(() => i18n.changeLanguage("en"));
afterEach(async () => {
  for (const c of cleanups.splice(0)) await c();
  document.body.innerHTML = "";
});

async function render(active = "primary") {
  const rootRoute = createRootRoute({ component: Outlet });
  const page = createRoute({ getParentRoute: () => rootRoute, path: "/mail/$box/{-$threadId}", component: () => <CategoryTabs tabs={TABS} active={active} search={{}} settings={null} /> });
  const router = createRouter({ routeTree: rootRoute.addChildren([page]), history: createMemoryHistory({ initialEntries: ["/mail/inbox"] }), defaultPendingMinMs: 0 });
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  cleanups.push(() => act(async () => root.unmount()));
  await router.load();
  await act(async () => root.render(<RouterProvider router={router} />));
}

/** A drag's data, as the browser keeps it between the row it started on and where it is dropped. */
function transfer() {
  const data = new Map<string, string>();
  return {
    get types() {
      return [...data.keys()];
    },
    setData: (k: string, v: string) => void data.set(k, v),
    getData: (k: string) => data.get(k) ?? "",
    setDragImage: () => {},
    effectAllowed: "all",
    dropEffect: "none",
  };
}

/** Drags these conversations from the list, as a row does. */
function dragRows(ids: string[]) {
  const dt = transfer();
  startThreadDrag({ dataTransfer: dt } as unknown as DragEvent, ids, `${ids.length} conversations`);
  return dt;
}

function fire(el: Element, type: "dragover" | "drop" | "dragleave", dt: ReturnType<typeof transfer>) {
  const e = new Event(type, { bubbles: true, cancelable: true });
  Object.defineProperty(e, "dataTransfer", { value: dt });
  act(() => void el.dispatchEvent(e));
  return e;
}

const link = (name: string) => [...document.querySelectorAll("a")].find((a) => a.textContent?.includes(name))!;

function takeDrops(accepts: (ids: string[], to: ThreadDropTarget) => boolean = () => true) {
  const drop = vi.fn();
  cleanups.push(takeThreadDrops(drop, accepts));
  return drop;
}

it("takes conversations dropped on another category's tab, lit while they are over it, and hands them to the page", async () => {
  const drop = takeDrops();
  await render();
  const dt = dragRows(["t1", "t2"]);
  const over = fire(link("Codes / Logins"), "dragover", dt);
  expect(over.defaultPrevented).toBe(true);
  expect(dt.dropEffect).toBe("move");
  expect(link("Codes / Logins").className).toContain("bg-chrome-hover");
  fire(link("Codes / Logins"), "drop", dt);
  expect(drop).toHaveBeenCalledWith(["t1", "t2"], { box: "inbox", category: "codes" });
  expect(link("Codes / Logins").className).not.toContain("bg-chrome-hover");
});

it("offers no drop where the page says the conversations' accounts lack the category", async () => {
  const accepts = vi.fn((_ids: string[], to: ThreadDropTarget) => to.category !== "updates");
  const drop = takeDrops(accepts);
  await render();
  const dt = dragRows(["t1"]);
  expect(fire(link("Updates"), "dragover", dt).defaultPrevented).toBe(false);
  expect(link("Updates").className).not.toContain("bg-chrome-hover");
  expect(accepts).toHaveBeenCalledWith(["t1"], { box: "inbox", category: "updates" });
  fire(link("Updates"), "drop", dt);
  expect(drop).not.toHaveBeenCalled();
  expect(fire(link("Codes / Logins"), "dragover", dt).defaultPrevented).toBe(true);
});

it("does nothing with conversations dropped on the tab in view, where they already are", async () => {
  const drop = takeDrops();
  await render("codes");
  const dt = dragRows(["t1"]);
  expect(fire(link("Codes / Logins"), "dragover", dt).defaultPrevented).toBe(false);
  fire(link("Codes / Logins"), "drop", dt);
  expect(drop).not.toHaveBeenCalled();
  expect(fire(link("Primary"), "dragover", dt).defaultPrevented).toBe(true);
});

it("ignores a drag that carries no conversations, such as a sidebar row", async () => {
  takeDrops();
  await render();
  const dt = transfer();
  dt.setData("application/x-od-sidebar-row", "{}");
  expect(dt.types).not.toContain(THREAD_DRAG);
  expect(fire(link("Updates"), "dragover", dt).defaultPrevented).toBe(false);
});

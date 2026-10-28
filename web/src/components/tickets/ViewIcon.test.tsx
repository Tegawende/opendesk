import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it } from "vitest";
import type { ViewIcon as ViewIconValue } from "@/lib/api/types";
import { ViewIcon, isOneEmoji, viewIconTone, viewName } from "./ViewIcon";
import { VIEW_ICONS, searchViewIcons, viewIconDef } from "./viewIcons";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

afterEach(() => {
  document.body.innerHTML = "";
});

async function render(icon: ViewIconValue | null | undefined, size?: number) {
  const host = document.createElement("div");
  document.body.appendChild(host);
  const root = createRoot(host);
  await act(async () => root.render(<ViewIcon icon={icon} size={size} />));
  return host;
}

const emoji = (value: string): ViewIconValue => ({ kind: "emoji", value, color: null });

it("shows the layers icon without an icon", async () => {
  const host = await render(null);
  const svg = host.querySelector("svg")!;
  expect(svg.getAttribute("data-view-icon")).toBe("none");
  expect(svg.getAttribute("class")).toContain("lucide-layers");
  expect(svg.getAttribute("style")).toBeNull();
});

it("shows an icon of the list, tinted in its colour", async () => {
  const host = await render({ kind: "icon", value: "truck", color: "teal" });
  const svg = host.querySelector("svg")!;
  expect(svg.getAttribute("data-view-icon")).toBe("icon");
  expect(svg.getAttribute("class")).toContain("lucide-truck");
  expect(svg.style.color).toBe("var(--tone-teal)");
  const plain = await render({ kind: "icon", value: "truck", color: null });
  expect(plain.querySelector("svg")!.getAttribute("style")).toBeNull();
});

it("never renders a name outside the list", async () => {
  const host = await render({ kind: "icon", value: "rocket", color: "red" });
  expect(host.querySelector("svg")!.getAttribute("class")).toContain("lucide-layers");
  expect(viewIconDef("constructor")).toBeUndefined();
  expect(viewIconDef("__proto__")).toBeUndefined();
});

it("shows an emoji in a box the size of the icons beside it, never tinted", async () => {
  const host = await render({ kind: "emoji", value: "💸", color: "red" });
  const span = host.querySelector<HTMLElement>('[data-view-icon="emoji"]')!;
  expect(span.textContent).toBe("💸");
  expect(span.getAttribute("aria-hidden")).toBe("true");
  expect(span.style.width).toBe("15px");
  expect(span.style.height).toBe("15px");
  expect(span.style.color).toBe("");
  expect(host.querySelector("svg")).toBeNull();
  expect(viewIconTone({ kind: "emoji", value: "💸", color: "red" })).toBeNull();
  const big = await render(emoji("💸"), 16);
  expect(big.querySelector<HTMLElement>('[data-view-icon="emoji"]')!.style.height).toBe("16px");
});

it("shows one or two characters on a tile of its colour", async () => {
  const host = await render({ kind: "text", value: "PO", color: "violet" });
  const tile = host.querySelector<HTMLElement>('[data-view-icon="text"]')!;
  expect(tile.textContent).toBe("PO");
  expect(tile.className).toContain("bg-tone-violet-soft");
  expect(tile.style.width).toBe("15px");
  const gray = await render({ kind: "text", value: "B", color: null });
  expect(gray.querySelector('[data-view-icon="text"]')!.className).toContain("bg-tone-gray-soft");
});

it("lists each icon once, by a stable lucide name, and finds them by their words", () => {
  const names = VIEW_ICONS.map((d) => d.name);
  expect(new Set(names).size).toBe(names.length);
  expect(names.length).toBeGreaterThanOrEqual(60);
  expect(names.length).toBeLessThanOrEqual(80);
  for (const d of VIEW_ICONS) {
    expect(d.name).toMatch(/^[a-z][a-z0-9]*(-[a-z0-9]+)*$/);
    expect(d.icon).toBeTruthy();
  }
  expect(names).toContain("layers");
  expect(searchViewIcons("refund").map((d) => d.name)).toEqual(expect.arrayContaining(["undo-2", "banknote"]));
  expect(searchViewIcons("shopping c").map((d) => d.name)).toEqual(["shopping-cart"]);
  expect(searchViewIcons("zzz")).toEqual([]);
  expect(searchViewIcons("  ")).toHaveLength(VIEW_ICONS.length);
});

it("tells one emoji from anything else", () => {
  for (const s of ["🔥", "☑️", "👨‍👩‍👧", "🇧🇪", "👋🏽", "1️⃣", "#⃣"]) expect(isOneEmoji(s), s).toBe(true);
  for (const s of ["", "a", "🔥🔥", "🔥 x", "PO", "<b>", "1"]) expect(isOneEmoji(s), s).toBe(false);
});

it("shows a name's leading emoji once when it is the view's icon", () => {
  expect(viewName({ name: "💸 Billing queue", icon: emoji("💸") })).toBe("Billing queue");
  expect(viewName({ name: "💸  Billing queue", icon: emoji("💸") })).toBe("Billing queue");
  // With or without the variation selector on either side.
  expect(viewName({ name: "☑️ Done", icon: emoji("☑") })).toBe("Done");
  expect(viewName({ name: "☑ Done", icon: emoji("☑️") })).toBe("Done");
  // Another emoji, a longer one that starts the same, no space after it, or nothing else.
  expect(viewName({ name: "🔥 Urgent", icon: emoji("💸") })).toBe("🔥 Urgent");
  expect(viewName({ name: "👍🏽 Thanks", icon: emoji("👍") })).toBe("👍🏽 Thanks");
  expect(viewName({ name: "👨‍👩‍👧 Family", icon: emoji("👨") })).toBe("👨‍👩‍👧 Family");
  expect(viewName({ name: "🔥Urgent", icon: emoji("🔥") })).toBe("🔥Urgent");
  expect(viewName({ name: "🔥", icon: emoji("🔥") })).toBe("🔥");
  // An icon, text or nothing leaves the name alone.
  expect(viewName({ name: "PO Orders", icon: { kind: "text", value: "PO", color: null } })).toBe("PO Orders");
  expect(viewName({ name: "🔥 Urgent", icon: { kind: "icon", value: "flame", color: "red" } })).toBe("🔥 Urgent");
  expect(viewName({ name: "🔥 Urgent", icon: null })).toBe("🔥 Urgent");
});

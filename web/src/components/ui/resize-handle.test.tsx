import { act, useRef } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import "@/i18n";
import { paneLimits, STEP } from "@/lib/panes";
import { ResizeHandle } from "./resize-handle";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let host: HTMLDivElement;
let root: Root;
// jsdom lays nothing out: the pane reports the width its style says.
const widthOf = (el: HTMLElement) => parseFloat(el.style.width) || 380;

beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    return { width: widthOf(this) } as DOMRect;
  });
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.restoreAllMocks();
});

async function mount(side: "before" | "after", width: number | null = null) {
  const onWidth = vi.fn();
  const onLive = vi.fn();
  function Pane() {
    const ref = useRef<HTMLDivElement>(null);
    return (
      <div>
        <div ref={ref} data-pane style={width === null ? undefined : { width: `${width}px` }} />
        <ResizeHandle
          pane="list"
          side={side}
          target={ref}
          room={() => 2000}
          width={width}
          onLive={(px) => {
            onLive(px);
            if (ref.current) ref.current.style.width = px === null ? "" : `${px}px`;
          }}
          onWidth={onWidth}
        />
      </div>
    );
  }
  await act(async () => root.render(<Pane />));
  return { handle: host.querySelector<HTMLElement>('[role="separator"]')!, onWidth, onLive };
}

const press = (el: HTMLElement, key: string, init: KeyboardEventInit = {}) =>
  act(async () => {
    el.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init }));
  });

it("is a focusable vertical separator that says the pane's width and limits", async () => {
  const { handle } = await mount("before", 420);
  expect(handle.tabIndex).toBe(0);
  expect(handle.getAttribute("aria-orientation")).toBe("vertical");
  expect(handle.getAttribute("aria-valuenow")).toBe("420");
  expect(handle.getAttribute("aria-valuemin")).toBe(String(paneLimits.list.min));
  expect(handle.getAttribute("aria-valuemax")).toBe(String(paneLimits.list.max));
  expect(handle.getAttribute("aria-label")).toBe("Resize the ticket list");
});

it("widens a pane left of it with ArrowRight and narrows it with ArrowLeft", async () => {
  const { handle, onWidth } = await mount("before");
  await press(handle, "ArrowRight");
  expect(onWidth).toHaveBeenLastCalledWith(380 + STEP);
  expect(handle.getAttribute("aria-valuenow")).toBe(String(380 + STEP));
  await press(handle, "ArrowLeft");
  expect(onWidth).toHaveBeenLastCalledWith(380);
});

it("widens a pane right of it with ArrowLeft", async () => {
  const { handle, onWidth } = await mount("after", 300);
  await press(handle, "ArrowLeft");
  expect(onWidth).toHaveBeenLastCalledWith(300 + STEP);
});

it("goes to the ends with Home and End, and leaves other keys to the page", async () => {
  const { handle, onWidth } = await mount("before");
  await press(handle, "End");
  expect(onWidth).toHaveBeenLastCalledWith(paneLimits.list.max);
  await press(handle, "Home");
  expect(onWidth).toHaveBeenLastCalledWith(paneLimits.list.min);
  onWidth.mockClear();
  const event = new KeyboardEvent("keydown", { key: "j", bubbles: true, cancelable: true });
  await act(async () => void handle.dispatchEvent(event));
  expect(event.defaultPrevented).toBe(false);
  await press(handle, "ArrowRight", { metaKey: true });
  expect(onWidth).not.toHaveBeenCalled();
});

it("goes back to the default on a double click", async () => {
  const { handle, onWidth } = await mount("before", 500);
  await act(async () => void handle.dispatchEvent(new MouseEvent("dblclick", { bubbles: true })));
  expect(onWidth).toHaveBeenLastCalledWith(null);
});

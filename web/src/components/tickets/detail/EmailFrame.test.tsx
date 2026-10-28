import { act, useState } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { useHotkeys } from "@/lib/hotkeys";
import { Popover } from "@/components/ui/popover";
import { EmailFrame } from "./EmailFrame";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
it("forwards Escape from email with the innermost popover closing first", async () => {
  const close = vi.fn();
  function View() {
    const [open, setOpen] = useState(true);
    useHotkeys({ escape: close });
    return <><EmailFrame html="<p>Mail</p>" title="Mail" /><Popover open={open} onOpenChange={setOpen} trigger={<button>Details</button>}><p>Details here</p></Popover></>;
  }
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  await act(async () => root.render(<View />));
  const frame = host.querySelector("iframe");
  if (!frame?.contentDocument) throw new Error("Missing email document");
  const doc = frame.contentDocument;
  await act(async () => { frame.dispatchEvent(new Event("load")); });
  const escape = () => doc.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
  try {
    expect(frame.getAttribute("sandbox")).not.toContain("allow-scripts");
    await act(async () => { escape(); });
    expect(close).not.toHaveBeenCalled();
    expect(document.querySelector("[data-radix-popper-content-wrapper]")).toBeNull();
    await act(async () => { escape(); });
    expect(close).toHaveBeenCalledTimes(1);
  } finally {
    await act(async () => root.unmount());
    host.remove();
  }
});

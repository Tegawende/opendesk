import { act, createRef } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@/i18n";
import { TooltipProvider } from "@/components/ui/tooltip";
import { RichEditor, type RichEditorHandle } from "./RichEditor";

describe("RichEditor links", () => {
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
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  async function render() {
    const ref = createRef<RichEditorHandle>();
    await act(async () =>
      root.render(
        <TooltipProvider>
          <RichEditor ref={ref} value={`<p>Pay <a href="https://pay.example.test/i/42">here</a> today</p>`} />
        </TooltipProvider>,
      ),
    );
    return ref.current!.editor!;
  }

  const button = (label: string) => document.querySelector<HTMLElement>(`[aria-label="${label}"]`);

  it("opens a link on Ctrl or Cmd click, and leaves a plain click to the cursor", async () => {
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    const editor = await render();
    const link = host.querySelector<HTMLAnchorElement>(".tiptap a")!;
    // ProseMirror turns a mousedown and mouseup into a click with a document position, which needs a layout jsdom lacks.
    const click = (o: MouseEventInit) => {
      const e = new MouseEvent("click", { bubbles: true, ...o });
      Object.defineProperty(e, "target", { value: link });
      return editor.view.someProp("handleClick", (f) => f(editor.view, 7, e)) ?? false;
    };
    expect(click({})).toBe(false);
    expect(open).not.toHaveBeenCalled();
    expect(click({ ctrlKey: true })).toBe(true);
    expect(click({ metaKey: true })).toBe(true);
    expect(open).toHaveBeenCalledTimes(2);
    expect(open).toHaveBeenCalledWith("https://pay.example.test/i/42", "_blank", "noopener,noreferrer");
  });

  it("offers to open the link from its box while the address is the link's own", async () => {
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    const editor = await render();
    await act(async () => editor.commands.setTextSelection(7));
    await act(async () => button("Link")!.click());
    expect(button("Open link")).not.toBeNull();
    await act(async () => button("Open link")!.click());
    expect(open).toHaveBeenCalledWith("https://pay.example.test/i/42", "_blank", "noopener,noreferrer");

    const input = button("Link address") as HTMLInputElement;
    const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    await act(async () => {
      setValue.call(input, "https://other.example.test");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(button("Open link")).toBeNull();
  });
});

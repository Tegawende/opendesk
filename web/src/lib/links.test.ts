import { afterEach, describe, expect, it, vi } from "vitest";
import { browserClick, modClickedLink, newTabKey, openableLink, openInNewTab } from "./links";

const click = (o: Partial<Parameters<typeof browserClick>[0]> = {}) => ({ button: 0, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, defaultPrevented: false, ...o });
const key = (o: Partial<Parameters<typeof newTabKey>[0]> = {}) => ({ key: "Enter", metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, ...o });

describe("browserClick", () => {
  it("leaves a plain click to the app", () => {
    expect(browserClick(click())).toBe(false);
  });

  it("leaves a new tab, a new window or a download to the browser", () => {
    expect(browserClick(click({ ctrlKey: true }))).toBe(true);
    expect(browserClick(click({ metaKey: true }))).toBe(true);
    expect(browserClick(click({ shiftKey: true }))).toBe(true);
    expect(browserClick(click({ altKey: true }))).toBe(true);
    expect(browserClick(click({ button: 1 }))).toBe(true);
  });

  it("leaves a click a handler already took", () => {
    expect(browserClick(click({ defaultPrevented: true }))).toBe(true);
  });
});

describe("newTabKey", () => {
  it("is Ctrl+Enter or Cmd+Enter", () => {
    expect(newTabKey(key({ ctrlKey: true }))).toBe(true);
    expect(newTabKey(key({ metaKey: true }))).toBe(true);
  });

  it("is not Enter alone, another key or another chord", () => {
    expect(newTabKey(key())).toBe(false);
    expect(newTabKey(key({ key: "k", ctrlKey: true }))).toBe(false);
    expect(newTabKey(key({ ctrlKey: true, shiftKey: true }))).toBe(false);
    expect(newTabKey(key({ ctrlKey: true, altKey: true }))).toBe(false);
  });
});

describe("openInNewTab", () => {
  afterEach(() => vi.restoreAllMocks());

  it("opens the href in a new tab without a handle on this one", () => {
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    openInNewTab("/inbox/open/t1");
    expect(open).toHaveBeenCalledWith("/inbox/open/t1", "_blank", "noopener");
  });
});

describe("openableLink", () => {
  it("is a web address, not a template tag, a mail link or an empty one", () => {
    expect(openableLink("https://pay.example.test/i/42")).toBe(true);
    expect(openableLink(" http://example.test ")).toBe(true);
    for (const href of ["{{ticket.url}}", "mailto:help@example.test", "tel:+3220000000", "https://", "", null, undefined]) expect(openableLink(href), String(href)).toBe(false);
  });
});

describe("modClickedLink", () => {
  const editor = () => {
    const div = document.createElement("div");
    div.innerHTML = `<p>Pay <a href="https://pay.example.test/i/42"><strong>here</strong></a> or <a href="{{ticket.url}}">there</a></p>`;
    return div;
  };
  const click = (target: Element, o: { ctrlKey?: boolean; metaKey?: boolean } = {}) => ({ target, ctrlKey: false, metaKey: false, ...o });

  it("is the link's address on a Ctrl or Cmd click, inside its text too", () => {
    const strong = editor().querySelector("strong")!;
    expect(modClickedLink(click(strong, { ctrlKey: true }))).toBe("https://pay.example.test/i/42");
    expect(modClickedLink(click(strong, { metaKey: true }))).toBe("https://pay.example.test/i/42");
  });

  it("is nothing on a plain click, off a link, or on a link that does not open as it is", () => {
    const div = editor();
    expect(modClickedLink(click(div.querySelector("strong")!))).toBeNull();
    expect(modClickedLink(click(div.querySelector("p")!, { ctrlKey: true }))).toBeNull();
    expect(modClickedLink(click(div.querySelectorAll("a")[1], { ctrlKey: true }))).toBeNull();
  });
});

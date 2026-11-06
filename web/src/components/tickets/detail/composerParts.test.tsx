import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@/i18n";
import type * as desktop from "@/lib/desktop";
import { UploadChip } from "./composerParts";
import type { Upload } from "./outbox";

const shell = vi.hoisted(() => ({ desktop: false }));
vi.mock("@/lib/desktop", async (orig) => ({
  ...(await orig<typeof desktop>()),
  get isDesktop() {
    return shell.desktop;
  },
}));

describe("UploadChip", () => {
  let host: HTMLDivElement;
  let root: Root;
  const objectUrls = { createObjectURL: URL.createObjectURL, revokeObjectURL: URL.revokeObjectURL };

  beforeEach(async () => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    vi.useFakeTimers();
    await i18n.changeLanguage("en");
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.innerHTML = "";
    vi.useRealTimers();
    Object.assign(URL, objectUrls);
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    shell.desktop = false;
  });

  const file = new File(["%PDF-1.7"], "pro-forma.pdf", { type: "application/pdf" });

  it("opens the file in a new tab from its name, and frees it later", async () => {
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    const create = vi.fn(() => "blob:http://localhost/f1");
    const revoke = vi.fn();
    Object.assign(URL, { createObjectURL: create, revokeObjectURL: revoke });
    const onRemove = vi.fn();
    const u: Upload = { key: "k", file, status: "done" };
    await act(async () => root.render(<UploadChip u={u} onRemove={onRemove} />));

    const name = host.querySelector<HTMLButtonElement>('button[aria-label="Open pro-forma.pdf"]')!;
    expect(name.textContent).toContain("pro-forma.pdf");
    await act(async () => name.click());
    expect(create).toHaveBeenCalledWith(file);
    expect(open).toHaveBeenCalledWith("blob:http://localhost/f1", "_blank", "noopener,noreferrer");
    expect(onRemove).not.toHaveBeenCalled();
    expect(revoke).not.toHaveBeenCalled();
    vi.advanceTimersByTime(60_000);
    expect(revoke).toHaveBeenCalledWith("blob:http://localhost/f1");
  });

  it("still removes the file from its own button", async () => {
    const onRemove = vi.fn();
    await act(async () => root.render(<UploadChip u={{ key: "k", file, status: "done" }} onRemove={onRemove} />));
    await act(async () => host.querySelector<HTMLButtonElement>('button[aria-label="Remove pro-forma.pdf"]')!.click());
    expect(onRemove).toHaveBeenCalledOnce();
  });

  it("opens HTML and SVG as their text, never as a page on the app's origin", async () => {
    vi.spyOn(window, "open").mockReturnValue(null);
    const blobs: Blob[] = [];
    const create = vi.fn((b: Blob) => {
      blobs.push(b);
      return "blob:http://localhost/f";
    });
    Object.assign(URL, { createObjectURL: create, revokeObjectURL: vi.fn() });
    const files = [
      new File(["<script>alert(1)</script>"], "page.html", { type: "text/html" }),
      new File(['<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>'], "logo.svg", { type: "image/svg+xml" }),
    ];
    for (const f of files) {
      await act(async () => root.render(<UploadChip u={{ key: f.name, file: f, status: "done" }} onRemove={() => {}} />));
      await act(async () => host.querySelector<HTMLButtonElement>(`button[aria-label="Open ${f.name}"]`)!.click());
    }
    expect(blobs.map((b) => b.type)).toEqual(["text/plain;charset=utf-8", "text/plain;charset=utf-8"]);
    expect(await blobs[0].text()).toBe("<script>alert(1)</script>");
  });

  it("does not offer to open a type the browser would not show", async () => {
    const zip = new File(["PK"], "archive.zip", { type: "application/zip" });
    await act(async () => root.render(<UploadChip u={{ key: "k", file: zip, status: "done" }} onRemove={() => {}} />));
    expect(host.querySelector('button[aria-label="Open archive.zip"]')).toBeNull();
    expect(host.textContent).toContain("archive.zip");
  });

  it("does not offer to open a file in the desktop app, whose windows open only web addresses", async () => {
    shell.desktop = true;
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    await act(async () => root.render(<UploadChip u={{ key: "k", file, status: "done" }} onRemove={() => {}} />));
    expect(host.querySelector('button[aria-label="Open pro-forma.pdf"]')).toBeNull();
    expect(host.textContent).toContain("pro-forma.pdf");
    expect(open).not.toHaveBeenCalled();
  });
});

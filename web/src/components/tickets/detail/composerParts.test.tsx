import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@/i18n";
import { UploadChip } from "./composerParts";
import type { Upload } from "./outbox";

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
});

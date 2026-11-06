import { describe, expect, it } from "vitest";
import { viewType } from "./fileView";

describe("viewType", () => {
  it("opens images, PDF, plain text, audio and video as themselves", () => {
    for (const t of ["image/png", "image/jpeg", "application/pdf", "text/plain", "audio/mpeg", "video/mp4"]) expect(viewType(t)).toBe(t);
    expect(viewType("IMAGE/PNG")).toBe("image/png");
  });

  it("opens active documents as plain text", () => {
    for (const t of ["text/html", "image/svg+xml", "application/xhtml+xml", "text/xml", "application/xml", "text/javascript", "application/javascript", "application/rss+xml", "text/html; charset=utf-8"]) {
      expect(viewType(t)).toBe("text/plain;charset=utf-8");
    }
  });

  it("does not open unknown or binary types", () => {
    for (const t of ["", "application/octet-stream", "application/zip", "application/x-msdownload", "application/vnd.ms-excel"]) expect(viewType(t)).toBeNull();
  });
});

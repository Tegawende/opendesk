import { describe, expect, it } from "vitest";
import { panelUrlOk, shopDomain } from "./ticketPanels";

describe("ticket panel inputs", () => {
  it("reads a shop as the server does: a handle, its domain or an admin URL", () => {
    expect(shopDomain("lumen-store")).toBe("lumen-store.myshopify.com");
    expect(shopDomain(" Lumen-Store.myshopify.com/ ")).toBe("lumen-store.myshopify.com");
    expect(shopDomain("https://lumen-store.myshopify.com/admin/orders")).toBe("lumen-store.myshopify.com");
    expect(shopDomain("evil.example.com")).toBeNull();
    expect(shopDomain("lumen store")).toBeNull();
  });

  it("accepts https URLs, and http only on this machine", () => {
    expect(panelUrlOk("https://erp.example.com/panel?key=1")).toBe(true);
    expect(panelUrlOk("http://localhost:8080/panel")).toBe(true);
    expect(panelUrlOk("http://erp.example.com/panel")).toBe(false);
    expect(panelUrlOk("erp.example.com")).toBe(false);
  });
});

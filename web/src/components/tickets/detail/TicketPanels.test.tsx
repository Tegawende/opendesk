import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import "@/i18n";
import type { PanelOrder, TicketPanelData } from "@/lib/api/types";
import { PanelBody, shippingLine } from "./TicketPanels";

const order = (patch: Partial<PanelOrder> = {}): PanelOrder => ({
  id: "gid://shopify/Order/1001",
  number: "#1042",
  url: "https://admin.shopify.com/store/lumen-store/orders/1001",
  created_at: "2026-10-20T09:00:00Z",
  total: "119.32",
  currency: "EUR",
  financial_status: "partially_refunded",
  fulfillment_status: "fulfilled",
  cancelled: false,
  refunded: "14.44",
  discount_codes: ["WELCOME10"],
  shipping: { city: "Brussels", country: "BE", method: "DHL Express" },
  items: [
    { title: "Lucide Leanne Wall Lamp", variant: "Black", quantity: 3, price: "14.44" },
    { title: "LED bulb E27", quantity: 1, price: "12.90" },
  ],
  tracking: [{ company: "DHL", number: "JD0123", url: "https://dhl.example/track/JD0123" }],
  ...patch,
});

const data = (orders: PanelOrder[]): TicketPanelData => ({
  panel_id: "p1",
  kind: "shopify",
  state: "ok",
  fetched_at: "2026-10-27T10:00:00Z",
  customer: {
    name: "Ada Buyer",
    url: "https://admin.shopify.com/store/lumen-store/customers/42",
    orders_count: 3,
    total_spent: "412.50",
    currency: "EUR",
    tags: ["footer_signup", "newsletter"],
  },
  orders,
});

// The rendered text, read through the DOM rather than by stripping tags.
const text = (html: string) => new DOMParser().parseFromString(html, "text/html").body.textContent ?? "";

describe("Shopify ticket panel", () => {
  it("shows each line with its unit price, where the order goes and how, codes, refunds and tracking", () => {
    const html = renderToStaticMarkup(<PanelBody data={data([order()])} />);
    const t = text(html);
    expect(t).toContain("3×Lucide Leanne Wall Lamp - Black€14.44");
    expect(t).toContain("LED bulb E27€12.90");
    expect(t).toContain("Brussels, Belgium · DHL Express");
    expect(t).toContain("WELCOME10");
    expect(t).toContain("€14.44 refunded");
    expect(t).toContain("Partly refunded");
    expect(t).toContain("DHL · JD0123");
    // The order number opens the order in Shopify admin.
    expect(html).toMatch(/<a href="https:\/\/admin\.shopify\.com\/store\/lumen-store\/orders\/1001"[^>]*title="Open order #1042 in Shopify"/);
  });

  it("marks a cancelled order with its date and leaves out what is not there", () => {
    const t = text(
      renderToStaticMarkup(
        <PanelBody
          data={data([
            order({
              cancelled: true,
              cancelled_at: "2026-09-03T08:30:00Z",
              financial_status: "refunded",
              fulfillment_status: "unfulfilled",
              refunded: undefined,
              discount_codes: [],
              shipping: undefined,
              tracking: [],
            }),
          ])}
        />,
      ),
    );
    expect(t).toMatch(/Cancelled Sep 3/);
    expect(t).toContain("Refunded");
    expect(t).not.toContain("Unfulfilled");
    expect(t).not.toContain("refunded€");
    expect(t).not.toContain("DHL Express");
  });

  it("shows the customer's tags only when the panel does", () => {
    expect(text(renderToStaticMarkup(<PanelBody data={data([])} />))).toContain("footer_signup");
    const off = text(renderToStaticMarkup(<PanelBody data={data([])} showTags={false} />));
    expect(off).not.toContain("footer_signup");
    expect(off).toContain("3 orders");
  });

  it("writes the destination from what the store knows", () => {
    expect(shippingLine({ city: "Lyon", country: "FR" })).toBe("Lyon, France");
    expect(shippingLine({ method: "Pickup" })).toBe("Pickup");
    expect(shippingLine(undefined)).toBe("");
  });
});

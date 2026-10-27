import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import "@/i18n";
import type { PanelOrder, ShopifyField, TicketPanelData } from "@/lib/api/types";
import { DEFAULT_SHOPIFY_FIELDS, normalizeFields, shownData } from "@/lib/panelFields";
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
    email: "ada@example.com",
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

  it("shows what it always has until an admin chooses, the email aside", () => {
    const t = text(renderToStaticMarkup(<PanelBody data={data([order(), order({ id: "o2", number: "#1041" })])} />));
    expect(t).toContain("footer_signup");
    expect(t).toContain("3 orders · €412.50 spent");
    expect(t).not.toContain("ada@example.com");
    expect(t).toContain("#1041");
  });

  it("shows only the fields the panel shows, and as many orders", () => {
    const two = data([order(), order({ id: "o2", number: "#1041" })]);
    const render = (fields: ShopifyField[], ordersShown?: number) => text(renderToStaticMarkup(<PanelBody data={two} fields={fields} ordersShown={ordersShown} />));

    // Nothing chosen: the customer's name and each order's number, date and total.
    const bare = render([]);
    expect(bare).toContain("Ada Buyer");
    expect(bare).toContain("#1042");
    expect(bare).toContain("€119.32");
    for (const gone of ["footer_signup", "orders", "spent", "Partly refunded", "Fulfilled", "Lucide", "Brussels", "WELCOME10", "refunded", "DHL"]) expect(bare).not.toContain(gone);

    // Lines without their prices; the store's email; the spend without the count.
    const some = render(["email", "total_spent", "line_items"], 1);
    expect(some).toContain("ada@example.com");
    expect(some).toContain("€412.50 spent");
    expect(some).not.toContain("3 orders");
    expect(some).toContain("3×Lucide Leanne Wall Lamp - Black");
    expect(some).not.toContain("€14.44");
    expect(some).not.toContain("#1041");

    // Each order field on its own.
    expect(render(["financial_status"])).toContain("Partly refunded");
    expect(render(["fulfillment_status"])).toContain("Fulfilled");
    expect(render(["line_items", "line_prices"])).toContain("€12.90");
    expect(render(["shipping"])).toContain("Brussels, Belgium · DHL Express");
    expect(render(["discounts"])).toContain("WELCOME10");
    expect(render(["refunds"])).toContain("€14.44 refunded");
    expect(render(["tracking"])).toContain("DHL · JD0123");
    expect(render(["tags"])).toContain("newsletter");
  });

  it("cuts an answer to the panel's choice, the way the mock and the server do", () => {
    const d = shownData(data([order(), order({ id: "o2" })]), ["line_items", "line_prices", "email"], 1);
    expect(d.customer).toEqual({ name: "Ada Buyer", url: "https://admin.shopify.com/store/lumen-store/customers/42", tags: [], email: "ada@example.com" });
    expect(d.orders).toHaveLength(1);
    expect(d.orders![0]).not.toHaveProperty("financial_status");
    expect(d.orders![0]).not.toHaveProperty("shipping");
    expect(d.orders![0].items[0].price).toBe("14.44");
    expect(d.orders![0].tracking).toEqual([]);
    expect(normalizeFields(["tracking", "line_prices", "tags", "tags"])).toEqual(["tags", "tracking"]);
    expect(DEFAULT_SHOPIFY_FIELDS).not.toContain("email");
  });

  it("writes the destination from what the store knows", () => {
    expect(shippingLine({ city: "Lyon", country: "FR" })).toBe("Lyon, France");
    expect(shippingLine({ method: "Pickup" })).toBe("Pickup");
    expect(shippingLine(undefined)).toBe("");
  });
});

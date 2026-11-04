import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { i18n } from "@/i18n";
import type { TicketLink } from "@/lib/api/types";
import { qk } from "@/lib/queries";
import { TicketLinks } from "./TicketLinks";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

it("shows the provider's current state beside a linked issue", async () => {
  await i18n.changeLanguage("en");
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  const link: TicketLink = {
    id: "link-1",
    ticket_id: "ticket-1",
    integration_id: null,
    provider: "linear",
    url: "https://linear.app/acme/issue/ENG-123",
    title: "ENG-123",
    external_id: "ENG-123",
    external_state: "done",
    external_state_name: "Shipped",
    synced_at: "2026-11-02T12:00:00Z",
    actor_type: "user",
    created_by: "user-1",
    created_at: "2026-11-02T12:00:00Z",
  };
  client.setQueryData(qk.ticketLinks("ticket-1"), [link]);
  client.setQueryData(qk.integrations, []);
  client.setQueryData(qk.aiConfig, { assistant_name: "Ollie" });
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);

  await act(async () => {
    root.render(
      <QueryClientProvider client={client}>
        <TooltipProvider>
          <TicketLinks ticketId="ticket-1" canEdit={false} />
        </TooltipProvider>
      </QueryClientProvider>,
    );
  });

  const state = [...host.querySelectorAll("span")].find((element) => element.textContent === "Shipped");
  expect(state?.className).toContain("bg-tone-green-soft");
  expect(host.textContent).toContain("ENG-123");

  await act(async () => root.unmount());
  client.clear();
  host.remove();
});

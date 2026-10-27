-- A Shopify ticket panel can leave the customer's tags out (contracts/integrations.md "Ticket panels"):
-- many stores tag customers for marketing (newsletter, signup source), which agents need not read.
ALTER TABLE ticket_panels ADD COLUMN show_tags boolean NOT NULL DEFAULT true;

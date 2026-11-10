// The ticket's shared draft on the mock (contracts/api.md "Drafts"): one pending reply body
// per ticket, staged by a teammate or by an AI client over MCP (`draft_reply`), shown in
// every teammate's composer. A reply going out clears it (handlers/tickets.ts).

import type { Attachment, ReplyDraft, Ticket, TicketDraft } from "@/lib/api/types";
import { db, userRef, type WsData } from "../db";
import { publish } from "../gateway";
import { del, forbidden, get, invalid, notFound, put, respond, type Ctx } from "../router";
import { findTicket } from "../tickets";
import { nowIso, textToHtml } from "../util";

const store = new WeakMap<WsData, Map<string, TicketDraft>>();
const drafts = (ws: WsData) => {
  let m = store.get(ws);
  if (!m) store.set(ws, (m = new Map()));
  return m;
};

// My own unsent reply per ticket (contracts/api.md "Reply drafts"): private, kept for the session.
const own = new WeakMap<WsData, Map<string, ReplyDraft>>();
const ownDrafts = (ws: WsData) => {
  let m = own.get(ws);
  if (!m) own.set(ws, (m = new Map()));
  return m;
};

/** My reply going out ends my own saved copy of it. */
export function clearOwnDraft(ws: WsData, t: Ticket) {
  ownDrafts(ws).delete(t.id);
}

const text = (html: string) => html.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim();

function ticketOf(ctx: Ctx): Ticket {
  const t = findTicket(ctx.ws, ctx.params.id);
  if (!t) throw notFound("Ticket");
  return t;
}

/** Stages (or replaces) the ticket's draft and tells every open tab. `by` names an AI client; absent, the current user staged it. */
export function stageDraft(ws: WsData, t: Ticket, html: string, by?: string, opts: { publish?: boolean; at?: string } = {}): TicketDraft {
  const user = by ? null : userRef(db.meId);
  const d: TicketDraft = {
    ticket_id: t.id,
    body_html: html,
    body_text: text(html),
    by: user ? { type: "user", name: user.name, user } : { type: "api", name: by ?? "API" },
    updated_at: opts.at ?? nowIso(),
  };
  drafts(ws).set(t.id, d);
  if (opts.publish ?? true) publish(ws, "ticket_draft.updated", { ticket_id: t.id, draft: d });
  return d;
}

/** Removes the ticket's draft, if any, and tells every open tab it is gone. */
export function clearDraft(ws: WsData, t: Ticket) {
  if (!drafts(ws).delete(t.id)) return;
  publish(ws, "ticket_draft.updated", { ticket_id: t.id, draft: null });
}

/** An AI agent drafted a reply on Maya's SSO ticket for her to review. */
function seedDrafts(ws: WsData) {
  const t = ws.tickets.find((x) => x.number === 1072);
  if (!t) return;
  const html = textToHtml(
    [
      "Hi Dana,",
      "Thanks for tracking that down. I've added the new certificate fingerprint (4F:1A:9C:22:E0:7B:51:D3) to your SSO connection, so sign-in should go straight through again. No rollback needed on your side.",
      "Could you ask one or two people to sign out and back in to confirm? If anyone still loops back to the sign-in page, send me their email address and the time it happened and I'll check the logs.",
    ].join("\n\n"),
  );
  stageDraft(ws, t, html, "Claude", { publish: false, at: ws.messages.filter((m) => m.ticket_id === t.id).at(-1)?.created_at });
}

export function registerDrafts() {
  for (const ws of db.workspaces) seedDrafts(ws);

  get("/v1/tickets/:id/draft", (ctx) => ({ draft: drafts(ctx.ws).get(ticketOf(ctx).id) ?? null }));

  put("/v1/tickets/:id/draft", (ctx) => {
    if ((ctx.ws.members.find((m) => m.user_id === db.meId)?.role ?? "viewer") === "viewer") throw forbidden("Viewers can't change tickets");
    const html = typeof ctx.body.body_html === "string" ? ctx.body.body_html : "";
    if (!text(html) && !html.includes("<img")) throw invalid("the draft is empty");
    return stageDraft(ctx.ws, ticketOf(ctx), html);
  });

  get("/v1/tickets/:id/reply-draft", (ctx) => ({ draft: ownDrafts(ctx.ws).get(ticketOf(ctx).id) ?? null }));

  put("/v1/tickets/:id/reply-draft", (ctx) => {
    if ((ctx.ws.members.find((m) => m.user_id === db.meId)?.role ?? "viewer") === "viewer") throw forbidden("Viewers can't change tickets");
    const t = ticketOf(ctx);
    const html = typeof ctx.body.body_html === "string" ? ctx.body.body_html : "";
    const ids = Array.isArray(ctx.body.attachment_ids) ? (ctx.body.attachment_ids as string[]) : [];
    if (!text(html) && !html.includes("<img") && ids.length === 0) {
      ownDrafts(ctx.ws).delete(t.id);
      return { draft: null };
    }
    const attachments: Attachment[] = ids.map((id) => ({ id, filename: "file", content_type: "application/octet-stream", size_bytes: 0, inline: false, url: "" }));
    const d: ReplyDraft = { ticket_id: t.id, body_html: html, attachments, updated_at: nowIso() };
    ownDrafts(ctx.ws).set(t.id, d);
    return { draft: d };
  });

  del("/v1/tickets/:id/reply-draft", (ctx) => {
    ownDrafts(ctx.ws).delete(ticketOf(ctx).id);
    return respond(204);
  });

  del("/v1/tickets/:id/draft", (ctx) => {
    if ((ctx.ws.members.find((m) => m.user_id === db.meId)?.role ?? "viewer") === "viewer") throw forbidden("Viewers can't change tickets");
    clearDraft(ctx.ws, ticketOf(ctx));
    return respond(204);
  });
}

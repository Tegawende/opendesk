// A new ticket being written: one draft per workspace, shared by the quick
// dialog and the full page, so opening the page carries everything written.
// Memory only, like the reply composer's drafts.

import { create } from "zustand";
import type { CreateTicketBody } from "@/lib/api/endpoints";
import type { Contact, FieldValues, Priority, Severity } from "@/lib/api/types";

export type NewTicketTo = { contact?: Contact; email?: string };

export type NewTicketDraft = {
  to: NewTicketTo | null;
  subject: string;
  body: string;
  kind: "reply" | "note";
  /** The address it goes from; set to the brand's first one that can send until someone picks. */
  mailbox?: string;
  priority: Priority;
  severity: Severity | null;
  /** undefined: whoever writes it; null: unassigned. */
  assignee?: string | null;
  brandId?: string;
  /** undefined follows the address's team; null: no team. */
  team?: string | null;
  tags: string[];
  fields: FieldValues;
  cc: string[];
  bcc: string[];
  showCc: boolean;
};

export const emptyNewTicket: NewTicketDraft = {
  to: null,
  subject: "",
  body: "",
  kind: "reply",
  priority: "normal",
  severity: null,
  tags: [],
  fields: {},
  cc: [],
  bcc: [],
  showCc: false,
};

const useDrafts = create<Record<string, NewTicketDraft>>(() => ({}));

export function useNewTicketDraft(wid: string): NewTicketDraft {
  return useDrafts((s) => s[wid]) ?? emptyNewTicket;
}

export const newTicketDrafts = {
  get: (wid: string): NewTicketDraft => useDrafts.getState()[wid] ?? emptyNewTicket,
  patch: (wid: string, p: Partial<NewTicketDraft>) => useDrafts.setState((s) => ({ [wid]: { ...(s[wid] ?? emptyNewTicket), ...p } })),
  clear: (wid: string) =>
    useDrafts.setState((s) => {
      const next = { ...s };
      delete next[wid];
      return next;
    }, true),
};

/** Something written that closing would lose (the address and defaults picked for it are not). */
export function isDirty(d: NewTicketDraft): boolean {
  return !!(d.to || d.subject || d.body || d.tags.length || Object.keys(d.fields).length || d.cc.length || d.bcc.length);
}

/** What is missing before it can be created, as an inbox `newTicket.*` key. */
export function newTicketProblem(d: NewTicketDraft, noSender: boolean): "needTo" | "needSubject" | "needBody" | "cannotSend" | null {
  if (!d.to) return "needTo";
  if (!d.subject.trim()) return "needSubject";
  if (!d.body) return "needBody";
  if (noSender) return "cannotSend";
  return null;
}

/**
 * The POST /v1/tickets body. What the dialog offers comes out as it always has; labels,
 * severity, fields, Cc and Bcc, and files go in the same request, only when given.
 */
export function createTicketBody(
  d: NewTicketDraft,
  o: { me: string; teamId: string | null; hasTeams: boolean; attachmentIds?: string[] },
): CreateTicketBody {
  const assignee = d.assignee === undefined ? o.me : d.assignee;
  const emailed = d.kind === "reply" && !!d.mailbox;
  const body: CreateTicketBody = {
    subject: d.subject.trim(),
    contact_id: d.to?.contact?.id,
    contact: !d.to?.contact && d.to?.email ? { email: d.to.email, name: "" } : undefined,
    // An email ticket needs its mailbox (later replies go out through it), also when it starts as a note.
    channel: d.mailbox ? "email" : "api",
    mailbox_id: d.mailbox || undefined,
    body_html: d.body,
    kind: d.kind,
    priority: d.priority,
    assignee_id: assignee ?? undefined,
    team_id: o.hasTeams && o.teamId ? o.teamId : undefined,
    status: d.kind === "reply" ? "pending" : "open",
  };
  if (d.severity) body.severity = d.severity;
  if (d.tags.length) body.tags = d.tags;
  if (Object.keys(d.fields).length) body.fields = d.fields;
  if (emailed && d.cc.length) body.cc = d.cc;
  if (emailed && d.bcc.length) body.bcc = d.bcc;
  if (o.attachmentIds?.length) body.attachment_ids = o.attachmentIds;
  return body;
}

import { describe, expect, it } from "vitest";
import type { Message } from "@/lib/api/types";
import { parsePrintSearch, printableMessages, printTitle } from "./ticketPrint";

let n = 0;
function msg(over: Partial<Message> = {}): Message {
  n++;
  return {
    id: `m${n}`,
    ticket_id: "t1",
    kind: "reply",
    direction: "inbound",
    author: { type: "contact", user: null, contact: { id: "c1", name: "Dana", email: "dana@example.com", avatar_url: null } },
    body_html: "<p>Hi</p>",
    body_text: "Hi",
    has_quoted: false,
    subject: "",
    from_email: "dana@example.com",
    from_name: "Dana",
    to: [],
    cc: [],
    bcc: [],
    attachments: [],
    delivery_status: "none",
    delivery_error: "",
    ai: false,
    thread_id: null,
    created_at: new Date(Date.UTC(2026, 9, 1, 9, n)).toISOString(),
    edited_at: null,
    ...over,
  };
}

const ids = (list: Message[]) => list.map((m) => m.id);

describe("printableMessages", () => {
  const customer = msg();
  const reply = msg({ direction: "outbound", author: { type: "user", user: { id: "u1", name: "Maya", email: "maya@acme.test", avatar_url: null }, contact: null } });
  const note = msg({ kind: "note", direction: "outbound" });
  const side = msg({ kind: "note", direction: "outbound", thread_id: "th1" });
  const sideAnswer = msg({ kind: "note", direction: "inbound", thread_id: "th1" });
  const deletedReply = msg({ direction: "outbound", deleted_at: "2026-10-02T00:00:00Z" });
  const deletedNote = msg({ kind: "note", direction: "outbound", deleted_at: "2026-10-02T00:00:00Z" });
  const held = msg({ direction: "outbound", delivery_status: "scheduled" });
  const sentAsReply = msg({ kind: "note", direction: "outbound", sent_as_reply_id: reply.id, deleted_at: "2026-10-02T00:00:00Z" });
  const sending = msg({ id: "tmp-1", direction: "outbound" });
  const all = [customer, reply, note, side, sideAnswer, deletedReply, deletedNote, held, sentAsReply, sending];

  it("prints the conversation with the customer, without notes or side conversations by default", () => {
    expect(ids(printableMessages(all, {}))).toEqual(ids([customer, reply, deletedReply]));
  });
  it("adds internal notes when asked, deleted ones as their marker", () => {
    expect(ids(printableMessages(all, { notes: true }))).toEqual(ids([customer, reply, note, deletedReply, deletedNote]));
  });
  it("adds side conversations when asked, both ways", () => {
    expect(ids(printableMessages(all, { side: true }))).toEqual(ids([customer, reply, side, sideAnswer, deletedReply]));
  });
  it("never prints a reply still held to be sent, a note sent as a reply, or one still sending", () => {
    const out = ids(printableMessages(all, { notes: true, side: true }));
    expect(out).not.toContain(held.id);
    expect(out).not.toContain(sentAsReply.id);
    expect(out).not.toContain(sending.id);
  });
  it("prints oldest first", () => {
    expect(ids(printableMessages([reply, customer], {}))).toEqual(ids([customer, reply]));
  });
  it("prints one message alone, whatever it is", () => {
    expect(ids(printableMessages(all, { message: note.id }))).toEqual([note.id]);
    expect(ids(printableMessages(all, { message: side.id }))).toEqual([side.id]);
    expect(printableMessages(all, { message: "gone" })).toEqual([]);
  });
});

describe("parsePrintSearch", () => {
  it("reads the options as flags and the message as an id", () => {
    expect(parsePrintSearch({ message: "m1", notes: "1", side: true })).toEqual({ message: "m1", notes: true, side: true });
    expect(parsePrintSearch({ notes: "0", side: "yes", message: "" })).toEqual({ message: undefined, notes: undefined, side: undefined });
  });
});

describe("printTitle", () => {
  it("names the page after the ticket", () => {
    expect(printTitle({ number: 1234, subject: "Refund for order" })).toBe("#1234 Refund for order");
    expect(printTitle({ number: 7, subject: "" })).toBe("#7");
  });
});

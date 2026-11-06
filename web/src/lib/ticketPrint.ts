// Print or save as PDF (the ticket print page, pages/inbox/TicketPrintPage.tsx): which of a
// ticket's messages go on the page, the page's search, and its title, which browsers use as
// the saved file's name.

import type { Message, Ticket } from "@/lib/api/types";

/** The print page's search: one message, or the conversation with internal notes and side conversations when asked. */
export type PrintSearch = { message?: string; notes?: boolean; side?: boolean };

const flag = (v: unknown): true | undefined => (v === true || v === "true" || v === "1" || v === 1 ? true : undefined);

export function parsePrintSearch(s: Record<string, unknown>): PrintSearch {
  return { message: typeof s.message === "string" && s.message ? s.message : undefined, notes: flag(s.notes), side: flag(s.side) };
}

/**
 * The messages a print shows, oldest first. One message prints alone, whatever it is. The
 * conversation is what the customer and the team said; internal notes and side conversations
 * join it only when asked for. A reply still held for its undo window has not been sent, and a
 * note sent as a reply is in that reply, so neither prints. A deleted message prints as its
 * marker, when its kind is included.
 */
export function printableMessages(messages: readonly Message[], search: PrintSearch): Message[] {
  if (search.message) return messages.filter((m) => m.id === search.message);
  return messages
    .filter((m) => {
      if (m.id.startsWith("tmp-") || m.delivery_status === "scheduled" || m.sent_as_reply_id) return false;
      if (m.thread_id) return !!search.side;
      if (m.kind === "note") return !!search.notes;
      return true;
    })
    .sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at));
}

/** "#1234 Subject": the page's title, so a saved PDF is named after the ticket. */
export function printTitle(t: Pick<Ticket, "number" | "subject">): string {
  return t.subject ? `#${t.number} ${t.subject}` : `#${t.number}`;
}

/**
 * Resolves once what the page shows has loaded: its fonts, its images and its email frames
 * with theirs, so the print has every image in it. Gives up waiting after `timeoutMs`; an image
 * that never answers prints as missing rather than holding the print back.
 */
export async function whenLoaded(root: HTMLElement, timeoutMs = 8000): Promise<void> {
  const settled = (async () => {
    await document.fonts?.ready;
    const frames = Array.from(root.querySelectorAll("iframe"));
    await Promise.all(frames.map(frameLoaded));
    const images = [
      ...Array.from(root.querySelectorAll("img")),
      ...frames.flatMap((f) => Array.from(f.contentDocument?.querySelectorAll("img") ?? [])),
    ];
    await Promise.all(images.map(imageLoaded));
    // The frames take their content's height on the next frames.
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
  })();
  await Promise.race([settled, new Promise((r) => setTimeout(r, timeoutMs))]);
}

function frameLoaded(f: HTMLIFrameElement): Promise<void> {
  // A new frame holds an empty about:blank document, already complete, until its own loads.
  const doc = f.contentDocument;
  if (doc && doc.readyState === "complete" && doc.URL !== "about:blank") return Promise.resolve();
  return new Promise((r) => f.addEventListener("load", () => r(), { once: true }));
}

function imageLoaded(img: HTMLImageElement): Promise<void> {
  if (!img.getAttribute("src") || img.complete) return Promise.resolve();
  return new Promise((r) => {
    img.addEventListener("load", () => r(), { once: true });
    img.addEventListener("error", () => r(), { once: true });
  });
}

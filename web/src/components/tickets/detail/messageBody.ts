import type { Message } from "@/lib/api/types";
import { isDesignedEmail, type Sanitized, sanitizeEmailHtml, sanitizeProse } from "@/lib/sanitize";

/** The name a message's header shows for its author (a bot's or the assistant's comes first, where it has one). */
export function authorName(m: Message): string {
  if (m.author.user) return m.author.user.name;
  if (m.author.contact) return m.author.contact.name || m.from_email;
  if (m.author.type === "system") return "opendesk";
  if (m.author.type === "api") return m.from_name || "API";
  return m.from_name || m.from_email || "Unknown";
}

/** What a message says as HTML: its stored HTML, or its text. */
export function messageHtml(m: Pick<Message, "body_html" | "body_text">): string {
  return m.body_html || `<p>${m.body_text}</p>`;
}

export type SanitizedBody = Sanitized & { designed: boolean; note: boolean };

/**
 * A message body, sanitized. An inbound email holds back images from other sites until
 * the reader loads them (remote); a designed one, received, sent or a note, renders in its
 * own frame (EmailFrame). A note's frame follows the app's theme: it is ours, not a sender's
 * layout to show as sent.
 */
export function sanitizeBody(
  html: string,
  o: { inbound: boolean; designable: boolean; remote: boolean; note?: boolean; eagerImages?: boolean },
): SanitizedBody {
  const note = !!o.note;
  const opts = { blockRemoteImages: o.inbound && !o.remote, eagerImages: o.eagerImages };
  if (o.designable && isDesignedEmail(html)) return { designed: true, note, ...sanitizeEmailHtml(html, opts) };
  return { designed: false, note, ...sanitizeProse(html, opts) };
}

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import "@/i18n";
import { i18n } from "@/i18n";
import type { MailMessage } from "@/lib/api/types";
import { DraftRow } from "./DraftRow";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(async () => {
  await i18n.changeLanguage("en");
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

function draft(drafted_by_ai?: string): MailMessage {
  return {
    id: "draft-1",
    thread_id: "thread-1",
    account_id: "account-1",
    message_id: "draft@example.test",
    from: { name: "Support", email: "support@example.test" },
    to: [],
    cc: [],
    bcc: [],
    reply_to: [],
    subject: "Order update",
    snippet: "We have an update",
    body_html: "<p>We have an update</p>",
    body_text: "We have an update",
    sent_at: new Date().toISOString(),
    unread: false,
    starred: false,
    draft: true,
    folder_ids: [],
    attachments: [],
    sent_by: null,
    drafted_by_ai,
    outgoing: true,
  };
}

describe("Collab DraftRow", () => {
  it("offers an AI draft while a person is composing and opens that draft", async () => {
    const onOpen = vi.fn();
    await act(async () => root.render(<DraftRow m={draft("Ollie") } composing onOpen={onOpen} />));

    expect(container.textContent).toContain("Use it");
    await act(async () => container.querySelector("button")?.click());
    expect(onOpen).toHaveBeenCalledOnce();
  });

  it("keeps the continue action outside composition and for a person's draft", async () => {
    await act(async () => root.render(<DraftRow m={draft("Ollie")} composing={false} onOpen={() => undefined} />));
    expect(container.textContent).toContain("Continue");

    await act(async () => root.render(<DraftRow m={draft()} composing onOpen={() => undefined} />));
    expect(container.textContent).toContain("Continue");
    expect(container.textContent).not.toContain("Use it");
  });
});

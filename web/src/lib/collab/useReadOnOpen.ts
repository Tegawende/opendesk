import { useEffect, useRef } from "react";
import type { MailThreadAction, MailThreadDetail } from "../api/types";

/** Read each delivery shown during this visit, without undoing an explicit mark as unread. */
export function useReadOnOpen({ detail, current, enabled, onAction }: {
  detail: (Pick<MailThreadDetail, "can_act"> & {
    thread: Pick<MailThreadDetail["thread"], "id" | "unread">;
    messages: Pick<MailThreadDetail["messages"][number], "id" | "draft">[];
  }) | undefined;
  current: boolean;
  enabled: boolean;
  onAction: (action: MailThreadAction, opts: { silent: boolean }) => void;
}) {
  const seen = useRef<{ threadId: string; messages: Set<string> } | null>(null);
  useEffect(() => {
    if (!detail || !current || !detail.can_act || !enabled) return;
    const messages = detail.messages.filter((m) => !m.draft);
    const previous = seen.current;
    const first = previous?.threadId !== detail.thread.id;
    const delivered = first || messages.some((m) => !previous.messages.has(m.id));
    seen.current = { threadId: detail.thread.id, messages: new Set(messages.map((m) => m.id)) };
    if (delivered && detail.thread.unread) onAction("read", { silent: true });
  }, [detail, current, enabled, onAction]);
}

// What of the customer's conversation goes along when a side conversation starts
// (contracts/api.md "Threads"): nothing unless picked in the send menu, and what is picked
// shows next to the uploads, where it can be taken out again. An email forwarded from its
// message menu shows whole under what is written, as it will go.

import { useQuery } from "@tanstack/react-query";
import { FileText, Forward, ImageOff, Quote, X } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { MenuCheckItem, MenuLabel } from "@/components/ui/menu";
import { Tooltip } from "@/components/ui/tooltip";
import type { Attachment, Message } from "@/lib/api/types";
import { api } from "@/lib/api/endpoints";
import { useAlwaysShowImages } from "@/lib/alwaysShowImages";
import { cn } from "@/lib/cn";
import { bytes, fullDate } from "@/lib/format";
import { q, qk } from "@/lib/queries";
import { sanitizeEmailHtml } from "@/lib/sanitize";
import { EmailFrame } from "./EmailFrame";

/** Messages the first email can quote: the server takes the last 10. */
const MAX_QUOTED = 10;

/** A public reply that reached the customer or came from them: what may go to a third party. */
export function inConversation(m: Message): boolean {
  return (
    m.kind === "reply" &&
    !m.thread_id &&
    !m.deleted_at &&
    !m.id.startsWith("tmp-") &&
    (m.direction === "inbound" ? !m.auth_failed : m.delivery_status === "sent" || m.delivery_status === "none")
  );
}

/** The customer's conversation. */
function conversationOf(messages: Message[] | undefined): Message[] {
  return (messages ?? []).filter(inConversation);
}

export type SideInclude = {
  /** How many messages the conversation would quote; 0 when there is none. */
  quoted: number;
  /** The files of the conversation, oldest first. */
  files: Attachment[];
  history: boolean;
  /** The email forwarded in full, which takes the conversation's place. */
  forward?: Message;
  /** The picked files, as attachment ids. */
  picked: string[];
  onChange: (p: { threadHistory?: boolean; threadFiles?: string[]; threadForward?: string }) => void;
};

/** What a new side conversation on the ticket could take along, and what is picked. */
export function useSideInclude(
  ticketId: string,
  history: boolean,
  picked: string[],
  onChange: SideInclude["onChange"],
  forwardId?: string,
): SideInclude {
  const { data: messages } = useQuery(q.messages(ticketId));
  const conversation = conversationOf(messages);
  const files: Attachment[] = [];
  for (const m of conversation) for (const a of m.attachments) if (!files.some((x) => x.id === a.id)) files.push(a);
  const forward = forwardId ? conversation.find((m) => m.id === forwardId) : undefined;
  return {
    quoted: Math.min(conversation.length, MAX_QUOTED),
    files,
    forward,
    history: history && conversation.length > 0 && !forward,
    picked: picked.filter((id) => files.some((a) => a.id === id)),
    onChange,
  };
}

/** The tooltip's words for what goes along, or "" for nothing. */
export function useIncludeSummary(inc: SideInclude | undefined): string {
  const { t } = useTranslation("inbox");
  if (!inc) return "";
  const n = inc.picked.length;
  if (inc.forward) return n ? t("side.withForwardAndFiles", { count: n }) : t("side.withForward");
  if (inc.history && n) return t("side.withBoth", { count: n });
  if (inc.history) return t("side.withConversation");
  if (n) return t("side.withFiles", { count: n });
  return "";
}

/** The send menu's "Include" section. Picking keeps the menu open. */
export function SideIncludeItems({ inc }: { inc: SideInclude }) {
  const { t } = useTranslation("inbox");
  const toggle = (id: string) => inc.onChange({ threadFiles: inc.picked.includes(id) ? inc.picked.filter((x) => x !== id) : [...inc.picked, id] });
  return (
    <>
      <MenuLabel>{t("side.include")}</MenuLabel>
      {/* A forwarded email takes the conversation's place. */}
      {inc.quoted > 0 && !inc.forward && (
        <MenuCheckItem
          checked={inc.history}
          onSelect={() => inc.onChange({ threadHistory: !inc.history })}
          icon={<Quote />}
          hint={t("side.messages", { count: inc.quoted })}
        >
          {t("side.conversation")}
        </MenuCheckItem>
      )}
      {inc.files.length > 1 && (
        <MenuCheckItem
          checked={inc.picked.length === inc.files.length}
          onSelect={() => inc.onChange({ threadFiles: inc.picked.length === inc.files.length ? [] : inc.files.map((a) => a.id) })}
          icon={<FileText />}
          hint={inc.files.length}
        >
          {t("side.allFiles")}
        </MenuCheckItem>
      )}
      <div className={cn("max-h-[184px] overflow-y-auto", inc.files.length > 1 && "pl-4")}>
        {inc.files.map((a) => (
          <MenuCheckItem key={a.id} checked={inc.picked.includes(a.id)} onSelect={() => toggle(a.id)} icon={<FileText />} hint={bytes(a.size_bytes)}>
            <span data-ph-mask>{a.filename}</span>
          </MenuCheckItem>
        ))}
      </div>
    </>
  );
}

/** What is picked, next to the uploads: each can be taken out again. */
export function SideIncludedChips({ inc }: { inc: SideInclude }) {
  const { t } = useTranslation("inbox");
  const files = inc.files.filter((a) => inc.picked.includes(a.id));
  return (
    <>
      {inc.history && (
        <Chip
          icon={<Quote className="size-3 text-fg-3" />}
          title={t("side.conversation")}
          meta={t("side.messages", { count: inc.quoted })}
          remove={t("side.remove", { name: t("side.conversation") })}
          onRemove={() => inc.onChange({ threadHistory: false })}
        />
      )}
      {files.map((a) => (
        <Chip
          key={a.id}
          icon={<FileText className="size-3 text-fg-3" />}
          title={a.filename}
          meta={t("side.fromCustomer", { size: bytes(a.size_bytes) })}
          remove={t("side.remove", { name: a.filename })}
          onRemove={() => inc.onChange({ threadFiles: inc.picked.filter((x) => x !== a.id) })}
          mask
        />
      ))}
    </>
  );
}

/**
 * The email a new side conversation forwards, under what is written and read-only: the header
 * the recipient gets, then the email whole, as it came in or went out. Its images from other
 * sites wait for the reader, as in the timeline.
 */
export function ForwardedEmail({ m, onRemove }: { m: Message; onRemove: () => void }) {
  const { t } = useTranslation("inbox");
  const always = useAlwaysShowImages();
  const [loaded, setLoaded] = useState(false);
  const { data, isPending } = useQuery({
    queryKey: qk.of("message-original", m.id),
    queryFn: () => api.messages.original(m.id),
    staleTime: Infinity,
    enabled: m.has_quoted,
  });
  const inbound = m.direction === "inbound";
  const html = (m.has_quoted ? data?.full_html : undefined) ?? m.body_html;
  const body = useMemo(() => sanitizeEmailHtml(html, { blockRemoteImages: inbound && !(always || loaded) }), [html, inbound, always, loaded]);
  const from = m.from_name && m.from_email ? `${m.from_name} <${m.from_email}>` : m.from_email || m.from_name;
  const rows: [string, string][] = [
    [t("side.forwardFrom"), from],
    [t("side.forwardDate"), fullDate(m.created_at)],
    [t("side.forwardSubject"), m.subject],
    [t("side.forwardTo"), m.to.join(", ")],
  ];
  return (
    <section aria-label={t("side.forwarded")} className="mx-3 mb-2.5 overflow-hidden rounded-md border border-line animate-pop-in motion-reduce:animate-none">
      <div className="flex items-start gap-2 border-b border-line-soft bg-sunken px-2.5 py-2">
        <Forward className="mt-0.5 size-3.5 shrink-0 text-fg-3" aria-hidden />
        <dl className="grid min-w-0 flex-1 grid-cols-[auto_minmax(0,1fr)] gap-x-2.5 text-[11.5px] leading-[18px]">
          {rows
            .filter(([, v]) => v)
            .map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="text-fg-4">{k}</dt>
                <dd className="truncate text-fg-2" data-ph-mask>
                  {v}
                </dd>
              </div>
            ))}
        </dl>
        <Tooltip content={t("side.forwardRemove")}>
          <button
            type="button"
            aria-label={t("side.forwardRemove")}
            onClick={onRemove}
            className="flex size-5 shrink-0 items-center justify-center rounded-sm text-fg-4 hover:bg-hover hover:text-fg"
          >
            <X className="size-3" />
          </button>
        </Tooltip>
      </div>
      {body.held > 0 && (
        <div className="flex items-center gap-2 border-b border-line-soft px-2.5 py-1 text-[11.5px] text-fg-3">
          <ImageOff className="size-3.5 shrink-0 text-fg-4" />
          <span className="min-w-0 flex-1">{t("message.imagesHeld")}</span>
          <Button size="xs" variant="secondary" className="shrink-0" onClick={() => setLoaded(true)}>
            {t("message.loadImages")}
          </Button>
        </div>
      )}
      <div className="max-h-[240px] overflow-y-auto" data-ph-mask>
        {m.has_quoted && isPending ? (
          <p className="px-2.5 py-2 text-xs text-fg-3">{t("message.loadingOriginal")}</p>
        ) : (
          <EmailFrame html={body.html} title={t("side.forwarded")} />
        )}
      </div>
    </section>
  );
}

function Chip({ icon, title, meta, remove, onRemove, mask }: { icon: ReactNode; title: string; meta: string; remove: string; onRemove: () => void; mask?: boolean }) {
  return (
    <span className="flex h-8 max-w-[240px] items-center gap-2 rounded-md border border-line bg-surface pl-1 pr-1 animate-pop-in motion-reduce:animate-none">
      <span className="flex size-6 shrink-0 items-center justify-center rounded-sm bg-hover">{icon}</span>
      <span className="min-w-0">
        <span className="block truncate text-[11.5px]" data-ph-mask={mask || undefined}>
          {title}
        </span>
        <span className="mono block truncate text-[10px] text-fg-4">{meta}</span>
      </span>
      <button type="button" aria-label={remove} onClick={onRemove} className="flex size-5 shrink-0 items-center justify-center rounded-sm text-fg-4 hover:bg-hover hover:text-fg">
        <X className="size-3" />
      </button>
    </span>
  );
}

import { useQuery } from "@tanstack/react-query";
import { getRouteApi, useCanGoBack, useNavigate, useRouter } from "@tanstack/react-router";
import { ImageOff, Lock, MessagesSquare, Paperclip, Printer, Trash2, X } from "lucide-react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useDocumentTitle } from "@/app/RootLayout";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { useAssistantName } from "@/components/tickets/ollie";
import { useDisplay } from "@/components/tickets/fields";
import { EmailFrame } from "@/components/tickets/detail/EmailFrame";
import { authorName, messageHtml, sanitizeBody, type SanitizedBody } from "@/components/tickets/detail/messageBody";
import { isApiError } from "@/lib/api/client";
import type { Message, Thread, Ticket } from "@/lib/api/types";
import { imagesLoadedFor, useAlwaysShowImages } from "@/lib/alwaysShowImages";
import { bytes, fullDate } from "@/lib/format";
import { q } from "@/lib/queries";
import { applyTheme } from "@/lib/theme";
import { printableMessages, printTitle, whenLoaded } from "@/lib/ticketPrint";

// Print or save as PDF: a ticket's conversation, or one message, as a plain light document
// that opens the browser's print dialog once it has loaded ("Save as PDF" is a destination
// there). Bodies go through the timeline's sanitizing and frame: an email's images from other
// sites stay held back unless the reader loaded them, or turns them on here for the print.

const route = getRouteApi("/tickets/$ticketId/print");

/** The page is light whatever the app's theme, on screen and on paper. */
function useLightPage() {
  useLayoutEffect(() => {
    const root = document.documentElement;
    root.classList.remove("dark");
    root.style.colorScheme = "light";
    return () => applyTheme();
  }, []);
}

export default function TicketPrintPage() {
  const { t } = useTranslation("inbox");
  const { ticketId } = route.useParams();
  const search = route.useSearch();
  const ticket = useQuery(q.ticket(ticketId));
  const id = ticket.data?.id ?? "";
  const messages = useQuery({ ...q.messages(id), enabled: !!id });
  const threads = useQuery({ ...q.threads(id), enabled: !!id });
  const tk = ticket.data;
  useLightPage();
  useDocumentTitle(tk ? printTitle(tk) : null, { bare: true });

  const always = useAlwaysShowImages();
  const [showRemote, setShowRemote] = useState(false);
  const list = useMemo(() => printableMessages(messages.data ?? [], search), [messages.data, search]);
  // An email whose images the reader loaded in the timeline prints with them.
  const bodies = useMemo(
    () => new Map(list.map((m) => [m.id, printBody(m, always || showRemote || imagesLoadedFor(m.id))])),
    [list, always, showRemote],
  );
  const held = [...bodies.values()].some((b) => b.held > 0);
  const threadById = useMemo(() => new Map((threads.data ?? []).map((th) => [th.id, th])), [threads.data]);

  // Print once, when everything on the page has loaded; later prints are the toolbar's.
  const doc = useRef<HTMLElement>(null);
  const printed = useRef(false);
  const [preparing, setPreparing] = useState(true);
  const ready = !!tk && messages.isSuccess;
  useEffect(() => {
    if (!ready || printed.current || !doc.current) return;
    let live = true;
    void whenLoaded(doc.current).then(() => {
      if (!live || printed.current) return;
      printed.current = true;
      setPreparing(false);
      window.print();
    });
    return () => {
      live = false;
    };
  }, [ready]);

  const router = useRouter();
  const navigate = useNavigate();
  const canGoBack = useCanGoBack();
  const close = () => (canGoBack ? router.history.back() : void navigate({ to: "/inbox/$folder/{-$ticketId}", params: { folder: "open", ticketId: id || ticketId } }));
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !e.defaultPrevented && !e.isComposing) close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  return (
    <div className="min-h-dvh bg-white text-fg [color-scheme:light] print:min-h-0">
      <div role="toolbar" aria-label={t("print.toolbar")} className="sticky top-0 z-10 flex h-12 items-center gap-2 border-b border-line bg-white/95 px-3 backdrop-blur print:hidden">
        <Button variant="ghost" size="sm" icon={<X className="size-4" />} onClick={close}>
          {t("print.close")}
        </Button>
        <span className="min-w-0 flex-1 truncate text-[12.5px] text-fg-3">{search.message ? t("print.oneMessage") : t("print.conversation")}</span>
        {(held || showRemote) && !always && (
          <label className="flex cursor-pointer items-center gap-2 text-[12.5px] text-fg-2" title={t("print.remoteImagesHint")}>
            <Switch checked={showRemote} onCheckedChange={setShowRemote} aria-label={t("print.remoteImages")} />
            <span className="max-sm:sr-only">{t("print.remoteImages")}</span>
          </label>
        )}
        <Button variant="primary" size="sm" icon={<Printer className="size-3.5" />} loading={preparing && !ticket.isError} disabled={!ready} onClick={() => window.print()}>
          {t("print.print")}
        </Button>
      </div>

      <main ref={doc} className="mx-auto w-full max-w-[672px] px-6 py-10 print:max-w-[672px] print:px-0 print:py-0">
        {ticket.isError ? (
          <EmptyState
            lit={3}
            title={isApiError(ticket.error, "not_found") ? t("detail.notFound") : t("detail.loadError")}
            description={isApiError(ticket.error, "not_found") ? t("detail.notFoundHint") : t("error.description")}
          />
        ) : !tk || messages.isPending ? (
          <div className="flex flex-col gap-3" aria-busy="true">
            <Skeleton className="h-3 w-16" />
            <Skeleton className="h-5 w-2/3" />
            <Skeleton className="mt-6 h-2.5 w-full" />
            <Skeleton className="h-2.5 w-4/5" />
          </div>
        ) : (
          <>
            <TicketHeader t={tk} />
            {list.length === 0 ? (
              <EmptyState compact lit={2} title={t("print.nothing")} description={t("print.nothingHint")} />
            ) : (
              <ol className="flex flex-col">
                {list.map((m) => (
                  <li key={m.id} className="border-t border-line py-5 first:border-t-0 first:pt-0">
                    {m.deleted_at ? <DeletedEntry m={m} /> : <PrintedMessage m={m} body={bodies.get(m.id)!} thread={m.thread_id ? threadById.get(m.thread_id) : undefined} />}
                  </li>
                ))}
              </ol>
            )}
          </>
        )}
      </main>
    </div>
  );
}

function printBody(m: Message, remote: boolean): SanitizedBody {
  const note = m.kind === "note" && !m.thread_id;
  const inbound = m.direction === "inbound" && !note;
  return sanitizeBody(messageHtml(m), { inbound, designable: true, remote, note, eagerImages: true });
}

function TicketHeader({ t }: { t: Ticket }) {
  const { t: tr } = useTranslation("inbox");
  const display = useDisplay();
  const requester = t.contact ? (t.contact.name && t.contact.email ? `${t.contact.name} <${t.contact.email}>` : t.contact.name || t.contact.email || "") : "";
  return (
    <header className="mb-8 border-b border-line pb-5 print-keep">
      <p className="mono text-[12px] text-fg-3">#{t.number}</p>
      <h1 className="mt-1 text-[20px] font-semibold leading-7 tracking-[-0.015em]">{t.subject || tr("row.noSubject")}</h1>
      <dl className="mt-4 grid grid-cols-[max-content_1fr] gap-x-6 gap-y-1 text-[12.5px]">
        {requester && (
          <>
            <dt className="text-fg-3">{tr("print.requester")}</dt>
            <dd className="min-w-0 break-words">{requester}</dd>
          </>
        )}
        <dt className="text-fg-3">{tr("print.status")}</dt>
        <dd>{display.status(t.status)}</dd>
        <dt className="text-fg-3">{tr("print.created")}</dt>
        <dd>{fullDate(t.created_at)}</dd>
      </dl>
    </header>
  );
}

function PrintedMessage({ m, body, thread }: { m: Message; body: SanitizedBody; thread?: Thread }) {
  const { t } = useTranslation("inbox");
  const ollie = useAssistantName();
  const bot = m.ai || m.author.type === "api" ? (m.author.bot ?? null) : null;
  const name = bot?.name ?? (m.ai ? ollie : authorName(m));
  const note = m.kind === "note" && !m.thread_id;
  // An email names the address it went through; a note, the member who wrote it.
  const email = (note ? m.author.user?.email : m.from_email) || m.from_email || m.author.user?.email || m.author.contact?.email || "";
  const files = m.attachments.filter((a) => !a.inline);
  return (
    <article aria-label={t(note ? "message.noteFrom" : m.thread_id ? "message.threadFrom" : "message.from", { name })}>
      <header className="print-keep mb-3 text-[12.5px]">
        <div className="flex items-baseline gap-3">
          <p className="min-w-0 flex-1 break-words">
            <span className="font-semibold text-fg">{name}</span>
            {email && email !== name && <span className="text-fg-3"> &lt;{email}&gt;</span>}
          </p>
          <time dateTime={m.created_at} className="shrink-0 text-[12px] text-fg-3">
            {fullDate(m.created_at)}
          </time>
        </div>
        {!note && m.to.length > 0 && (
          <p className="break-words text-fg-3">
            {t("print.to")} {m.to.join(", ")}
          </p>
        )}
        {!note && m.cc.length > 0 && (
          <p className="break-words text-fg-3">
            {t("print.cc")} {m.cc.join(", ")}
          </p>
        )}
        {note && (
          <p className="mt-1 flex items-center gap-1 font-medium text-fg-2">
            <Lock className="size-3" /> {t("message.internalNote")}
          </p>
        )}
        {m.thread_id && (
          <p className="mt-1 flex items-center gap-1 font-medium text-fg-2">
            <MessagesSquare className="size-3" /> {thread ? t("print.sideConversationNamed", { subject: thread.subject }) : t("print.sideConversation")}
          </p>
        )}
      </header>
      {body.designed ? (
        <EmailFrame html={body.html} title={t("message.emailContent")} themed={body.note} print />
      ) : (
        <div className="prose-od ph-no-capture text-[13px]" data-ph-mask dangerouslySetInnerHTML={{ __html: body.html }} />
      )}
      {body.held > 0 && (
        <p className="mt-2 flex items-center gap-1.5 text-[11.5px] text-fg-3">
          <ImageOff className="size-3" /> {t("print.imagesHidden")}
        </p>
      )}
      {files.length > 0 && (
        <div className="print-keep mt-3 text-[12px]">
          <p className="flex items-center gap-1.5 font-medium text-fg-2">
            <Paperclip className="size-3" /> {t("print.attachments")}
          </p>
          <ul className="mt-1 flex flex-col gap-0.5 pl-[18px] text-fg-2">
            {files.map((a) => (
              <li key={a.id} className="break-words">
                {a.filename} <span className="text-fg-4">({bytes(a.size_bytes)})</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </article>
  );
}

/** A deleted message prints as the timeline's marker: who deleted it and when. */
function DeletedEntry({ m }: { m: Message }) {
  const { t } = useTranslation("inbox");
  const ollie = useAssistantName();
  const name = m.ai ? ollie : authorName(m);
  const who = m.deleted_by?.name ?? t("message.someone");
  const own = !!m.deleted_by && m.deleted_by.id === m.author.user?.id;
  return (
    <p className="print-keep flex items-center gap-2 text-[12.5px] text-fg-3">
      <Trash2 className="size-3.5 shrink-0" />
      <span className="min-w-0 flex-1">
        {own
          ? t(m.kind === "note" ? "message.deletedOwnNote" : "message.deletedOwnMessage", { who })
          : t(m.kind === "note" ? "message.deletedNote" : "message.deletedMessage", { who, name })}
      </span>
      {m.deleted_at && <time dateTime={m.deleted_at}>{fullDate(m.deleted_at)}</time>}
    </p>
  );
}

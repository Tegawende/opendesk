import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, CircleDot, RefreshCw, Search, X } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Drawer } from "@/components/ui/dialog";
import { TextInput } from "@/components/ui/input";
import { Segmented } from "@/components/ui/tabs";
import { Tooltip } from "@/components/ui/tooltip";
import { EmptyState } from "@/components/ui/empty";
import { ListSkeleton } from "@/components/settings2/bits";
import { toast } from "@/components/ui/toast";
import { api } from "@/lib/api/endpoints";
import type { ApiKey, ApiKeyRequest, ApiKeyRequestDetail, ApiKeyRequestFilter } from "@/lib/api/types";
import { intlLocale } from "@/i18n";
import { fullDate, relShort } from "@/lib/format";
import { qk, upsertInList } from "@/lib/queries";
import { cn } from "@/lib/cn";
import { tone } from "@/lib/tones";

type StatusFilter = "all" | NonNullable<ApiKeyRequestFilter["status"]>;

/** The HTTP status in its tone: 2xx green, 3xx amber, 4xx and 5xx red. */
function CodePill({ code }: { code: number }) {
  const t = code < 300 ? tone.green : code < 400 ? tone.amber : tone.red;
  return <span className={cn("mono inline-flex h-5 items-center rounded-sm px-1.5 text-[11px] font-medium tabular-nums", t.chip)}>{code}</span>;
}

/**
 * When a request was made, short and in the past tense: "il y a 2 min", "vor 2 Min.", "2m". French's
 * narrow relative format writes "-2 min", which reads as a negative duration.
 */
export function ago(iso: string, now = Date.now()): string {
  const loc = intlLocale();
  if (loc.toLowerCase().startsWith("en")) return relShort(iso, now);
  const s = Math.round((now - Date.parse(iso)) / 1000);
  if (s >= 7 * 86400) return relShort(iso, now);
  const f = new Intl.RelativeTimeFormat(loc, { style: "short", numeric: "auto" });
  const m = Math.round(s / 60);
  const h = Math.round(m / 60);
  if (s < 45) return f.format(0, "second");
  if (m < 60) return f.format(-m, "minute");
  if (h < 24) return f.format(-h, "hour");
  return f.format(-Math.round(h / 24), "day");
}

/** A body as sent: JSON indented, anything else as it is. */
export function pretty(body: string): string {
  try {
    return JSON.stringify(JSON.parse(body), null, 2);
  } catch {
    return body;
  }
}

function TicketLink({ r }: { r: ApiKeyRequest }) {
  const { t } = useTranslation("settingsDev");
  if (r.ticket?.number != null)
    return (
      <a
        href={`/inbox/open/${r.ticket.id}`}
        target="_blank"
        rel="noopener noreferrer"
        onClick={(e) => e.stopPropagation()}
        className="mono text-[12px] font-medium text-tone-violet underline-offset-2 hover:underline"
        aria-label={t("apiKeys.requests.openTicket", { number: r.ticket.number })}
      >
        #{r.ticket.number}
      </a>
    );
  if (r.ticket_ref) return <span className="mono truncate text-[12px] text-fg-3">{r.ticket_ref}</span>;
  return <span className="text-fg-4">–</span>;
}

const grid = "grid grid-cols-[92px_minmax(0,1fr)_48px_72px] items-center gap-3 px-4";

/**
 * The requests an API key made while "Record requests" was on (GET /v1/api-keys/{id}/requests):
 * a list to filter by answer and ticket, and each request with what it sent and the answer.
 */
export function ApiKeyRequestsDrawer({
  apiKey,
  onClose,
  initial,
}: {
  apiKey: ApiKey;
  onClose: () => void;
  /** Opens already filtered, e.g. on the failures of the last 24 hours. */
  initial?: { status?: StatusFilter; since?: string };
}) {
  const { t } = useTranslation("settingsDev");
  const qc = useQueryClient();
  const [status, setStatus] = useState<StatusFilter>(initial?.status ?? "all");
  const [since, setSince] = useState(initial?.since ?? "");
  const [ticketInput, setTicketInput] = useState("");
  const [ticket, setTicket] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  const filter: ApiKeyRequestFilter = { ...(status !== "all" ? { status } : {}), ...(ticket ? { ticket } : {}), ...(since ? { since } : {}) };

  const list = useInfiniteQuery({
    queryKey: [...qk.apiKeys, "requests", apiKey.id, filter] as const,
    queryFn: ({ pageParam }) => api.apiKeys.requests(apiKey.id, { limit: 50, cursor: pageParam, ...filter }),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
  });
  const record = useMutation({
    mutationFn: () => api.apiKeys.update(apiKey.id, { record_requests: true }),
    onSuccess: (k) => upsertInList(qc, qk.apiKeys, k),
    onError: (e) => toast.error(e, t("apiKeys.requests.recordError")),
  });
  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const recording = record.data?.record_requests ?? apiKey.record_requests;

  return (
    <Drawer open onOpenChange={(o) => !o && onClose()} title={t("apiKeys.requests.title", { name: apiKey.name })} width={760}>
      {open ? (
        <RequestDetail apiKey={apiKey} requestId={open} onBack={() => setOpen(null)} />
      ) : (
        <div className="flex flex-col">
          <div className="flex flex-col gap-3 border-b border-line-soft px-4 py-3">
            <p className="text-[12.5px] text-fg-3">{t("apiKeys.requests.hint")}</p>
            {!recording && (
              <div className="flex flex-wrap items-center gap-2 rounded-md bg-tone-amber-soft px-3 py-2 text-[12px] text-tone-amber">
                <span className="flex-1">{t("apiKeys.requests.off")}</span>
                <Button size="xs" icon={<CircleDot />} loading={record.isPending} onClick={() => record.mutate()}>
                  {t("apiKeys.requests.turnOn")}
                </Button>
              </div>
            )}
            <div className="flex flex-wrap items-center gap-2">
              <Segmented
                size="xs"
                value={status}
                onChange={setStatus}
                options={[
                  { value: "all", label: t("apiKeys.requests.filterAll") },
                  { value: "errors", label: t("apiKeys.requests.filterErrors") },
                  { value: "2xx", label: "2xx" },
                  { value: "4xx", label: "4xx" },
                  { value: "5xx", label: "5xx" },
                ]}
              />
              {since && (
                <button
                  type="button"
                  onClick={() => setSince("")}
                  aria-label={t("apiKeys.requests.sinceClear")}
                  className="flex h-6 items-center gap-1 rounded-md border border-line px-2 text-[11.5px] text-fg-2 transition-colors hover:bg-hover"
                >
                  {t("apiKeys.requests.sinceLabel", { when: fullDate(since) })}
                  <X className="size-3 text-fg-4" />
                </button>
              )}
              <form
                className="flex min-w-[220px] flex-1 items-center gap-1"
                onSubmit={(e) => {
                  e.preventDefault();
                  setTicket(ticketInput.trim());
                }}
              >
                <TextInput
                  inputSize="sm"
                  value={ticketInput}
                  onChange={(e) => setTicketInput(e.target.value)}
                  onBlur={() => setTicket(ticketInput.trim())}
                  placeholder={t("apiKeys.requests.ticketPlaceholder")}
                  aria-label={t("apiKeys.requests.ticketLabel")}
                  className="mono"
                />
                <button
                  type="submit"
                  aria-label={t("apiKeys.requests.ticketLabel")}
                  className="flex size-7 shrink-0 items-center justify-center rounded-md text-fg-3 hover:bg-hover hover:text-fg"
                >
                  <Search className="size-3.5" />
                </button>
              </form>
              <Tooltip content={t("apiKeys.requests.refresh")}>
                <button
                  type="button"
                  aria-label={t("apiKeys.requests.refresh")}
                  onClick={() => void list.refetch()}
                  className="flex size-7 items-center justify-center rounded-md text-fg-3 transition-colors hover:bg-hover hover:text-fg"
                >
                  <RefreshCw className={cn("size-3.5", list.isFetching && "animate-spin")} />
                </button>
              </Tooltip>
            </div>
          </div>
          {list.isLoading ? (
            <ListSkeleton rows={6} />
          ) : list.isError ? (
            <EmptyState compact lit={0} className="py-10" title={t("apiKeys.requests.loadError")} />
          ) : !rows.length ? (
            <EmptyState
              compact
              lit={0}
              className="py-10"
              title={status !== "all" || ticket || since ? t("apiKeys.requests.emptyFiltered") : t("apiKeys.requests.empty")}
              description={status !== "all" || ticket || since ? undefined : t("apiKeys.requests.emptyHint")}
            />
          ) : (
            <div role="table" aria-label={t("apiKeys.requests.title", { name: apiKey.name })} className="text-[12.5px]">
              <div role="row" className={cn(grid, "h-8 border-b border-line-soft bg-sunken")}>
                <span role="columnheader" className="label-caps">{t("apiKeys.requests.col.when")}</span>
                <span role="columnheader" className="label-caps">{t("apiKeys.requests.col.request")}</span>
                <span role="columnheader" className="label-caps">{t("apiKeys.requests.col.status")}</span>
                <span role="columnheader" className="label-caps text-right">{t("apiKeys.requests.col.ticket")}</span>
              </div>
              {rows.map((r) => (
                <div
                  key={r.id}
                  role="row"
                  tabIndex={0}
                  onClick={() => setOpen(r.id)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") setOpen(r.id);
                  }}
                  className={cn(grid, "h-10 cursor-pointer border-b border-line-soft transition-colors last:border-0 hover:bg-sunken")}
                >
                  <span role="cell">
                    <Tooltip content={fullDate(r.created_at)}>
                      <span className="mono text-[11.5px] text-fg-3">{ago(r.created_at)}</span>
                    </Tooltip>
                  </span>
                  <span role="cell" className="mono flex min-w-0 items-center gap-2 text-[12px]">
                    <span className="w-12 shrink-0 font-medium text-fg-2">{r.method}</span>
                    <span className="truncate text-fg">
                      {r.path}
                      {r.query && <span className="text-fg-4">?{r.query}</span>}
                    </span>
                  </span>
                  <span role="cell">
                    <CodePill code={r.status} />
                  </span>
                  <span role="cell" className="flex justify-end">
                    <TicketLink r={r} />
                  </span>
                </div>
              ))}
              {list.hasNextPage && (
                <div className="flex justify-center py-3">
                  <Button size="sm" loading={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>
                    {t("apiKeys.requests.loadMore")}
                  </Button>
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </Drawer>
  );
}

const KB = (n: number) => `${Math.round(n / 1024)} KB`;

function RequestDetail({ apiKey, requestId, onBack }: { apiKey: ApiKey; requestId: string; onBack: () => void }) {
  const { t } = useTranslation("settingsDev");
  const { data: r, isLoading, isError } = useQuery({
    queryKey: [...qk.apiKeys, "requests", apiKey.id, "detail", requestId] as const,
    queryFn: () => api.apiKeys.request(apiKey.id, requestId),
  });
  const facts = (d: ApiKeyRequestDetail): [string, React.ReactNode][] =>
    [
      [t("apiKeys.requests.detail.when"), fullDate(d.created_at)],
      [t("apiKeys.requests.detail.duration"), `${d.duration_ms} ms`],
      [t("apiKeys.requests.detail.ticket"), d.ticket || d.ticket_ref ? <TicketLink r={d} /> : null],
      [t("apiKeys.requests.detail.message"), d.message_id && <span className="mono">{d.message_id}</span>],
      [t("apiKeys.requests.detail.contact"), d.contact_id && <span className="mono">{d.contact_id}</span>],
      [t("apiKeys.requests.detail.idempotency"), d.idempotency_key && <span className="mono">{d.idempotency_key}</span>],
      [t("apiKeys.requests.detail.replayed"), d.replayed ? t("apiKeys.requests.detail.replayedYes") : null],
      [t("apiKeys.requests.detail.requestId"), d.request_id && <span className="mono">{d.request_id}</span>],
      [t("apiKeys.requests.detail.ip"), d.ip && <span className="mono">{d.ip}</span>],
      [t("apiKeys.requests.detail.userAgent"), d.user_agent],
      [t("apiKeys.requests.detail.content"), d.content_type ? `${d.content_type}${d.request_size ? ` · ${KB(d.request_size)}` : ""}` : null],
    ].filter(([, v]) => v) as [string, React.ReactNode][];

  return (
    <div className="flex flex-col gap-4 px-4 py-3">
      <div>
        <Button size="xs" variant="ghost" icon={<ArrowLeft />} onClick={onBack}>
          {t("apiKeys.requests.back")}
        </Button>
      </div>
      {isLoading ? (
        <ListSkeleton rows={4} />
      ) : isError || !r ? (
        <EmptyState compact lit={0} className="py-10" title={t("apiKeys.requests.loadError")} />
      ) : (
        <>
          <div className="mono flex min-w-0 flex-wrap items-center gap-2 text-[13px]">
            <CodePill code={r.status} />
            <span className="font-medium">{r.method}</span>
            <span className="min-w-0 break-all">
              {r.path}
              {r.query && <span className="text-fg-4">?{r.query}</span>}
            </span>
          </div>
          <dl className="grid grid-cols-[140px_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-[12.5px]">
            {facts(r).map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="text-fg-3">{k}</dt>
                <dd className="min-w-0 break-words text-fg">{v}</dd>
              </div>
            ))}
          </dl>
          <section className="flex flex-col gap-1.5">
            <h3 className="label-caps">{t("apiKeys.requests.detail.sent")}</h3>
            {r.request_body != null ? (
              <>
                <pre className="mono max-h-[360px] overflow-auto rounded-md border border-line bg-sunken p-3 text-[11.5px] leading-relaxed" aria-label={t("apiKeys.requests.detail.sent")}>
                  {pretty(r.request_body)}
                </pre>
                {r.request_truncated && <p className="text-[12px] text-tone-amber">{t("apiKeys.requests.detail.truncated", { size: "64 KB" })}</p>}
              </>
            ) : (
              <p className="text-[12px] text-fg-4">{t("apiKeys.requests.detail.noBody")}</p>
            )}
          </section>
          {r.response_body != null && (
            <section className="flex flex-col gap-1.5">
              <h3 className="label-caps">{t("apiKeys.requests.detail.answer")}</h3>
              <pre className="mono max-h-[260px] overflow-auto rounded-md border border-line bg-sunken p-3 text-[11.5px] leading-relaxed" aria-label={t("apiKeys.requests.detail.answer")}>
                {pretty(r.response_body)}
              </pre>
              {r.response_truncated && <p className="text-[12px] text-tone-amber">{t("apiKeys.requests.detail.truncated", { size: "8 KB" })}</p>}
            </section>
          )}
          {r.status < 400 && <p className="text-[12px] text-fg-4">{t("apiKeys.requests.detail.answerNotKept")}</p>}
        </>
      )}
    </div>
  );
}

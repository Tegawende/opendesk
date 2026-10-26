// Ticket panels (contracts/integrations.md "Ticket panels"): beside the ticket, what Shopify
// or a URL of the workspace's own knows about its contact. Read-only and never in the way:
// each panel loads on its own, folds away, and a failure is one quiet line.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUpRight, ChevronRight, Globe, RefreshCw, Truck } from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { AppMark } from "@/components/appmark";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip } from "@/components/ui/tooltip";
import { api } from "@/lib/api/endpoints";
import type { PanelOrder, PanelSection, TicketPanel, TicketPanelData } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { money, shortDate } from "@/lib/format";
import { ease } from "@/lib/motion";
import { q, qk } from "@/lib/queries";

const CLOSED_KEY = "od-ticket-panels-closed";

function readClosed(): Set<string> {
  try {
    const v = JSON.parse(localStorage.getItem(CLOSED_KEY) ?? "[]");
    return new Set(Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : []);
  } catch {
    return new Set();
  }
}

function writeClosed(ids: Set<string>) {
  try {
    localStorage.setItem(CLOSED_KEY, JSON.stringify([...ids]));
  } catch {
    /* storage unavailable */
  }
}

/** The mark of a panel's source: Shopify's own, or the globe for a URL. */
export function PanelMark({ panel, size = "sm" }: { panel: Pick<TicketPanel, "kind" | "name">; size?: "sm" | "md" }) {
  return panel.kind === "shopify" ? (
    <AppMark app="shopify" name={panel.name} tint="green" size={size} />
  ) : (
    <AppMark name={panel.name} tint="blue" glyph={<Globe />} size={size} />
  );
}

/** Every enabled panel of the workspace, for the open ticket. */
export function TicketPanels({ ticketId }: { ticketId: string }) {
  const { data: panels } = useQuery(q.ticketPanels());
  const [closed, setClosed] = useState(readClosed);
  const on = (panels ?? []).filter((p) => p.enabled);
  if (!on.length) return null;
  const toggle = (id: string) =>
    setClosed((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      writeClosed(n);
      return n;
    });
  return (
    <>
      {on.map((p) => (
        <PanelSectionView key={p.id} ticketId={ticketId} panel={p} open={!closed.has(p.id)} onToggle={() => toggle(p.id)} />
      ))}
    </>
  );
}

function PanelSectionView({ ticketId, panel, open, onToggle }: { ticketId: string; panel: TicketPanel; open: boolean; onToggle: () => void }) {
  const { t } = useTranslation("inbox");
  const qc = useQueryClient();
  const reduce = useReducedMotion();
  const { data, isLoading } = useQuery({ ...q.ticketPanel(ticketId, panel.id), enabled: open });
  const [refreshing, setRefreshing] = useState(false);
  const refresh = async () => {
    setRefreshing(true);
    try {
      qc.setQueryData(qk.ticketPanel(ticketId, panel.id), await api.tickets.panel(ticketId, panel.id, true));
    } catch {
      /* the panel keeps what it showed */
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <section className="border-b border-line-soft" aria-label={panel.name}>
      <div className="group/panel flex items-center gap-2 px-4 py-3">
        <button
          type="button"
          onClick={onToggle}
          aria-expanded={open}
          aria-label={open ? t("panels.collapse", { name: panel.name }) : t("panels.expand", { name: panel.name })}
          className="flex min-w-0 flex-1 items-center gap-2 rounded-md text-left outline-none focus-visible:shadow-[0_0_0_2px_var(--focus)]"
        >
          <PanelMark panel={panel} />
          <h3 className="label-caps min-w-0 truncate">{panel.name}</h3>
          <ChevronRight className={cn("size-3.5 shrink-0 text-fg-4 transition-transform duration-150 motion-reduce:transition-none", open && "rotate-90")} />
        </button>
        {open && (
          <Tooltip content={t("panels.refresh", { name: panel.name })}>
            <button
              type="button"
              onClick={() => void refresh()}
              disabled={refreshing}
              aria-label={t("panels.refresh", { name: panel.name })}
              className="flex size-6 items-center justify-center rounded-md text-fg-4 opacity-0 transition-opacity hover:bg-hover hover:text-fg focus-visible:opacity-100 group-hover/panel:opacity-100 disabled:opacity-60"
            >
              <RefreshCw className={cn("size-3.5", refreshing && "animate-spin motion-reduce:animate-none")} />
            </button>
          </Tooltip>
        )}
      </div>
      <AnimatePresence initial={false}>
        {open && (
          <motion.div
            key="body"
            initial={reduce ? { opacity: 0 } : { height: 0, opacity: 0 }}
            animate={reduce ? { opacity: 1 } : { height: "auto", opacity: 1 }}
            exit={reduce ? { opacity: 0 } : { height: 0, opacity: 0 }}
            transition={{ duration: 0.2, ease: ease.out }}
            className="overflow-hidden"
          >
            <div className="px-4 pb-3.5" data-ph-mask>
              {isLoading || !data ? <PanelSkeleton /> : <PanelBody data={data} onRetry={() => void refresh()} />}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </section>
  );
}

function PanelSkeleton() {
  return (
    <div className="flex flex-col gap-2">
      <Skeleton className="h-3.5 w-2/3" />
      <Skeleton className="h-3 w-1/2" />
      <Skeleton className="h-10 w-full" />
    </div>
  );
}

function Quiet({ children }: { children: ReactNode }) {
  return <p className="text-[12px] text-fg-4">{children}</p>;
}

export function PanelBody({ data, onRetry }: { data: TicketPanelData; onRetry?: () => void }) {
  const { t } = useTranslation("inbox");
  switch (data.state) {
    case "no_contact":
      return <Quiet>{t("panels.noContact")}</Quiet>;
    case "empty":
      return <Quiet>{data.kind === "shopify" ? t("panels.emptyShopify") : t("panels.empty")}</Quiet>;
    case "error":
      return (
        <p className="text-[12px] text-fg-4">
          {t("panels.error", { message: data.message ?? "" })}{" "}
          {onRetry && (
            <button type="button" onClick={onRetry} className="text-fg-3 underline-offset-2 outline-none hover:text-fg hover:underline focus-visible:underline">
              {t("panels.retry")}
            </button>
          )}
        </p>
      );
  }
  return data.kind === "shopify" ? <Commerce data={data} /> : <Sections sections={data.sections ?? []} />;
}

/** "Partly refunded" for partially_refunded, the word Shopify uses otherwise. */
function useStatusWord() {
  const { t, i18n } = useTranslation("inbox");
  return (s: string) => {
    const key = `panels.status.${s}`;
    return i18n.exists(key, { ns: "inbox" }) ? t(key as "panels.status.paid") : s.replace(/_/g, " ");
  };
}

function statusTone(s: string): "ok" | "warn" | "danger" | "neutral" {
  if (s === "paid" || s === "fulfilled") return "ok";
  if (s === "refunded" || s === "voided" || s === "expired") return "danger";
  if (s === "pending" || s === "partially_paid" || s === "partially_refunded" || s === "partially_fulfilled" || s === "on_hold") return "warn";
  return "neutral";
}

function Commerce({ data }: { data: TicketPanelData }) {
  const { t } = useTranslation("inbox");
  const c = data.customer;
  const orders = data.orders ?? [];
  return (
    <div className="flex flex-col gap-2.5">
      {c && (
        <div className="flex flex-col gap-1">
          <a
            href={c.url}
            target="_blank"
            rel="noopener noreferrer"
            title={t("panels.openCustomer")}
            className="group inline-flex min-w-0 items-center gap-1 text-[12.5px] font-medium text-fg outline-none hover:underline focus-visible:underline"
          >
            <span className="truncate">{c.name}</span>
            <ArrowUpRight className="size-3 shrink-0 text-fg-4 group-hover:text-fg" />
          </a>
          <p className="text-[11.5px] text-fg-3">
            {t("panels.orders", { count: c.orders_count })} · {t("panels.spent", { amount: money(c.total_spent, c.currency) })}
          </p>
          {c.tags.length > 0 && (
            <div className="flex flex-wrap gap-1">
              {c.tags.slice(0, 6).map((tag) => (
                <Badge key={tag}>{tag}</Badge>
              ))}
            </div>
          )}
        </div>
      )}
      {orders.length ? (
        <ul className="flex flex-col gap-1.5">
          {orders.map((o) => (
            <OrderRow key={o.id} o={o} />
          ))}
        </ul>
      ) : (
        <Quiet>{t("panels.noOrders")}</Quiet>
      )}
    </div>
  );
}

function OrderRow({ o }: { o: PanelOrder }) {
  const { t } = useTranslation("inbox");
  const word = useStatusWord();
  return (
    <li className="rounded-md border border-line-soft px-2.5 py-2">
      <div className="flex items-baseline gap-2">
        <a
          href={o.url}
          target="_blank"
          rel="noopener noreferrer"
          title={t("panels.openOrder", { number: o.number })}
          className="mono text-[12px] font-medium text-fg outline-none hover:underline focus-visible:underline"
        >
          {o.number}
        </a>
        <span className="text-[11.5px] text-fg-4">{shortDate(o.created_at)}</span>
        <span className="ml-auto text-[12px] tabular-nums text-fg-2">{money(o.total, o.currency)}</span>
      </div>
      <div className="mt-1 flex flex-wrap gap-1">
        {o.cancelled ? (
          <Badge tone="danger">{t("panels.cancelled")}</Badge>
        ) : (
          <>
            {o.financial_status && <Badge tone={statusTone(o.financial_status)}>{word(o.financial_status)}</Badge>}
            {o.fulfillment_status && <Badge tone={statusTone(o.fulfillment_status)}>{word(o.fulfillment_status)}</Badge>}
          </>
        )}
      </div>
      {o.items.length > 0 && (
        <ul className="mt-1.5 flex flex-col gap-0.5 text-[11.5px] text-fg-3">
          {o.items.map((it, i) => (
            <li key={i} className="flex gap-1.5">
              <span className="tabular-nums text-fg-4">{it.quantity}×</span>
              <span className="min-w-0 flex-1 truncate">
                {it.title}
                {it.variant ? <span className="text-fg-4"> · {it.variant}</span> : null}
              </span>
            </li>
          ))}
        </ul>
      )}
      {o.tracking.length > 0 && (
        <ul className="mt-1.5 flex flex-col gap-0.5">
          {o.tracking.map((tr, i) => (
            <li key={i} className="flex items-center gap-1.5 text-[11.5px] text-fg-3">
              <Truck className="size-3 shrink-0 text-fg-4" />
              <span className="min-w-0 truncate">{[tr.company, tr.number].filter(Boolean).join(" · ")}</span>
              {tr.url && (
                <a href={tr.url} target="_blank" rel="noopener noreferrer" className="ml-auto shrink-0 text-fg-2 outline-none hover:underline focus-visible:underline">
                  {t("panels.track")}
                </a>
              )}
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}

function Sections({ sections }: { sections: PanelSection[] }) {
  return (
    <div className="flex flex-col gap-2.5">
      {sections.map((s, i) => (
        <div key={i} className="flex flex-col gap-1">
          {s.title && <h4 className="text-[11.5px] font-medium text-fg-2">{s.title}</h4>}
          <dl className="flex flex-col gap-0.5">
            {s.items.map((it, j) => (
              <div key={j} className="flex items-baseline gap-2 text-[12px]">
                <dt className="w-2/5 shrink-0 truncate text-fg-4">{it.label}</dt>
                <dd className="min-w-0 flex-1 break-words text-fg-2">
                  {it.url ? (
                    <a href={it.url} target="_blank" rel="noopener noreferrer" className="outline-none hover:underline focus-visible:underline">
                      {it.value || it.url}
                    </a>
                  ) : (
                    it.value
                  )}
                </dd>
              </div>
            ))}
          </dl>
        </div>
      ))}
    </div>
  );
}

// What an empty inbox list says: per folder, the page's art over its faded
// layout, what the folder is for, the next useful action and a few tips.

import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { format, startOfDay } from "date-fns";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { TicketCounts } from "@/lib/api/types";
import type { FolderId, InboxSearch, TeamTab } from "@/lib/folders";
import { q } from "@/lib/queries";
import { useSession } from "@/lib/session";
import { Button } from "@/components/ui/button";
import { EmptyArt, EmptyFact, EmptyState, GhostList } from "@/components/ui/empty";
import { AnimatedNumber } from "@/components/motion";
import { CheckBurst } from "@/components/tickets/CheckBurst";
import { useDisplay } from "@/components/tickets/fields";
import { shell } from "@/app/shellStore";

const ghost = <GhostList variant="tickets" />;
const link = "font-medium text-fg underline-offset-2 hover:underline";

// Inbox zero in Mine celebrates once per session.
let celebrated = false;

function MineArt() {
  const [burst] = useState(() => !celebrated);
  useEffect(() => {
    celebrated = true;
  }, []);
  return (
    <span className="relative inline-flex">
      <EmptyArt kind="mine" />
      {burst && <CheckBurst size={24} className="absolute -bottom-2 -right-2" />}
    </span>
  );
}

function useToday() {
  const today = format(startOfDay(new Date()), "yyyy-MM-dd");
  return { from: today, to: today };
}

export function InboxEmpty({
  folder,
  counts,
  search,
  onSearch,
  viewId,
  teamName,
  tab,
}: {
  folder: FolderId | "view" | "team";
  counts?: TicketCounts;
  search: InboxSearch;
  onSearch: (s: InboxSearch) => void;
  viewId?: string;
  /** A team inbox: its name (or "My teams") and tab. */
  teamName?: string;
  tab?: TeamTab;
}) {
  const { t } = useTranslation("inbox");
  const finish = useDisplay().finishWords;
  const navigate = useNavigate();
  const me = useSession((s) => s.user?.id);
  const toFolder = (f: FolderId) => void navigate({ to: "/inbox/$folder/{-$ticketId}", params: { folder: f, ticketId: undefined } });
  const filtered = !!(search.q || search.priority || search.severity || search.channel || search.tag || search.mailbox || search.to || search.team || search.brand || search.fields);
  const wantsReport = folder === "mine" || folder === "solved";
  const range = useToday();
  const month = { from: format(new Date(Date.now() - 29 * 86_400_000), "yyyy-MM-dd"), to: range.to };
  const report = useQuery({ ...q.reports(folder === "solved" ? month : range), enabled: wantsReport && !filtered, retry: false, staleTime: 60_000 });

  if (filtered)
    return (
      <EmptyState
        art="search"
        ghost={ghost}
        title={search.q ? t("empty.search.title", { q: search.q }) : t("empty.filtered.title")}
        description={t("empty.filtered.description")}
        action={<Button onClick={() => onSearch({ sort: search.sort, order: search.order, tab: search.tab })}>{t("empty.filtered.clear")}</Button>}
        secondary={
          search.q ? (
            <Button variant="ghost" onClick={() => shell.palette(true)}>
              {t("empty.search.all")}
            </Button>
          ) : undefined
        }
        tips={[
          { label: t("empty.search.spelling") },
          { kbd: "/", label: t("empty.search.focus") },
          { kbd: "mod k", label: t("empty.search.everywhere") },
        ]}
      />
    );

  switch (folder) {
    case "team":
      return (
        <EmptyState
          art={tab === "pending" ? "pending" : "inbox"}
          ghost={ghost}
          title={t(`empty.team.${tab ?? "unassigned"}`, { team: teamName ?? "" })}
          description={t("empty.team.description")}
          action={
            tab === "unassigned" || !tab ? (
              <Button onClick={() => onSearch({ ...search, tab: "open" })}>{t("empty.team.seeOpen")}</Button>
            ) : (
              <Button onClick={() => onSearch({ ...search, tab: undefined })}>{t("empty.team.seeUnassigned")}</Button>
            )
          }
          secondary={
            <Button variant="ghost" onClick={() => toFolder("unassigned")}>
              {t("empty.backToInbox")}
            </Button>
          }
          tips={[
            { kbd: "g t", label: t("empty.tips.switchTeam") },
            { kbd: "t", label: t("empty.tips.moveTeam") },
          ]}
        />
      );
    case "spam":
      return (
        <EmptyState
          art="spam"
          ghost={ghost}
          title={t("empty.spam.title")}
          description={t("empty.spam.how")}
          action={<Button onClick={() => toFolder("unassigned")}>{t("empty.goUnassigned")}</Button>}
          secondary={
            <Button variant="ghost" onClick={() => void navigate({ to: "/settings/mailboxes" })}>
              {t("empty.settings")}
            </Button>
          }
          tips={[
            { kbd: "⇧S", label: t("empty.spam.tipMark") },
            { label: t("empty.spam.tipBlocked") },
          ]}
        >
          <div className="flex flex-wrap items-center justify-center gap-2">
            <EmptyFact hue="green">
              {t("empty.spam.on")} ·{" "}
              <Link to="/settings/mailboxes" className={link}>
                {t("empty.settings")}
              </Link>
            </EmptyFact>
            <EmptyFact hue="gray">{t("empty.spam.caught", { count: counts?.spam ?? 0 })}</EmptyFact>
          </div>
        </EmptyState>
      );
    case "mine": {
      const waiting = counts?.unassigned ?? 0;
      const solvedToday = report.data?.by_agent?.find((a) => a.user.id === me)?.solved;
      return (
        <EmptyState
          icon={<MineArt />}
          ghost={ghost}
          title={t("empty.mine.title")}
          description={t("empty.mine.description")}
          action={
            waiting > 0 ? (
              <Button variant="primary" onClick={() => toFolder("unassigned")}>
                {t("empty.mine.pickUp")} <span className="tabular-nums opacity-70">({waiting})</span>
              </Button>
            ) : (
              <Button onClick={() => shell.newTicket()}>{t("empty.newTicket")}</Button>
            )
          }
          tips={[
            { kbd: "g i", label: t("empty.tips.unassigned") },
            { kbd: "i", label: t("empty.tips.assignMe") },
          ]}
        >
          {solvedToday ? (
            <EmptyFact hue="green">
              <AnimatedNumber value={solvedToday} className="font-medium text-fg" /> {t("empty.mine.solvedToday", { count: solvedToday })}
            </EmptyFact>
          ) : null}
        </EmptyState>
      );
    }
    case "solved": {
      const avg = report.data?.csat_avg;
      const n = report.data?.csat_count ?? 0;
      return (
        <EmptyState
          art="solved"
          ghost={ghost}
          title={t("empty.solved.title")}
          description={t("empty.solved.description")}
          action={<Button onClick={() => toFolder("open")}>{t("empty.goOpen")}</Button>}
          tips={[
            { kbd: "e", label: finish.tip() },
            { kbd: "mod shift enter", label: finish.sendTip() },
          ]}
        >
          {avg != null && n > 0 ? (
            <EmptyFact hue="amber">{t("empty.solved.csat", { avg: avg.toFixed(1), count: n })}</EmptyFact>
          ) : (
            <EmptyFact hue="gray">{t("empty.solved.csatHint")}</EmptyFact>
          )}
        </EmptyState>
      );
    }
    case "pending":
      return (
        <EmptyState
          art="pending"
          ghost={ghost}
          title={t("empty.pending.title")}
          description={t("empty.pending.description")}
          action={<Button onClick={() => toFolder("open")}>{t("empty.goOpen")}</Button>}
          secondary={
            <Button variant="ghost" onClick={() => toFolder("unassigned")}>
              {t("empty.goUnassigned")}
            </Button>
          }
          tips={[
            { kbd: "s", label: t("empty.tips.setPending") },
            { kbd: "h", label: t("empty.tips.snooze") },
          ]}
        />
      );
    case "view":
      return (
        <EmptyState
          art="views"
          ghost={ghost}
          title={t("empty.view.title")}
          description={t("empty.view.description")}
          action={
            <Button variant="primary" onClick={() => void navigate({ to: "/settings/views", search: viewId ? ({ edit: viewId } as never) : undefined })}>
              {t("empty.view.edit")}
            </Button>
          }
          secondary={
            <Button variant="ghost" onClick={() => toFolder("unassigned")}>
              {t("empty.backToInbox")}
            </Button>
          }
          tips={[{ kbd: "g i", label: t("empty.tips.unassigned") }]}
        />
      );
    default:
      return (
        <EmptyState
          art="inbox"
          ghost={ghost}
          title={t(`empty.${folder}.title`)}
          description={t(`empty.${folder}.description`)}
          action={
            <Button variant="primary" onClick={() => shell.newTicket()}>
              {t("empty.newTicket")}
            </Button>
          }
          secondary={
            <Button variant="ghost" onClick={() => void navigate({ to: "/settings/mailboxes" })}>
              {t("empty.connect")}
            </Button>
          }
          tips={[
            { kbd: "c", label: t("empty.tips.newTicket") },
            { kbd: "g m", label: t("empty.tips.mine") },
          ]}
        />
      );
  }
}

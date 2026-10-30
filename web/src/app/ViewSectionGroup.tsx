// A view section in the sidebar (lib/viewSections): a header row that folds its views and,
// folded, shows what they would. It sits under Views and, once pinned, under Favorites
// (contracts/collab.md SidebarPrefs.pinned, kind view_section); each place folds on its own,
// per member, in this browser. The rail shows every view, a hairline above the section.

import { useRouterState } from "@tanstack/react-router";
import { ChevronRight } from "lucide-react";
import { Fragment, useId, useState, type ReactElement, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { View } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { useSession } from "@/lib/session";
import { readFolded, writeFolded, type FoldPlace } from "@/lib/viewSections";
import { Count, Label, rowBase, rowIdle, rowShape, WaitingBadge } from "./sidebarParts";

/** The view open in the list pane, so a folded section that holds it reads as current. */
export function useOpenView(): string | undefined {
  return useRouterState({
    select: (s) => {
      const p = s.location.pathname.split("/").filter(Boolean);
      return p[0] === "views" ? p[1] : undefined;
    },
  });
}

/** The member's folded sections in one place, read again when the workspace or member changes. */
export function useFoldedSections(place: FoldPlace): [ReadonlySet<string>, (key: string) => void] {
  const wid = useSession((s) => s.workspaceId) ?? "";
  const uid = useSession((s) => s.user?.id) ?? "";
  const owner = `${wid}:${uid}`;
  const [folded, setFolded] = useState(() => readFolded(wid, uid, place));
  const [foldedFor, setFoldedFor] = useState(owner);
  if (foldedFor !== owner) {
    setFoldedFor(owner);
    setFolded(readFolded(wid, uid, place));
  }
  const toggle = (key: string) => {
    const next = new Set(folded);
    if (!next.delete(key)) next.add(key);
    setFolded(next);
    writeFolded(wid, uid, next, place);
  };
  return [folded, toggle];
}

export function ViewSectionGroup({
  name,
  views,
  rail,
  folded,
  onToggle,
  divider,
  openView,
  row,
  count,
  waiting,
  menu,
}: {
  name: string;
  views: View[];
  rail: boolean;
  folded: boolean;
  onToggle: () => void;
  /** The rail's hairline above it (not above the first entry of its list). */
  divider: boolean;
  /** The open view (useOpenView). */
  openView?: string;
  /** A view's row, indented under the header. */
  row: (v: View) => ReactNode;
  /** The number a view's row shows. */
  count: (v: View) => number | undefined;
  /** Visitors waiting in chat on a view's tickets. */
  waiting: (v: View) => number;
  /** Wraps the header in its menu (pin to Favorites). */
  menu?: (header: ReactElement) => ReactElement;
}) {
  const { t } = useTranslation("shell");
  const listId = useId();
  const shut = folded && !rail;
  const nums = views.map(count).filter((n): n is number => n !== undefined);
  const sum = nums.length ? nums.reduce((a, b) => a + b, 0) : undefined;
  const current = shut && views.some((v) => v.id === openView);
  const header = (
    <button
      type="button"
      aria-expanded={!shut}
      aria-controls={listId}
      onClick={onToggle}
      title={name}
      className={cn(rowBase, rowShape(rail), rowIdle, current && "font-medium text-fg")}
    >
      <ChevronRight className={cn("transition-transform duration-150 motion-reduce:transition-none", !shut && "rotate-90")} />
      <Label rail={rail}>{name}</Label>
      {current && <span className="sr-only">{t("nav.viewSectionCurrent")}</span>}
      {/* Folded away: what its views would show, here. */}
      {shut && <WaitingBadge n={views.reduce((a, v) => a + waiting(v), 0)} rail={rail} />}
      {shut && <Count n={sum} rail={rail} />}
    </button>
  );
  return (
    <div role="group" aria-label={t("nav.viewSection", { name })} className="flex flex-col gap-px">
      {rail ? divider && <div aria-hidden className="mx-4 my-1 h-px bg-chrome-line" /> : menu ? menu(header) : header}
      <div id={listId} className="flex flex-col gap-px">
        {!shut && views.map((v) => <Fragment key={v.id}>{row(v)}</Fragment>)}
      </div>
    </div>
  );
}

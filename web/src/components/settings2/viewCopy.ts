// Duplicating a view (Settings → Views, the sidebar's row menu): the copy the editor opens
// with, and the room made for it right after the original. The copy carries every field of
// the original but its id and place, so fields views gain later come along with it.

import type { TFunction } from "i18next";
import type { Team, View } from "@/lib/api/types";

/** A view before it is saved: everything but its id and place. */
export type ViewDraft = Omit<View, "id" | "position">;

/**
 * Which views the signed-in person may create or move (internal/helpdesk/resources/views.go):
 * admins any; everyone else their own, and a team's when they lead that team.
 */
export type ViewRights = { admin: boolean; leads: ReadonlySet<string> };

export function viewRights(admin: boolean, teams: Team[], userId: string): ViewRights {
  return { admin, leads: new Set(teams.filter((t) => t.members.some((m) => m.user_id === userId && m.role === "lead")).map((t) => t.id)) };
}

/** Whether the person may write a view with this audience. */
export function canWriteView(r: ViewRights, v: { shared: boolean; team_id?: string | null }): boolean {
  if (v.team_id) return r.admin || r.leads.has(v.team_id);
  return !v.shared || r.admin;
}

/** The copy of `source` the editor opens with: named `name`, and personal when the person may not create it for the same people. */
export function copyView(source: View, name: string, r: ViewRights): ViewDraft {
  const { id: _id, position: _position, ...rest } = structuredClone(source);
  const copy: ViewDraft = { ...rest, name };
  return canWriteView(r, copy) ? copy : { ...copy, shared: false, team_id: null };
}

/**
 * The views to move down so a copy at `position` sits right after `sourceId`: those after the
 * original that would share or come before its place, each one further down, as far as the
 * person may move them. `sorted` is in list order.
 */
export function makeRoom(sorted: View[], sourceId: string, position: number, canMove: (v: View) => boolean): { id: string; position: number }[] {
  const i = sorted.findIndex((v) => v.id === sourceId);
  if (i < 0) return [];
  const out: { id: string; position: number }[] = [];
  let last = position;
  for (const v of sorted.slice(i + 1)) {
    if (v.position > last) break;
    if (!canMove(v)) continue;
    last += 1;
    out.push({ id: v.id, position: last });
  }
  return out;
}

/** Why a copy is personal when its original is shared or a team's (shown in the editor). */
export function copyPersonalNote(t: TFunction<"settingsInbox">, source: View, teams: Team[]): string {
  if (!source.team_id) return t("views.copyPersonal.everyone");
  const team = teams.find((x) => x.id === source.team_id)?.name;
  return team ? t("views.copyPersonal.team", { team }) : t("views.copyPersonal.anyTeam");
}

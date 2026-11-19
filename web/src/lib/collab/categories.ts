// Inbox categories (contracts/collab.md "Categories"): the tabs above an inbox, worked
// out from the accounts in view, and the editor's helpers. Pure, so the tabs, the
// counts and the editor agree.

import type { MailAccount, MailCategory, MailCounts, MailThread } from "../api/types";
import { isTone, type Tone } from "../tones";

export const PRIMARY = "primary";
export const MAX_CATEGORIES = 10;
export const MIN_CATEGORIES = 2;

/** A tab: a category key with the name and colour of the set whose order the tabs follow. */
export type CategoryTab = MailCategory & { unread: number };

/**
 * The tabs of an inbox spanning these accounts: the union of their categories by key, in the
 * order of the set most of them share (Primary is every set's first) (the first account's on a tie), with its names and
 * colours. A category only other accounts have comes right after the one it follows there, so
 * a set that was reordered or extended keeps its order. None when no account is split.
 */
export function categoryTabs(accounts: MailAccount[], counts?: MailCounts): CategoryTab[] {
  const split = accounts.filter((a) => a.categories?.enabled && a.categories.categories.length > 1);
  if (!split.length) return [];
  const sets = new Map<string, { list: MailCategory[]; n: number }>();
  for (const a of split) {
    const id = a.categories.categories.map((c) => c.key).join("\n");
    const s = sets.get(id) ?? { list: a.categories.categories, n: 0 };
    s.n++;
    sets.set(id, s);
  }
  // Map keeps insertion order and the sort is stable: a tie goes to the set seen first.
  const ordered = [...sets.values()].sort((x, y) => y.n - x.n).map((s) => s.list);
  const tabs: CategoryTab[] = ordered[0].map((c) => ({ ...c, unread: 0 }));
  for (const list of ordered.slice(1)) {
    let at = 0;
    for (const c of list) {
      const i = tabs.findIndex((x) => x.key === c.key);
      if (i >= 0) at = i + 1;
      else tabs.splice(at++, 0, { ...c, unread: 0 });
    }
  }
  const byKey = new Map(tabs.map((x) => [x.key, x]));
  for (const a of split) for (const c of a.categories.categories) byKey.get(c.key)!.unread += counts?.categories?.[a.id]?.[c.key] ?? 0;
  // Accounts not split show all their inbox under Primary.
  const primary = byKey.get(PRIMARY);
  if (primary) for (const a of accounts) if (!split.includes(a)) primary.unread += counts?.accounts[a.id] ?? 0;
  return tabs;
}

/** A category's tone; an unknown colour reads as grey. */
export const categoryTone = (c: Pick<MailCategory, "color">): Tone => (isTone(c.color) ? c.color : "gray");

/** The categories a conversation of this account may move to. */
export function accountCategories(a: MailAccount | undefined): MailCategory[] {
  return a?.categories?.enabled ? a.categories.categories : [];
}

/** Whether conversations may move to a category, as a tab they are dropped on: there are some and each one's account has it. */
export function canMoveToCategory(threads: Pick<MailThread, "account_id">[], accounts: Map<string, MailAccount>, key: string): boolean {
  return threads.length > 0 && threads.every((x) => accountCategories(accounts.get(x.account_id)).some((c) => c.key === key));
}

/** A key from a name, as the server makes it. */
export function slugKey(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 36);
}

/** Keys for categories added in the editor (left empty, the server makes them). */
export function withKeys(list: MailCategory[]): MailCategory[] {
  const seen = new Set<string>();
  return list.map((c, i) => {
    let key = i === 0 ? PRIMARY : c.key || slugKey(c.name) || "category";
    if (i > 0 && key === PRIMARY) key = "category";
    for (let n = 2, base = key; seen.has(key); n++) key = `${base}-${n}`;
    seen.add(key);
    return { ...c, key };
  });
}

/** What is wrong with a set before it is saved, as a message key of collab categories.problem, or null. */
export function categoryProblem(list: MailCategory[]): "count" | "name" | "repeat" | null {
  if (list.length < MIN_CATEGORIES || list.length > MAX_CATEGORIES) return "count";
  const names = new Set<string>();
  for (const c of list) {
    const n = c.name.trim().toLowerCase();
    if (!n || n.length > 40) return "name";
    if (names.has(n)) return "repeat";
    names.add(n);
  }
  return null;
}

/** The sender a rule would name for a conversation: its latest sender that is not one of ours. */
export function ruleSender(participants: { email: string }[], own: Set<string>): string | null {
  const p = participants.find((x) => x.email && !own.has(x.email.toLowerCase()));
  return p ? p.email.toLowerCase() : null;
}

import { Eye, Pencil } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Tooltip } from "@/components/ui/tooltip";
import type { ApiKey } from "@/lib/api/types";
import { cn } from "@/lib/cn";

/** The resources a key's scopes reach, in the order the scope checklist lists them. */
export const RESOURCES = ["tickets", "contacts", "kb", "changelog", "wiki", "settings", "realtime", "collab"] as const;
export type Resource = (typeof RESOURCES)[number];
/** What a key may do on a resource: write carries read; realtime subscribes to live events; collab drafts replies as the key's creator. */
export type Level = "write" | "read" | "subscribe" | "draft";

/** A key's scopes as one entry per resource, at the highest level it has there. */
export function groupScopes(scopes: string[]): { resource: Resource; level: Level }[] {
  const set = new Set(scopes);
  const out: { resource: Resource; level: Level }[] = [];
  for (const r of RESOURCES) {
    if (r === "realtime") {
      if (set.has("realtime")) out.push({ resource: r, level: "subscribe" });
    } else if (r === "collab") {
      if (set.has("collab:draft")) out.push({ resource: r, level: "draft" });
    } else if (set.has(`${r}:write`)) out.push({ resource: r, level: "write" });
    else if (set.has(`${r}:read`)) out.push({ resource: r, level: "read" });
  }
  return out;
}

/** Chips shown before "+N". */
export const SHOWN = 3;

const chipBase = "inline-flex h-5 shrink-0 items-center gap-1 rounded-sm border border-line px-1.5 text-[11.5px] text-fg-2";

/**
 * One resource as a chip with only the levels the key has there: an eye for read, the pencil
 * beside it for write. Realtime and collab have one scope each and say it in a word. Its tooltip
 * says it in full ("Tickets : lecture et écriture").
 */
function Chip({ resource, level }: { resource: Resource; level: Level }) {
  const { t } = useTranslation("settingsDev");
  const name = t(`apiKeys.list.resource.${resource}`);
  const words = t("apiKeys.list.chip", { resource: name, access: t(`apiKeys.list.access.${level}`) });
  return (
    <Tooltip content={words}>
      <span aria-label={words} data-level={level} className={chipBase}>
        {name}
        {level === "subscribe" || level === "draft" ? (
          <span className="text-fg-4">· {t(`apiKeys.list.word.${level}`)}</span>
        ) : (
          <>
            <Eye aria-hidden className="size-3 text-tone-violet" />
            {level === "write" && <Pencil aria-hidden className="size-3 text-tone-amber" />}
          </>
        )}
      </span>
    </Tooltip>
  );
}

/**
 * A key's access, one chip per resource on one or two lines, three then "+N". Every scope reads
 * "Accès complet", every read scope "Lecture seule". With onEdit (admins), the whole cell is a
 * button that opens the key's "Modifier les autorisations" dialog, where every scope is listed;
 * without it, "+N" lists the rest on hover.
 */
export function ScopeSummary({
  scopes,
  full,
  readOnly,
  onEdit,
  name,
}: {
  scopes: string[];
  full: boolean;
  readOnly: boolean;
  onEdit?: () => void;
  /** The key's name, for the cell's button label. */
  name?: string;
}) {
  const { t } = useTranslation("settingsDev");
  const groups = groupScopes(scopes);
  const rest = groups.slice(SHOWN);
  const words = (g: { resource: Resource; level: Level }) =>
    t("apiKeys.list.chip", { resource: t(`apiKeys.list.resource.${g.resource}`), access: t(`apiKeys.list.access.${g.level}`) });
  const chips =
    full || readOnly ? (
      <span className={cn(chipBase, full && "border-tone-amber-line text-tone-amber")}>{t(full ? "apiKeys.list.full" : "apiKeys.list.readOnly")}</span>
    ) : (
      <>
        {groups.slice(0, SHOWN).map((g) => (
          <Chip key={g.resource} {...g} />
        ))}
        {rest.length > 0 && (
          <Tooltip
            content={
              onEdit ? (
                t("apiKeys.list.edit")
              ) : (
                <span className="flex flex-col gap-0.5">
                  {rest.map((g) => (
                    <span key={g.resource}>{words(g)}</span>
                  ))}
                </span>
              )
            }
          >
            <span data-more aria-label={rest.map(words).join(", ")} className={cn(chipBase, "text-fg-3", onEdit && "group-hover/rights:border-line-strong")}>
              {t("apiKeys.list.more", { count: rest.length })}
            </span>
          </Tooltip>
        )}
      </>
    );
  if (!onEdit)
    return (
      <div className="flex min-w-0 flex-wrap items-center gap-1">
        {chips}
      </div>
    );
  return (
    <button
      type="button"
      onClick={onEdit}
     
      aria-label={`${t("apiKeys.list.edit")}: ${name ?? ""}`}
      className="group/rights -mx-1.5 -my-1 flex min-w-0 cursor-pointer flex-wrap items-center gap-1 rounded-md px-1.5 py-1 text-left transition-colors hover:bg-hover focus-visible:outline-none focus-visible:shadow-[0_0_0_2px_var(--focus)]"
    >
      {chips}
    </button>
  );
}

/** What the Journal cell opens: the whole log, or its failures of the last 24 hours. */
export type LogView = { status?: "errors"; since?: string } | undefined;

/**
 * The Journal column: for a key that records its requests, "● Actif", which opens the request
 * log, and its failures of the last 24 hours, which open the log on those failures; a dash for a
 * key that does not record.
 */
export function LogCell({ apiKey, onOpen }: { apiKey: ApiKey; onOpen: (view: LogView) => void }) {
  const { t } = useTranslation("settingsDev");
  if (!apiKey.record_requests)
    return (
      <Tooltip content={t("apiKeys.list.logOff")}>
        <span className="text-[12px] text-fg-4" aria-label={t("apiKeys.list.logOff")}>
          —
        </span>
      </Tooltip>
    );
  const errors = apiKey.error_count_24h ?? 0;
  const label = t("apiKeys.list.errors", { count: errors });
  return (
    <div className="flex min-w-0 flex-col items-start">
      <Tooltip content={t("apiKeys.list.logOpen")}>
        <button
          type="button"
          onClick={() => onOpen(undefined)}
          aria-label={`${t("apiKeys.list.logOpen")}: ${apiKey.name}`}
          className="-mx-1 flex items-center gap-1.5 rounded-sm px-1 text-[12px] font-medium text-tone-green transition-colors hover:bg-hover"
        >
          <span className="size-1.5 shrink-0 rounded-full bg-tone-green-solid" />
          {t("apiKeys.list.logOn")}
        </button>
      </Tooltip>
      {errors > 0 ? (
        <Tooltip content={t("apiKeys.list.errorsOpen")}>
          <button
            type="button"
            data-errors={errors}
            onClick={() => onOpen({ status: "errors", since: new Date(Date.now() - 86400_000).toISOString() })}
            aria-label={`${t("apiKeys.list.errorsOpen")}: ${apiKey.name}`}
            className="-mx-1 rounded-sm px-1 text-left text-[11px] font-medium text-danger-fg underline-offset-2 transition-colors hover:bg-hover hover:underline"
          >
            {label}
          </button>
        </Tooltip>
      ) : (
        <span data-errors={0} className="text-[11px] text-fg-4">
          {label}
        </span>
      )}
    </div>
  );
}

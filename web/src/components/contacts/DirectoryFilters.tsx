// The filter and sort row of the People and Companies lists: a label, whether a person is
// blocked (People), and how the list is ordered. Each choice is a search parameter of the page,
// so a filtered list can be linked and survives a reload.

import { useQuery } from "@tanstack/react-query";
import { ArrowDownUp, Ban, Tag as TagIcon, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Menu, MenuChoice, MenuContent, MenuLabel, MenuTrigger } from "@/components/ui/menu";
import { Picker } from "@/components/ui/picker";
import type { CompanySort, ContactSort } from "@/lib/api/endpoints";
import type { LabelScope } from "@/lib/api/types";
import { COMPANY_SORTS, PEOPLE_SORTS } from "@/lib/directorySort";
import { q } from "@/lib/queries";

type Common = { tag?: string; onTag: (name: string | undefined) => void };
type Props =
  | (Common & { scope: "contacts"; sort?: ContactSort; onSort: (s: ContactSort | undefined) => void; blocked?: "true" | "false"; onBlocked: (b: "true" | "false" | undefined) => void })
  | (Common & { scope: "companies"; sort?: CompanySort; onSort: (s: CompanySort | undefined) => void });

const chip =
  "inline-flex h-7 items-center gap-1.5 rounded-md border px-2 text-[12.5px] outline-none transition-colors focus-visible:shadow-[0_0_0_3px_var(--focus)]";
const idle = "border-line bg-surface text-fg-2 hover:border-line-strong hover:text-fg";
const active = "border-line-strong bg-hover text-fg";

/** The row's controls. The default sort is not a filter, so choosing it clears the parameter. */
export function DirectoryFilters(props: Props) {
  const { t } = useTranslation("contacts");
  const { data: tags } = useQuery(q.tags());
  const scope: LabelScope = props.scope;
  const labels = (tags ?? []).filter((x) => !x.archived && x.applies_to.includes(scope));
  const sorts: (ContactSort | CompanySort)[] = props.scope === "contacts" ? PEOPLE_SORTS : COMPANY_SORTS;
  const fallback = sorts[0];
  const sort: ContactSort | CompanySort = props.sort ?? fallback;
  const blocked = props.scope === "contacts" ? props.blocked : undefined;

  const clear = (label: string, onClick: () => void) => (
    <button type="button" aria-label={label} onClick={onClick} className="-mr-0.5 flex size-4 items-center justify-center rounded-sm text-fg-4 hover:bg-active hover:text-fg">
      <X className="size-3" />
    </button>
  );

  return (
    <div className="flex min-h-10 flex-wrap items-center gap-2 border-b border-line-soft px-4 py-1.5 md:px-5">
      <Picker
        items={labels.map((l) => ({ value: l.name, label: l.name }))}
        value={props.tag ?? null}
        width={240}
        placeholder={t("filter.searchLabels")}
        emptyText={t("filter.noLabels")}
        onSelect={(name) => props.onTag(name === props.tag ? undefined : name)}
        trigger={
          <button type="button" className={`${chip} ${props.tag ? active : idle}`}>
            <TagIcon className="size-3.5 text-fg-3" aria-hidden />
            {props.tag ? t("filter.label", { name: props.tag }) : t("filter.labels")}
            {props.tag && clear(t("filter.clearLabel"), () => props.onTag(undefined))}
          </button>
        }
      />

      {props.scope === "contacts" && (
        <Menu>
          <MenuTrigger asChild>
            <button type="button" className={`${chip} ${blocked ? active : idle}`}>
              <Ban className="size-3.5 text-fg-3" aria-hidden />
              {blocked === "true" ? t("filter.blockedOnly") : blocked === "false" ? t("filter.notBlocked") : t("filter.blockedAny")}
            </button>
          </MenuTrigger>
          <MenuContent align="start">
            <MenuLabel>{t("filter.blockedTitle")}</MenuLabel>
            <MenuChoice checked={!blocked} onSelect={() => props.onBlocked(undefined)}>
              {t("filter.blockedAny")}
            </MenuChoice>
            <MenuChoice checked={blocked === "true"} onSelect={() => props.onBlocked("true")}>
              {t("filter.blockedOnly")}
            </MenuChoice>
            <MenuChoice checked={blocked === "false"} onSelect={() => props.onBlocked("false")}>
              {t("filter.notBlocked")}
            </MenuChoice>
          </MenuContent>
        </Menu>
      )}

      <div className="ml-auto">
        <Menu>
          <MenuTrigger asChild>
            <button type="button" className={`${chip} ${idle}`}>
              <ArrowDownUp className="size-3.5 text-fg-3" aria-hidden />
              {t("sort.label", { by: t(`sort.${sort}`) })}
            </button>
          </MenuTrigger>
          <MenuContent align="end">
            <MenuLabel>{t("sort.title")}</MenuLabel>
            {sorts.map((s) => (
              <MenuChoice
                key={s}
                checked={sort === s}
                onSelect={() => {
                  const next = s === fallback ? undefined : s;
                  if (props.scope === "contacts") props.onSort(next as ContactSort | undefined);
                  else props.onSort(next as CompanySort | undefined);
                }}
              >
                {t(`sort.${s}`)}
              </MenuChoice>
            ))}
          </MenuContent>
        </Menu>
      </div>
      {(props.tag || blocked) && (
        <Button
          size="xs"
          variant="ghost"
          onClick={() => {
            props.onTag(undefined);
            if (props.scope === "contacts") props.onBlocked(undefined);
          }}
        >
          {t("clearFilters")}
        </Button>
      )}
    </div>
  );
}

// Rename and recolour statuses and priorities for this workspace, and hide
// statuses from the agents' menus. Display only: the API values never change. Shapes follow `status_labels` and
// `priority_labels` in the workspace settings contract.

import { Eye, EyeOff, RotateCcw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { PriorityIcon, StatusIcon, priorityLabel, priorityOrder, statusLabel, statusOrder } from "@/components/indicators";
import { Popover } from "@/components/ui/popover";
import { TextInput } from "@/components/ui/input";
import { Tooltip } from "@/components/ui/tooltip";
import type { Priority, TicketStatus } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { priorityTone, statusTone, tone as tones, type Tone } from "@/lib/tones";
import { ColorSwatchPicker } from "./ColorSwatchPicker";

export type DisplayOverride = { label?: string; color?: Tone; hidden?: boolean };
export type Overrides<K extends string> = Partial<Record<K, DisplayOverride>>;

const labelRe = /^[^<>]{0,40}$/;

export function DisplayOverridesEditor({
  statuses,
  priorities,
  onStatuses,
  onPriorities,
  disabled,
}: {
  statuses: Overrides<TicketStatus>;
  priorities: Overrides<Priority>;
  onStatuses: (v: Overrides<TicketStatus>) => void;
  onPriorities: (v: Overrides<Priority>) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation("settings");
  useTranslation("indicators");
  return (
    <div className="grid gap-x-8 md:grid-cols-2">
      <OverrideList
        title={t("display.statuses")}
        keys={statusOrder}
        defaults={statusLabel}
        tones={statusTone}
        icon={(k) => <StatusIcon status={k} />}
        value={statuses}
        onChange={onStatuses}
        hideable={hideableStatus}
        disabled={disabled}
      />
      <OverrideList
        title={t("display.priorities")}
        keys={priorityOrder}
        defaults={priorityLabel}
        tones={priorityTone}
        icon={(k) => <PriorityIcon priority={k} />}
        value={priorities}
        onChange={onPriorities}
        disabled={disabled}
      />
    </div>
  );
}

function OverrideList<K extends string>({
  title,
  keys,
  defaults,
  tones: defaultTones,
  icon,
  value,
  onChange,
  disabled,
  hideable,
}: {
  title: string;
  keys: K[];
  defaults: (key: K) => string;
  tones: Record<K, Tone>;
  icon: (k: K) => React.ReactNode;
  value: Overrides<K>;
  onChange: (v: Overrides<K>) => void;
  disabled?: boolean;
  /** Whether a key may be hidden now (statuses only); "never" shows no toggle. */
  hideable?: (k: K, value: Overrides<K>) => boolean | "never";
}) {
  const { t } = useTranslation("settings");
  useTranslation("indicators");
  const set = (k: K, patch: DisplayOverride) => {
    const next: DisplayOverride = { ...value[k], ...patch };
    if (!next.label) delete next.label;
    if (!next.color || next.color === defaultTones[k]) delete next.color;
    if (!next.hidden) delete next.hidden;
    const out = { ...value };
    if (Object.keys(next).length) out[k] = next;
    else delete out[k];
    onChange(out);
  };
  return (
    <div className="py-3">
      <div className="label-caps mb-2">{title}</div>
      <div className="flex flex-col">
        {keys.map((k) => {
          const o = value[k] ?? {};
          const hue = o.color ?? defaultTones[k];
          const changed = !!(o.label || o.color || o.hidden);
          const canHide = hideable?.(k, value) ?? "never";
          const bad = !!o.label && !labelRe.test(o.label);
          return (
            <div key={k} className={cn("group flex h-10 items-center gap-2.5", o.hidden && "opacity-60")}>
              <span className="flex w-5 justify-center">{icon(k)}</span>
              <TextInput
                inputSize="sm"
                value={o.label ?? ""}
                placeholder={defaults(k)}
                maxLength={40}
                invalid={bad}
                disabled={disabled}
                aria-label={t("display.labelFor", { name: defaults(k) })}
                onChange={(e) => set(k, { label: e.target.value })}
                className="min-w-0 flex-1"
              />
              <Popover
                align="end"
                className="p-2.5"
                trigger={
                  <button
                    type="button"
                    disabled={disabled}
                    aria-label={t("display.colourFor", { name: defaults(k) })}
                    className="flex size-7 shrink-0 items-center justify-center rounded-md border border-line bg-surface transition-colors hover:border-line-strong disabled:opacity-50"
                  >
                    <span className={cn("size-3.5 rounded-xs", tones[hue].solid)} />
                  </button>
                }
              >
                <ColorSwatchPicker value={hue} onChange={(c) => set(k, { color: c })} size="sm" className="w-[124px]" label={t("display.colourFor", { name: defaults(k) })} />
              </Popover>
              {canHide !== "never" && (
                <Tooltip content={t(o.hidden ? "display.showFor" : "display.hideFor", { name: defaults(k) })}>
                  <button
                    type="button"
                    aria-label={t(o.hidden ? "display.showFor" : "display.hideFor", { name: defaults(k) })}
                    aria-pressed={!!o.hidden}
                    disabled={disabled || (!o.hidden && !canHide)}
                    onClick={() => set(k, { hidden: !o.hidden })}
                    className="flex size-7 shrink-0 items-center justify-center rounded-md text-fg-3 transition-colors hover:bg-hover hover:text-fg disabled:opacity-40"
                  >
                    {o.hidden ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                  </button>
                </Tooltip>
              )}
              <Tooltip content={t("display.reset")}>
                <button
                  type="button"
                  aria-label={t("display.resetFor", { name: defaults(k) })}
                  disabled={disabled || !changed}
                  onClick={() => {
                    const out = { ...value };
                    delete out[k];
                    onChange(out);
                  }}
                  className="flex size-7 shrink-0 items-center justify-center rounded-md text-fg-4 transition-[opacity,color] hover:bg-hover hover:text-fg disabled:pointer-events-none disabled:opacity-0"
                >
                  <RotateCcw className="size-3.5" />
                </button>
              </Tooltip>
            </div>
          );
        })}
      </div>
    </div>
  );
}

/** open always stays, and so does one way to finish a ticket (solved or closed): the API refuses the rest. */
export function hideableStatus(k: TicketStatus, _value: Overrides<TicketStatus>): boolean | "never" {
  if (k === "open" || k === "solved") return "never";
  return true;
}

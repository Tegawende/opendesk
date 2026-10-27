// Value pickers shared by the view filter builder and the automation builder.

import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ChevronDown, Inbox, Lock, MessageSquareReply, PencilLine, PhoneCall, Reply, SquarePlus } from "lucide-react";
import { forwardRef, type ReactNode } from "react";
import { Avatar } from "@/components/ui/avatar";
import { Picker, type PickerItem } from "@/components/ui/picker";
import { TagChip } from "@/components/ui/badge";
import {
  ChannelIcon,
  PriorityIcon,
  SentimentIcon,
  SeverityChip,
  StatusIcon,
  channelLabel,
  priorityLabel,
  priorityOrder,
  severityHint,
  severityLabel,
  severityOrder,
  statusLabel,
  statusOrder,
} from "@/components/tickets/bits";
import { i18n } from "@/i18n";
import type { AutomationTrigger, Channel } from "@/lib/api/types";
import { isTone, tone, toneFor } from "@/lib/tones";
import { sentimentLabel, sentiments, triggerLabel, triggerTone } from "./automation";
import { q } from "@/lib/queries";
import { cn } from "@/lib/cn";

export const statusItems = (): PickerItem[] => statusOrder.map((s) => ({
  value: s,
  label: statusLabel(s),
  icon: <StatusIcon status={s} />,
}));
export const priorityItems = (): PickerItem[] => priorityOrder.map((p) => ({
  value: p,
  label: priorityLabel(p),
  icon: <PriorityIcon priority={p} />,
}));
/** Severity levels, sev1 first, each with what it means. */
export const severityItems = (): PickerItem[] => severityOrder.map((s) => ({
  value: s,
  label: severityLabel(s),
  hint: severityHint(s),
  icon: <SeverityChip severity={s} />,
}));
/** Severity levels plus "no severity" ("none"), for conditions and filters. */
export const severityFilterItems = (): PickerItem[] => [
  ...severityItems(),
  { value: "none", label: i18n.t("settingsInbox:edit.noSeverity"), icon: <span className="h-[16px] w-[22px] rounded-[4px] border border-dashed border-line-strong" /> },
];
export const channelItems = (): PickerItem[] => (["email", "chat", "api", "portal", "phone"] as Channel[]).map((c) => ({
  value: c,
  label: channelLabel(c),
  icon: <ChannelIcon channel={c} />,
}));

export const sentimentItems = (): PickerItem[] =>
  sentiments.map((v) => ({
    value: v,
    label: sentimentLabel(v),
    icon: <SentimentIcon sentiment={v} />,
  }));

const triggerGlyph: Record<AutomationTrigger, ReactNode> = {
  ticket_created: <SquarePlus />,
  customer_replied: <MessageSquareReply />,
  agent_replied: <Reply />,
  ticket_updated: <PencilLine />,
  call_ended: <PhoneCall />,
};

/** The trigger's glyph in its hue. */
export function TriggerIcon({ trigger, className }: { trigger: AutomationTrigger; className?: string }) {
  return <span className={cn("flex shrink-0 [&_svg]:size-3.5", tone[triggerTone[trigger]].text, className)}>{triggerGlyph[trigger]}</span>;
}

export const triggerItems = () =>
  (Object.keys(triggerLabel) as AutomationTrigger[]).map((t) => ({
    value: t,
    label: triggerLabel[t],
    icon: <TriggerIcon trigger={t} />,
  }));

/** A tag's colour square, the same hue its chip uses. */
export function TagSwatch({ name, color }: { name: string; color?: string | null }) {
  const hue = isTone(color) ? color : toneFor(name);
  return <span aria-hidden className={cn("size-2 shrink-0 rounded-xs", tone[hue].solid)} />;
}

export function useMemberItems(): PickerItem[] {
  const { data } = useQuery(q.members());
  return (data ?? [])
    .filter((m) => m.role !== "viewer")
    .map((m) => ({
      value: m.user.id,
      label: m.user.name || m.user.email,
      hint: m.user.email,
      keywords: [m.user.email],
      icon: <Avatar name={m.user.name || m.user.email} src={m.user.avatar_url} size="xs" presence={m.presence} />,
    }));
}

export function useTeamItems(): PickerItem[] {
  const { t: tr } = useTranslation("inbox");
  const { data } = useQuery(q.teams());
  return (data ?? []).map((t) => ({
    value: t.id,
    label: t.name,
    icon: <span aria-hidden className={cn("size-2 shrink-0 rounded-[3px]", tone[isTone(t.color) ? t.color : toneFor(t.name)].solid)} />,
    hint: t.private ? <Lock className="size-3 text-fg-4" aria-label={tr("teams.private")} /> : undefined,
  }));
}

export function useMailboxItems(): PickerItem[] {
  const { data } = useQuery(q.mailboxes());
  return (data ?? []).map((m) => ({
    value: m.id,
    label: m.email,
    hint: m.name !== m.email ? m.name : undefined,
    icon: <Inbox />,
  }));
}

export function useTagItems(): PickerItem[] {
  const { data } = useQuery(q.tags());
  return (data ?? []).map((t) => ({
    value: t.name,
    label: t.name,
    icon: <TagSwatch name={t.name} color={t.color} />,
  }));
}

/**
 * Trigger heights: `sm` (28 px) for builder rows and dense lists, `md` (32 px) for a
 * form field beside text inputs and selects, which are 32 px.
 */
export type PickSize = "sm" | "md";
const pickHeights: Record<PickSize, string> = { sm: "h-7 text-[12.5px]", md: "h-8 px-2.5 text-[13px]" };

/** The trigger every builder picker uses: a quiet bordered button showing the current value. */
type ValueButtonProps = {
  children?: ReactNode;
  placeholder: string;
  icon?: ReactNode;
  className?: string;
  size?: PickSize;
  invalid?: boolean;
  disabled?: boolean;
} & Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "children">;

export const ValueButton = forwardRef<HTMLButtonElement, ValueButtonProps>(function ValueButton(
  { children, placeholder, icon, className, invalid, disabled, size = "sm", ...rest },
  ref,
) {
  const empty = children === undefined || children === null || children === "";
  return (
    <button
      ref={ref}
      type="button"
      disabled={disabled}
      aria-invalid={invalid || undefined}
      {...rest}
      className={cn(
        "inline-flex min-w-0 max-w-full items-center gap-1.5 rounded-md border border-line bg-surface px-2 text-fg outline-none transition-colors hover:border-line-strong focus-visible:shadow-[0_0_0_3px_var(--focus)] disabled:opacity-60 data-[state=open]:border-fg-3 aria-[invalid=true]:border-danger [&_svg]:size-3.5 [&_svg]:shrink-0",
        pickHeights[size],
        className,
      )}
    >
      {icon}
      <span className={cn("min-w-0 truncate", empty && "text-fg-4")}>{empty ? placeholder : children}</span>
      <ChevronDown className={cn("ml-auto", size === "md" ? "text-fg-3" : "!size-3 text-fg-4")} />
    </button>
  );
});

/** Single-value picker with a ValueButton trigger. */
export function SinglePick({
  items,
  value,
  onChange,
  placeholder,
  searchPlaceholder,
  onCreate,
  invalid,
  disabled,
  width,
  className,
  size,
  renderValue,
}: {
  items: PickerItem[];
  value: string | null | undefined;
  onChange: (v: string) => void;
  placeholder: string;
  searchPlaceholder?: string;
  onCreate?: (q: string) => void;
  invalid?: boolean;
  disabled?: boolean;
  width?: number;
  className?: string;
  size?: PickSize;
  /** Draw the chosen value in the trigger yourself (a tag chip, say); the icon is then left out. */
  renderValue?: (value: string) => ReactNode;
}) {
  useTranslation("settingsInbox");
  const cur = items.find((i) => i.value === value);
  const custom = value && renderValue ? renderValue(value) : null;
  const trigger = (
    <ValueButton placeholder={placeholder} icon={custom ? undefined : cur?.icon} invalid={invalid} disabled={disabled} className={className} size={size}>
      {custom ?? cur?.label ?? (value || undefined)}
    </ValueButton>
  );
  if (disabled) return trigger;
  return (
    <Picker
      items={items}
      value={value ?? null}
      onSelect={onChange}
      placeholder={searchPlaceholder ?? i18n.t("settingsInbox:pickers.search")}
      onCreate={onCreate}
      width={width}
      trigger={trigger}
    />
  );
}

/** Multi-value picker showing a compact summary of the selection. */
export function MultiPick({
  items,
  values,
  onChange,
  placeholder,
  disabled,
  className,
  size,
}: {
  items: PickerItem[];
  values: string[];
  onChange: (v: string[]) => void;
  placeholder: string;
  disabled?: boolean;
  className?: string;
  size?: PickSize;
}) {
  useTranslation("settingsInbox");
  const chosen = items.filter((i) => values.includes(i.value));
  const summary =
    chosen.length === 0
      ? undefined
      : chosen.length <= 2
        ? chosen.map((c) => c.label).join(", ")
        : i18n.t("settingsInbox:pickers.andMore", {
            first: chosen[0].label,
            count: chosen.length - 1,
          });
  const icons = (
    <span className="flex items-center gap-0.5">
      {chosen.slice(0, 3).map((c) => (
        <span key={c.value} className="flex">
          {c.icon}
        </span>
      ))}
    </span>
  );
  const trigger = (
    <ValueButton placeholder={placeholder} icon={chosen.length ? icons : undefined} disabled={disabled} className={className} size={size}>
      {summary}
    </ValueButton>
  );
  if (disabled) return trigger;
  return <Picker multi items={items} values={values} onChange={onChange} placeholder={i18n.t("settingsInbox:pickers.search")} trigger={trigger} />;
}

/** A tag picker whose trigger shows the tag as its coloured chip. */
export function TagPick(props: Omit<React.ComponentProps<typeof SinglePick>, "items" | "renderValue">) {
  const items = useTagItems();
  const { data } = useQuery(q.tags());
  return <SinglePick {...props} items={items} renderValue={(v) => <TagChip name={v} color={data?.find((t) => t.name === v)?.color} />} />;
}

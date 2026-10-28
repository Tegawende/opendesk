import { PencilLine } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { MailMessage } from "@/lib/api/types";

/** A conversation draft; an AI draft can be taken into an open composer. */
export function DraftRow({ m, composing, onOpen }: { m: MailMessage; composing: boolean; onOpen: () => void }) {
  const { t } = useTranslation("collab");
  const to = [...m.to, ...m.cc].map((a) => a.name || a.email).join(", ");
  return (
    <button
      type="button"
      onClick={onOpen}
      className="group flex w-full items-center gap-3 rounded-lg border border-dashed border-line-strong bg-surface px-3.5 py-2.5 text-left outline-none transition-colors hover:bg-sunken focus-visible:shadow-[0_0_0_2px_var(--focus)]"
    >
      <span className="inline-flex h-[18px] shrink-0 items-center rounded-sm bg-tone-red-soft px-1.5 text-[10.5px] font-medium text-tone-red">
        {m.drafted_by_ai ? t("thread.aiDraftBy", { name: m.drafted_by_ai }) : t("composer.draft")}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[12.5px] text-fg-2" data-ph-mask>
          {to ? t("thread.draftTo", { who: to }) : t("thread.draftNoRecipients")}
        </span>
        <span className="block truncate text-xs text-fg-4" data-ph-mask>
          {m.snippet || m.subject}
        </span>
      </span>
      <span className="flex shrink-0 items-center gap-1 text-[11.5px] text-fg-3 group-hover:text-fg">
        <PencilLine className="size-3.5" />
        {m.drafted_by_ai && composing ? t("thread.useDraft") : t("composer.editDraft")}
      </span>
    </button>
  );
}

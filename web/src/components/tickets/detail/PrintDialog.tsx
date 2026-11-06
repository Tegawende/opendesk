import { useNavigate, useRouter } from "@tanstack/react-router";
import { Printer } from "lucide-react";
import { useEffect, useId, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Switch } from "@/components/ui/switch";
import type { PrintSearch } from "@/lib/ticketPrint";

/** The ticket's "Print or save as PDF": what goes in with the conversation, then the print page. */
export function PrintDialog({ ticketId, open, onOpenChange }: { ticketId: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const { t } = useTranslation("inbox");
  const navigate = useNavigate();
  const router = useRouter();
  const [notes, setNotes] = useState(false);
  const [side, setSide] = useState(false);
  const search: PrintSearch = { notes: notes || undefined, side: side || undefined };
  const target = { to: "/tickets/$ticketId/print", params: { ticketId }, search } as const;

  useEffect(() => {
    if (open) void router.preloadRoute(target).catch(() => undefined);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, ticketId]);

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      size="sm"
      title={t("print.dialogTitle")}
      description={t("print.dialogDescription")}
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            variant="primary"
            icon={<Printer className="size-3.5" />}
            onClick={() => {
              onOpenChange(false);
              void navigate(target);
            }}
          >
            {t("print.open")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-1 pb-1">
        <Option checked={notes} onChange={setNotes} label={t("print.notes")} hint={t("print.notesHint")} />
        <Option checked={side} onChange={setSide} label={t("print.side")} hint={t("print.sideHint")} />
      </div>
    </Dialog>
  );
}

function Option({ checked, onChange, label, hint }: { checked: boolean; onChange: (v: boolean) => void; label: ReactNode; hint: ReactNode }) {
  const id = useId();
  return (
    <div className="flex items-start gap-3 rounded-md py-2">
      <div className="min-w-0 flex-1">
        <label htmlFor={id} className="block cursor-pointer text-[13px] font-medium text-fg">
          {label}
        </label>
        <p className="mt-0.5 text-[12px] leading-[17px] text-fg-3">{hint}</p>
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onChange} className="mt-0.5" />
    </div>
  );
}

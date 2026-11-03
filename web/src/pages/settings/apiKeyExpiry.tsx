import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, TextInput } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { ReauthNotice, isReauthError } from "@/components/settings/ReauthNotice";
import { api } from "@/lib/api/endpoints";
import { errorMessage } from "@/lib/api/client";
import type { ApiKey } from "@/lib/api/types";
import { fullDate } from "@/lib/format";
import { qk, upsertInList } from "@/lib/queries";

type Choice = "never" | "30" | "90" | "365" | "date";

const DAY = 86400_000;

/** YYYY-MM-DD of a moment, in the browser's time zone (what a date input shows). */
function localDay(ms: number): string {
  const d = new Date(ms);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** The expiry a choice means, as an ISO string, null for never, undefined when no valid date is picked. */
function chosenExpiry(choice: Choice, date: string, now: number): string | null | undefined {
  if (choice === "never") return null;
  if (choice === "date") {
    // The end of the picked day, where the admin is.
    const at = Date.parse(`${date}T23:59:00`);
    return Number.isFinite(at) && at > now ? new Date(at).toISOString() : undefined;
  }
  return new Date(now + Number(choice) * DAY).toISOString();
}

/** A later date, or none where there was one: the key works longer. */
export function expiryWidens(from: string | null, to: string | null): boolean {
  if (to === null) return from !== null;
  if (from === null) return false;
  return Date.parse(to) > Date.parse(from);
}

/**
 * Moves an existing key's expiry. An earlier date applies at once; a later one, or never, asks
 * for the same recent sign-in as making a key (the notice below explains a refusal and signs in
 * again). An expired key can't be brought back: it isn't offered.
 */
export function ExpiryDialog({ apiKey, onClose }: { apiKey: ApiKey; onClose: () => void }) {
  const { t } = useTranslation("settingsDev");
  const qc = useQueryClient();
  const [now] = useState(() => Date.now());
  const current = apiKey.expires_at ?? null;
  const [choice, setChoice] = useState<Choice>(current ? "date" : "never");
  const [date, setDate] = useState(current ? localDay(Date.parse(current)) : localDay(now + 90 * DAY));
  const [error, setError] = useState<string | null>(null);
  const next = chosenExpiry(choice, date, now);
  // The dialog opens on the key's own date: picking that same day again changes nothing.
  const sameDay = choice === "date" && current !== null && date === localDay(Date.parse(current));
  const unchanged = next === undefined || (next === null && current === null) || sameDay;
  const widens = !unchanged && next !== undefined && expiryWidens(current, next);

  const save = useMutation({
    mutationFn: () => api.apiKeys.update(apiKey.id, { expires_at: next ?? null }),
    onSuccess: (k) => {
      upsertInList(qc, qk.apiKeys, k);
      onClose();
    },
    onError: (e) => {
      if (!isReauthError(e)) setError(errorMessage(e));
    },
  });

  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("apiKeys.expiry.title", { name: apiKey.name })}
      description={current ? t("apiKeys.expiry.current", { date: fullDate(current) }) : t("apiKeys.expiry.currentNever")}
      footer={
        <>
          <span className="mr-auto text-xs text-danger-fg">{error}</span>
          <Button variant="ghost" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" loading={save.isPending} disabled={unchanged} onClick={() => (setError(null), save.mutate())}>
            {t("apiKeys.expiry.save")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3 pb-1">
        <Field label={t("apiKeys.form.expires")}>
          {(p) => (
            <Select<Choice>
              id={p.id}
              value={choice}
              onChange={setChoice}
              options={[
                { value: "never", label: t("apiKeys.form.never") },
                { value: "30", label: t("apiKeys.form.in30") },
                { value: "90", label: t("apiKeys.form.in90") },
                { value: "365", label: t("apiKeys.form.in365") },
                { value: "date", label: t("apiKeys.expiry.onDate") },
              ]}
            />
          )}
        </Field>
        {choice === "date" && (
          <Field label={t("apiKeys.expiry.date")} error={next === undefined ? t("apiKeys.expiry.future") : undefined}>
            {(p) => <TextInput {...p} type="date" min={localDay(now + DAY)} value={date} invalid={next === undefined} onChange={(e) => setDate(e.target.value)} />}
          </Field>
        )}
        {widens && <p className="text-[12px] text-fg-3">{t("apiKeys.expiry.widenNeedsSignIn")}</p>}
        {isReauthError(save.error) && <ReauthNotice error={save.error} />}
      </div>
    </Dialog>
  );
}

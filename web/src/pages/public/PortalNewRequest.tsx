// A signed-in customer starts a request from the portal: a subject and what they need. Priority,
// team and the rest are the team's to set when it arrives (contracts/customer-portal.md "Starting a request").

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Notice } from "@/components/auth/parts";
import { Button } from "@/components/ui/button";
import { TextInput, Textarea } from "@/components/ui/input";
import { isApiError } from "@/lib/api/client";
import { customerPortal } from "@/lib/customerPortal/client";

const publicTextLink = "text-fg-2 underline underline-offset-4 hover:text-fg focus-visible:rounded-sm focus-visible:shadow-[0_0_0_3px_var(--focus)]";

export function NewRequest({ slug }: { slug: string }) {
  const { t } = useTranslation("public");
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const create = useMutation({
    mutationFn: () => customerPortal.create(slug, subject.trim(), body.trim()),
    onSuccess: (ticket) => {
      void qc.invalidateQueries({ queryKey: ["customerPortal", slug] });
      void navigate({ to: "/portal/$slug/tickets/$ticketId", params: { slug, ticketId: ticket.id } });
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (subject.trim() && body.trim() && !create.isPending) create.mutate();
  };
  return (
    <div className="mx-auto w-full max-w-xl">
      <Link to="/portal/$slug" params={{ slug }} className={`mb-5 inline-block text-sm ${publicTextLink}`}>{t("portal.back_requests")}</Link>
      <h1 className="text-pretty text-2xl font-semibold">{t("portal.new.title")}</h1>
      <form className="mt-6 flex flex-col gap-4" onSubmit={submit}>
        {create.isError && <Notice>{isApiError(create.error, "rate_limited") ? t("portal.new.limit") : t("portal.error")}</Notice>}
        <label className="text-sm font-medium">
          {t("portal.new.subject")}
          <TextInput className="mt-1.5" name="subject" maxLength={200} required value={subject} onChange={(e) => setSubject(e.target.value)} />
        </label>
        <label className="text-sm font-medium">
          {t("portal.new.message")}
          <Textarea className="mt-1.5 min-h-40" name="body" maxLength={10000} required value={body} onChange={(e) => setBody(e.target.value)} />
        </label>
        <Button variant="primary" size="lg" type="submit" loading={create.isPending} disabled={!subject.trim() || !body.trim() || create.isPending}>
          {t("portal.new.send")}
        </Button>
      </form>
    </div>
  );
}

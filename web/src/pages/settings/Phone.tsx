// Phone (contracts/phone.md): connect a cloud phone system so every call lands on the
// caller's ticket, and choose how calls find their ticket.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { KeyRound, PhoneCall, PhoneMissed, RefreshCw, ScrollText, Trash2, Workflow } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { AppMark } from "@/components/appmark";
import { Row, Section, SettingsPage } from "@/components/layout";
import { type ItemState, ListSkeleton, StatePill } from "@/components/settings2/bits";
import { SettingsEmpty } from "@/components/settings2/SettingsEmpty";
import { Button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { CopyField } from "@/components/ui/copy-button";
import { Dialog } from "@/components/ui/dialog";
import { Field, TextInput } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { toast } from "@/components/ui/toast";
import { errorMessage } from "@/lib/api/client";
import { api } from "@/lib/api/endpoints";
import type { Automation, PhoneConnection, PhoneProvider } from "@/lib/api/types";
import { relShort } from "@/lib/format";
import { q, qk, removeFromList, upsertInList } from "@/lib/queries";
import { atLeast, useRole } from "@/lib/session";

const REUSE = [0, 1, 3, 7, 14, 30] as const;
const FIELDS = ["api_id", "api_token"] as const;
const knownField = (k: string): k is (typeof FIELDS)[number] => (FIELDS as readonly string[]).includes(k);

function connectionState(c: PhoneConnection): { state: ItemState; key: "healthy" | "action_required" | "error" | "unknown" } {
  if (c.status === "error" || c.health_status === "error") return { state: "failing", key: "error" };
  if (c.health_status === "healthy") return { state: "active", key: "healthy" };
  if (c.health_status === "action_required") return { state: "pending", key: "action_required" };
  return { state: "disabled", key: "unknown" };
}

/** The example from the docs (guides/aircall): a missed call outside opening hours reopens the ticket. */
export function missedCallAutomation(name: string, note: string): Omit<Automation, "id" | "run_count"> {
  return {
    name,
    trigger: "call_ended",
    conditions: [
      { field: "call_direction", op: "equals", value: "inbound" },
      { field: "call_outcome", op: "equals", value: "missed" },
      { field: "call_time", op: "equals", value: "outside_business_hours" },
    ],
    actions: [
      { type: "set_status", value: "open" },
      { type: "add_note", value: `<p>${note}</p>` },
      { type: "set_priority", value: "high" },
    ],
    enabled: false,
    position: 0,
  };
}

export default function Phone() {
  const { t } = useTranslation("settingsChannels");
  const role = useRole();
  const admin = atLeast(role, "admin");
  const qc = useQueryClient();
  const confirm = useConfirm();
  const navigate = useNavigate();
  const { data: providers } = useQuery(q.phoneProviders());
  const { data: connections, isLoading } = useQuery(q.phoneConnections());
  const [connecting, setConnecting] = useState<PhoneProvider | null>(null);
  const [rotating, setRotating] = useState<PhoneConnection | null>(null);

  const test = useMutation({
    mutationFn: api.phone.test,
    onSuccess: (row) => {
      upsertInList(qc, qk.phoneConnections, row);
      if (row.health_status === "healthy") toast.success(t("phone.health.tested"));
      else toast.info(t("phone.health.testedAttention"));
    },
    onError: (e) => toast.error(e, t("phone.health.testError")),
  });
  const update = useMutation({
    mutationFn: ({ id, ...b }: { id: string; reuse_days?: number; assign_answered?: boolean; insight_cards?: boolean }) => api.phone.update(id, b),
    onSuccess: (row) => upsertInList(qc, qk.phoneConnections, row),
    onError: (e) => toast.error(e, t("phone.saveError")),
  });
  const disconnect = useMutation({
    mutationFn: api.phone.disconnect,
    onSuccess: (_, id) => {
      removeFromList(qc, qk.phoneConnections, id);
      toast.success(t("phone.disconnected"));
    },
    onError: (e) => toast.error(e, t("phone.disconnectError")),
  });
  const example = useMutation({
    mutationFn: () => api.automations.create(missedCallAutomation(t("phone.example.name"), t("phone.example.note"))),
    onSuccess: (a) => {
      upsertInList(qc, qk.automations, a);
      toast.success(t("phone.example.created"));
      void navigate({ to: "/settings/automations/$automationId", params: { automationId: a.id } });
    },
    onError: (e) => toast.error(e, t("phone.example.error")),
  });

  const connected = new Set((connections ?? []).map((c) => c.provider));
  return (
    <SettingsPage wide title={t("phone.title")} description={t("phone.description")}>
      <Section flush title={t("phone.connections.title")} description={t("phone.connections.description")}>
        {isLoading ? (
          <ListSkeleton rows={2} />
        ) : !connections?.length ? (
          <SettingsEmpty
            art="settings"
            title={t("phone.empty.title")}
            description={t("phone.empty.body")}
            tips={[
              { label: t("phone.empty.tipTimeline"), icon: <PhoneCall /> },
              { label: t("phone.empty.tipMissed"), icon: <PhoneMissed /> },
              { label: t("phone.empty.tipAutomations"), icon: <Workflow /> },
            ]}
            className="min-h-[260px]"
          />
        ) : (
          <div className="divide-y divide-line-soft">
            {connections.map((c) => {
              const provider = providers?.find((p) => p.key === c.provider);
              const state = connectionState(c);
              const detail = c.health_code ? t(`phone.healthCode.${c.health_code}`) : c.health_detail;
              return (
                <div key={c.id} className="px-4 py-3.5" data-testid="phone-connection">
                  <div className="flex items-start gap-3">
                    <AppMark app={c.provider} name={provider?.name ?? c.provider} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-[13px] font-medium">{c.name}</span>
                        <span className="text-[12px] text-fg-3">{provider?.name ?? c.provider}</span>
                        <StatePill state={state.state}>{t(`phone.health.${state.key}`)}</StatePill>
                      </div>
                      {detail && <p className={state.state === "failing" ? "mt-1 text-[12px] text-danger-fg" : "mt-1 text-[12px] text-fg-2"}>{detail}</p>}
                      <p className="mt-0.5 text-[12px] text-fg-3">
                        {c.last_event_at ? t("phone.lastEvent", { when: relShort(c.last_event_at) }) : t("phone.noEventYet")}
                      </p>
                      {admin && (
                        <div className="mt-2 max-w-[560px]">
                          <p className="mb-1 text-[11.5px] text-fg-4">{t("phone.webhook")}</p>
                          <CopyField value={c.webhook_url} />
                        </div>
                      )}
                    </div>
                    {admin && (
                      <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
                        <Button icon={<RefreshCw />} loading={test.isPending && test.variables === c.id} onClick={() => test.mutate(c.id)}>
                          {t("phone.health.test")}
                        </Button>
                        <Button square icon={<KeyRound />} aria-label={t("phone.rotate.label", { name: c.name })} onClick={() => setRotating(c)} />
                        <Button
                          variant="danger"
                          square
                          icon={<Trash2 />}
                          aria-label={t("phone.disconnectLabel", { name: c.name })}
                          onClick={() =>
                            confirm.show(t("phone.disconnectBody", { name: c.name }), () => disconnect.mutateAsync(c.id), {
                              title: t("phone.disconnectTitle"),
                              confirmLabel: t("phone.disconnect"),
                              danger: true,
                            })
                          }
                        />
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Section>

      {connections?.map((c) => (
        <Section key={c.id} title={t("phone.placement.title", { name: c.name })} description={t("phone.placement.description")}>
          <Row label={t("phone.placement.reuse")} description={c.reuse_days ? t("phone.placement.reuseHint", { count: c.reuse_days }) : t("phone.placement.reuseNever")}>
            <Select
              size="sm"
              className="w-[180px]"
              aria-label={t("phone.placement.reuse")}
              disabled={!admin}
              value={String(c.reuse_days)}
              onChange={(v) => update.mutate({ id: c.id, reuse_days: Number(v) })}
              options={REUSE.map((n) => ({ value: String(n), label: n === 0 ? t("phone.placement.never") : n === 1 ? t("phone.placement.day") : t("phone.placement.daysN", { n }) }))}
            />
          </Row>
          <Row label={t("phone.placement.assign")} description={t("phone.placement.assignHint")}>
            <Switch
              aria-label={t("phone.placement.assign")}
              checked={c.assign_answered}
              disabled={!admin}
              onCheckedChange={(v) => update.mutate({ id: c.id, assign_answered: v })}
            />
          </Row>
          <Row label={t("phone.placement.cards")} description={t("phone.placement.cardsHint")}>
            <Switch
              aria-label={t("phone.placement.cards")}
              checked={c.insight_cards}
              disabled={!admin}
              onCheckedChange={(v) => update.mutate({ id: c.id, insight_cards: v })}
            />
          </Row>
          <Row label={t("phone.placement.dial")} description={t("phone.placement.dialHint")} />
        </Section>
      ))}

      {!!connections?.length && (
        <Section title={t("phone.automations.title")} description={t("phone.automations.description")}>
          <Row label={t("phone.example.name")} description={t("phone.example.hint")}>
            {admin && (
              <Button icon={<Workflow />} loading={example.isPending} onClick={() => example.mutate()}>
                {t("phone.example.create")}
              </Button>
            )}
          </Row>
          <Row label={t("phone.automations.docs")} description={t("phone.automations.docsHint")}>
            <a
              href="https://docs.opendesk.sh/guides/aircall/"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex h-7 items-center gap-1.5 rounded-md border border-line px-2.5 text-[12px] font-medium text-fg-2 hover:border-line-strong hover:text-fg"
            >
              <ScrollText className="size-3.5" aria-hidden />
              {t("phone.automations.openDocs")}
            </a>
          </Row>
        </Section>
      )}

      {(providers ?? []).some((p) => !connected.has(p.key)) && (
        <Section title={t("phone.providers.title")} description={admin ? t("phone.providers.descriptionAdmin") : t("phone.providers.description")}>
          <div className="grid gap-3 py-3 sm:grid-cols-3">
            {(providers ?? [])
              .filter((p) => !connected.has(p.key))
              .map((p) => (
                <button
                  key={p.key}
                  type="button"
                  disabled={!admin}
                  onClick={() => setConnecting(p)}
                  className="rounded-lg border border-line bg-surface p-3 text-left transition-colors hover:border-line-strong hover:bg-hover disabled:cursor-default disabled:opacity-60"
                >
                  <div className="flex items-center gap-2.5">
                    <AppMark app={p.key} name={p.name} size="sm" />
                    <span className="flex-1 text-[13px] font-medium">{p.name}</span>
                  </div>
                  <p className="mt-1.5 text-[12px] leading-[18px] text-fg-3">{p.key === "aircall" ? t("phone.provider.aircall") : p.name}</p>
                </button>
              ))}
          </div>
        </Section>
      )}

      {connecting && <CredentialsDialog provider={connecting} onClose={() => setConnecting(null)} />}
      {rotating && <CredentialsDialog provider={providers?.find((p) => p.key === rotating.provider)} connection={rotating} onClose={() => setRotating(null)} />}
    </SettingsPage>
  );
}

/** Connect a provider, or replace a connection's credentials. */
function CredentialsDialog({ provider, connection, onClose }: { provider?: PhoneProvider; connection?: PhoneConnection; onClose: () => void }) {
  const { t } = useTranslation("settingsChannels");
  const qc = useQueryClient();
  const [credentials, setCredentials] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const name = provider?.name ?? connection?.provider ?? "";
  const save = useMutation({
    mutationFn: () =>
      connection ? api.phone.update(connection.id, { credentials }) : api.phone.connect({ provider: provider?.key ?? "", credentials }),
    onSuccess: (row) => {
      upsertInList(qc, qk.phoneConnections, row);
      toast.success(connection ? t("phone.rotate.saved") : t("phone.connect.connected", { name }));
      onClose();
    },
    onError: (e) => setError(errorMessage(e, t("phone.connect.error", { name }))),
  });
  const fields = provider?.fields ?? [];
  const valid = fields.length > 0 && fields.every((f) => credentials[f.name]?.trim());
  return (
    <Dialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={connection ? t("phone.rotate.title", { name: connection.name }) : t("phone.connect.title", { name })}
      description={provider?.key === "aircall" ? t("phone.connect.aircallSetup") : undefined}
      footer={
        <>
          <Button onClick={onClose}>{t("phone.connect.cancel")}</Button>
          <Button variant="primary" disabled={!valid} loading={save.isPending} onClick={() => save.mutate()}>
            {connection ? t("phone.rotate.submit") : t("phone.connect.submit")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {fields.map((f) => (
          <Field key={f.name} label={knownField(f.name) ? t(`phone.field.${f.name}`) : f.label}>
            {(props) => (
              <TextInput
                {...props}
                type={f.secret ? "password" : "text"}
                autoComplete="off"
                spellCheck={false}
                className="mono"
                value={credentials[f.name] ?? ""}
                onChange={(e) => setCredentials((old) => ({ ...old, [f.name]: e.target.value }))}
              />
            )}
          </Field>
        ))}
        {error && <p className="text-[12px] text-danger-fg">{error}</p>}
      </div>
    </Dialog>
  );
}

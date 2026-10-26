// Settings → Integrations → Ticket panels (contracts/integrations.md "Ticket panels"): admins
// add a Shopify store or a URL of their own; every ticket then shows what it knows about the
// contact. A store connects with its app's client ID and secret, or an older app's access
// token. Secrets and tokens never come back: they are replaced, a signing secret renewed.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUpRight, KeyRound, Link2, MoreHorizontal, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Section } from "@/components/layout";
import { PanelMark } from "@/components/tickets/detail/TicketPanels";
import { Button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { CopyField } from "@/components/ui/copy-button";
import { Dialog } from "@/components/ui/dialog";
import { Field, TextInput } from "@/components/ui/input";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { Switch } from "@/components/ui/switch";
import { Segmented } from "@/components/ui/tabs";
import { toast } from "@/components/ui/toast";
import { api } from "@/lib/api/endpoints";
import { errorMessage } from "@/lib/api/client";
import type { TicketPanel } from "@/lib/api/types";
import { qk, q, removeFromList, upsertInList } from "@/lib/queries";
import { ListSkeleton, StatePill } from "./bits";
import { MotionList, MotionRow } from "./motion";

type Kind = TicketPanel["kind"];
type ShopifyAuth = NonNullable<TicketPanel["auth"]>;
type ShopifyCred = { auth: ShopifyAuth; clientId: string; clientSecret: string; token: string };

const DEV_DASHBOARD = "https://dev.shopify.com/dashboard";

/** What a Shopify panel is saved with: its app's client ID and secret, or a token; null when one is missing. */
export function shopifyBody(c: ShopifyCred): { client_id: string; client_secret: string } | { token: string } | null {
  if (c.auth === "access_token") return c.token.trim() ? { token: c.token.trim() } : null;
  const id = c.clientId.trim();
  const secret = c.clientSecret.trim();
  return id && secret ? { client_id: id, client_secret: secret } : null;
}

/** https://, or http:// on this machine (a development instance allows it). */
export function panelUrlOk(v: string): boolean {
  try {
    const u = new URL(v.trim());
    const local = u.hostname === "localhost" || u.hostname === "[::1]" || /^127\.\d+\.\d+\.\d+$/.test(u.hostname);
    return u.protocol === "https:" || (u.protocol === "http:" && local);
  } catch {
    return false;
  }
}

/** A shop's handle, domain or admin URL, as the server will read it; null when it can't be one. */
export function shopDomain(v: string): string | null {
  let s = v.trim().toLowerCase();
  try {
    if (s.includes("://")) s = new URL(s).host;
  } catch {
    return null;
  }
  s = s.replace(/\/+$/, "");
  if (s && !s.includes(".")) s += ".myshopify.com";
  return /^[a-z0-9][a-z0-9-]{0,59}\.myshopify\.com$/.test(s) ? s : null;
}

export function TicketPanelsSection({ admin }: { admin: boolean }) {
  const { t } = useTranslation("settingsDev");
  const { data: panels, isLoading } = useQuery(q.ticketPanels());
  const [adding, setAdding] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const rows = panels ?? [];

  return (
    <Section
      flush
      title={t("panels.title")}
      description={admin ? t("panels.hint") : `${t("panels.hint")} ${t("panels.adminsOnly")}`}
      actions={
        admin && (
          <Button size="xs" icon={<Plus className="size-3.5" />} onClick={() => setAdding(true)}>
            {t("panels.add")}
          </Button>
        )
      }
    >
      {isLoading ? (
        <ListSkeleton rows={1} />
      ) : rows.length ? (
        <MotionList>
          {rows.map((p) => (
            <PanelRow key={p.id} p={p} admin={admin} onSecret={setSecret} />
          ))}
        </MotionList>
      ) : (
        <p className="px-4 py-3 text-[12px] text-fg-3">{t("panels.empty")}</p>
      )}
      {adding && (
        <AddPanelDialog
          onClose={() => setAdding(false)}
          onAdded={(p, s) => {
            setAdding(false);
            toast.success(t("panels.added", { name: p.name }));
            if (s) setSecret(s);
          }}
        />
      )}
      <PanelSecretDialog secret={secret} onClose={() => setSecret(null)} />
    </Section>
  );
}

function PanelRow({ p, admin, onSecret }: { p: TicketPanel; admin: boolean; onSecret: (s: string) => void }) {
  const { t } = useTranslation("settingsDev");
  const qc = useQueryClient();
  const confirm = useConfirm();
  const [editing, setEditing] = useState(false);
  const state = !p.enabled ? ("paused" as const) : p.status === "error" ? ("failing" as const) : ("active" as const);
  const stateWord = !p.enabled ? t("panels.state.off") : t(`panels.state.${p.status}`);

  const update = useMutation({
    mutationFn: (b: { enabled?: boolean }) => api.ticketPanels.update(p.id, b),
    onMutate: (b) => {
      const prev = qc.getQueryData<TicketPanel[]>(qk.ticketPanels);
      upsertInList(qc, qk.ticketPanels, { ...p, ...b });
      return { prev };
    },
    onError: (e, _b, c) => {
      if (c?.prev) qc.setQueryData(qk.ticketPanels, c.prev);
      toast.error(e, t("panels.updateError"));
    },
    onSuccess: (row) => upsertInList(qc, qk.ticketPanels, row),
  });
  const rotate = useMutation({
    mutationFn: () => api.ticketPanels.rotateSecret(p.id),
    onSuccess: (r) => {
      upsertInList(qc, qk.ticketPanels, r.panel);
      onSecret(r.secret);
    },
    onError: (e) => toast.error(e),
  });
  const remove = useMutation({
    mutationFn: () => api.ticketPanels.remove(p.id),
    onSuccess: () => {
      removeFromList(qc, qk.ticketPanels, p.id);
      toast.success(t("panels.removed", { name: p.name }));
    },
    onError: (e) => toast.error(e),
  });

  return (
    <MotionRow>
      <div className="flex items-center gap-3 px-4 py-2.5">
        <PanelMark panel={p} size="md" />
        <div className={p.enabled ? "min-w-0 flex-1" : "min-w-0 flex-1 opacity-60"}>
          <div className="flex items-center gap-2">
            <span className="truncate text-[13px] font-medium text-fg">{p.name}</span>
            <span className="mono hidden truncate text-[11.5px] text-fg-4 sm:inline">{p.host}</span>
          </div>
          <div className="truncate text-[12px] text-fg-3">{p.status === "error" && p.last_error ? p.last_error : t(`panels.kind.${p.kind}`)}</div>
        </div>
        <StatePill state={state}>{stateWord}</StatePill>
        {admin && (
          <div className="flex items-center gap-1">
            <Switch checked={p.enabled} onCheckedChange={(enabled) => update.mutate({ enabled })} aria-label={t("panels.toggle", { name: p.name })} />
            <Menu>
              <MenuTrigger asChild>
                <button
                  type="button"
                  aria-label={t("panels.actions", { name: p.name })}
                  className="flex size-7 items-center justify-center rounded-md text-fg-3 transition-colors hover:bg-hover hover:text-fg"
                >
                  <MoreHorizontal className="size-4" />
                </button>
              </MenuTrigger>
              <MenuContent>
                {p.kind === "shopify" ? (
                  <MenuItem icon={<KeyRound />} onSelect={() => setEditing(true)}>
                    {t("panels.replaceCredentials")}
                  </MenuItem>
                ) : (
                  <>
                    <MenuItem icon={<Link2 />} onSelect={() => setEditing(true)}>
                      {t("panels.changeUrl")}
                    </MenuItem>
                    <MenuItem icon={<KeyRound />} onSelect={() => rotate.mutate()}>
                      {t("panels.rotateSecret")}
                    </MenuItem>
                  </>
                )}
                <MenuSeparator />
                <MenuItem
                  danger
                  icon={<Trash2 />}
                  onSelect={() =>
                    confirm.show(t("panels.removeBody"), () => remove.mutateAsync(), {
                      title: t("panels.removeTitle", { name: p.name }),
                      confirmLabel: t("panels.remove"),
                      danger: true,
                    })
                  }
                >
                  {t("panels.remove")}
                </MenuItem>
              </MenuContent>
            </Menu>
          </div>
        )}
      </div>
      {editing && <EditPanelDialog p={p} onClose={() => setEditing(false)} />}
    </MotionRow>
  );
}

function AddPanelDialog({ onClose, onAdded }: { onClose: () => void; onAdded: (p: TicketPanel, secret?: string) => void }) {
  const { t } = useTranslation("settingsDev");
  const qc = useQueryClient();
  const [kind, setKind] = useState<Kind>("shopify");
  const [shop, setShop] = useState("");
  const [cred, setCred] = useState<ShopifyCred>({ auth: "client_credentials", clientId: "", clientSecret: "", token: "" });
  const [url, setUrl] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () =>
      api.ticketPanels.create(
        kind === "shopify"
          ? { kind, shop: shop.trim(), ...shopifyBody(cred), name: name.trim() || undefined }
          : { kind, url: url.trim(), name: name.trim() || undefined },
      ),
    onSuccess: (r) => {
      upsertInList(qc, qk.ticketPanels, r.panel);
      onAdded(r.panel, r.secret);
    },
    onError: (e) => setError(errorMessage(e)),
  });

  const submit = () => {
    setError(null);
    if (kind === "shopify") {
      if (!shopDomain(shop)) return setError(t("panels.shopRequired"));
      if (!shopifyBody(cred)) return setError(t(cred.auth === "access_token" ? "panels.tokenRequired" : "panels.appRequired"));
    } else if (!panelUrlOk(url)) return setError(t("panels.urlRequired"));
    create.mutate();
  };

  return (
    <Dialog
      open
      size="md"
      onOpenChange={(o) => !o && onClose()}
      title={t("panels.addTitle")}
      description={t("panels.addDescription")}
      footer={
        <>
          <span className="mr-auto text-xs text-danger-fg">{error}</span>
          <Button variant="ghost" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" loading={create.isPending} onClick={submit}>
            {t("panels.connect")}
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-4 pb-1"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <Segmented<Kind>
          value={kind}
          onChange={(k) => {
            setKind(k);
            setError(null);
          }}
          options={[
            { value: "shopify", label: t("panels.kind.shopify") },
            { value: "http", label: t("panels.kind.http") },
          ]}
        />
        {kind === "shopify" ? (
          <>
            <Field label={t("panels.shop")} hint={t("panels.shopHint")}>
              {(f) => <TextInput {...f} autoFocus className="mono" value={shop} placeholder="my-store.myshopify.com" onChange={(e) => setShop(e.target.value)} />}
            </Field>
            <ShopifyCredentialFields value={cred} onChange={setCred} />
          </>
        ) : (
          <Field label={t("panels.url")} hint={t("panels.urlHint")}>
            {(f) => <TextInput {...f} autoFocus className="mono" value={url} placeholder="https://erp.example.com/opendesk/panel" onChange={(e) => setUrl(e.target.value)} />}
          </Field>
        )}
        <Field label={t("panels.name")} optional>
          {(f) => (
            <TextInput {...f} value={name} maxLength={64} placeholder={kind === "shopify" ? t("panels.kind.shopify") : t("panels.namePlaceholder")} onChange={(e) => setName(e.target.value)} />
          )}
        </Field>
        <button type="submit" className="hidden" />
      </form>
    </Dialog>
  );
}

/** How a Shopify panel connects: its app's client ID and secret, or an older app's access token. */
function ShopifyCredentialFields({ value, onChange, autoFocus }: { value: ShopifyCred; onChange: (c: ShopifyCred) => void; autoFocus?: boolean }) {
  const { t } = useTranslation("settingsDev");
  const set = (c: Partial<ShopifyCred>) => onChange({ ...value, ...c });
  return (
    <>
      <div className="flex flex-col gap-1.5">
        <span className="text-[12.5px] font-medium text-fg-2">{t("panels.auth.label")}</span>
        <Segmented<ShopifyAuth>
          value={value.auth}
          onChange={(auth) => set({ auth })}
          className="self-start"
          options={[
            { value: "client_credentials", label: t("panels.auth.client_credentials") },
            { value: "access_token", label: t("panels.auth.access_token") },
          ]}
        />
      </div>
      {value.auth === "client_credentials" ? (
        <>
          <Field label={t("panels.clientId")}>
            {(f) => (
              <TextInput {...f} autoFocus={autoFocus && !value.clientId} autoComplete="off" spellCheck={false} className="mono" value={value.clientId} onChange={(e) => set({ clientId: e.target.value })} />
            )}
          </Field>
          <Field
            label={t("panels.clientSecret")}
            hint={
              <>
                {t("panels.appHint")}{" "}
                <a href={DEV_DASHBOARD} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-0.5 text-fg-2 underline decoration-line-strong underline-offset-2 hover:text-fg">
                  {t("panels.openDevDashboard")}
                  <ArrowUpRight className="size-3" aria-hidden />
                </a>
              </>
            }
          >
            {(f) => (
              <TextInput {...f} autoFocus={autoFocus && !!value.clientId} type="password" autoComplete="off" spellCheck={false} className="mono" value={value.clientSecret} onChange={(e) => set({ clientSecret: e.target.value })} />
            )}
          </Field>
        </>
      ) : (
        <Field label={t("panels.token")} hint={t("panels.tokenHint")}>
          {(f) => (
            <TextInput
              {...f}
              autoFocus={autoFocus}
              type="password"
              autoComplete="off"
              spellCheck={false}
              className="mono"
              value={value.token}
              placeholder="shpat_…"
              onChange={(e) => set({ token: e.target.value })}
            />
          )}
        </Field>
      )}
    </>
  );
}

function EditPanelDialog({ p, onClose }: { p: TicketPanel; onClose: () => void }) {
  const { t } = useTranslation("settingsDev");
  const qc = useQueryClient();
  const shopify = p.kind === "shopify";
  // A rotated secret keeps the app's client ID, so it starts filled in.
  const [cred, setCred] = useState<ShopifyCred>({ auth: p.auth ?? "client_credentials", clientId: p.client_id ?? "", clientSecret: "", token: "" });
  const [url, setUrl] = useState("");
  const [error, setError] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: () => api.ticketPanels.update(p.id, shopify ? { ...shopifyBody(cred) } : { url: url.trim() }),
    onSuccess: (row) => {
      upsertInList(qc, qk.ticketPanels, row);
      onClose();
    },
    onError: (e) => setError(errorMessage(e)),
  });
  const submit = () => {
    setError(null);
    if (shopify && !shopifyBody(cred)) return setError(t(cred.auth === "access_token" ? "panels.tokenRequired" : "panels.appRequired"));
    if (!shopify && !panelUrlOk(url)) return setError(t("panels.urlRequired"));
    save.mutate();
  };
  return (
    <Dialog
      open
      size="md"
      onOpenChange={(o) => !o && onClose()}
      title={shopify ? t("panels.credentialsTitle", { name: p.name }) : t("panels.urlTitle", { name: p.name })}
      footer={
        <>
          <span className="mr-auto text-xs text-danger-fg">{error}</span>
          <Button variant="ghost" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" loading={save.isPending} onClick={submit}>
            {t("panels.save")}
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-4 pb-1"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        {shopify ? (
          <ShopifyCredentialFields value={cred} onChange={setCred} autoFocus />
        ) : (
          <Field label={t("panels.url")} hint={t("panels.urlHint")}>
            {(f) => (
              <TextInput {...f} autoFocus className="mono" autoComplete="off" value={url} placeholder="https://erp.example.com/opendesk/panel" onChange={(e) => setUrl(e.target.value)} />
            )}
          </Field>
        )}
        <button type="submit" className="hidden" />
      </form>
    </Dialog>
  );
}

const ANSWER_SAMPLE = `{ "sections": [ { "title": "Order", "items": [
  { "label": "Confirmed delivery", "value": "2026-11-04", "url": "https://erp.example.com/so/42" } ] } ] }`;

function PanelSecretDialog({ secret, onClose }: { secret: string | null; onClose: () => void }) {
  const { t } = useTranslation("settingsDev");
  return (
    <Dialog
      open={!!secret}
      onOpenChange={(o) => !o && onClose()}
      size="lg"
      title={t("panels.secretTitle")}
      description={t("panels.secretDescription")}
      footer={
        <Button variant="primary" onClick={onClose}>
          {t("panels.secretSaved")}
        </Button>
      }
    >
      <div className="flex flex-col gap-4 pb-1">
        <CopyField value={secret ?? ""} />
        <div className="flex flex-col gap-2">
          <p className="text-[12.5px] text-fg-2">{t("panels.secretVerify")}</p>
          <CopyField multiline value={`OD-Signature: t=<unix>,v1=<hex hmac_sha256(secret, "<t>.<body>")>`} />
          <p className="text-[12.5px] text-fg-2">{t("panels.secretAnswer")}</p>
          <CopyField multiline value={ANSWER_SAMPLE} />
        </div>
      </div>
    </Dialog>
  );
}

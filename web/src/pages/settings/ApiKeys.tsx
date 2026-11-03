import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, CircleDot, CircleOff, EyeOff, KeyRound, ListTree, MoreHorizontal, Pencil, Plus, ShieldCheck, SlidersHorizontal, Trash2 } from "lucide-react";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { NoAccess, Section, SettingsPage } from "@/components/layout";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, TextInput } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Select } from "@/components/ui/select";
import { Segmented } from "@/components/ui/tabs";
import { SettingsEmpty } from "@/components/settings2/SettingsEmpty";
import { CopyField } from "@/components/ui/copy-button";
import { Tooltip } from "@/components/ui/tooltip";
import { Avatar } from "@/components/ui/avatar";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { useConfirm } from "@/components/ui/confirm";
import { IconTile, ListSkeleton, MonoChip, StatePill, SummaryStrip } from "@/components/settings2/bits";
import { MotionList, MotionRow } from "@/components/settings2/motion";
import { useListMutation } from "@/components/settings2/optimistic";
import { GrantedTeams, PrivateTeamsField } from "@/components/settings2/teams";
import { ReauthNotice, isReauthError } from "@/components/settings/ReauthNotice";
import { api } from "@/lib/api/endpoints";
import { errorMessage } from "@/lib/api/client";
import type { ApiKey } from "@/lib/api/types";
import { fullDate, relLong, shortDate } from "@/lib/format";
import { q, qk, upsertInList } from "@/lib/queries";
import { atLeast, useRole } from "@/lib/session";
import { cn } from "@/lib/cn";
import { ApiKeyRequestsDrawer } from "./ApiKeyRequests";
import { LogCell, ScopeSummary, type LogView } from "./apiKeysList";

const scopeGroups: { resource: string; label: string; scopes: { id: string; label: string }[] }[] = [
  { resource: "tickets", label: "Tickets", scopes: [{ id: "tickets:read", label: "Read" }, { id: "tickets:write", label: "Write" }] },
  { resource: "contacts", label: "Contacts", scopes: [{ id: "contacts:read", label: "Read" }, { id: "contacts:write", label: "Write" }] },
  { resource: "kb", label: "Knowledge base", scopes: [{ id: "kb:read", label: "Read" }, { id: "kb:write", label: "Write" }] },
  { resource: "changelog", label: "Changelog", scopes: [{ id: "changelog:read", label: "Read" }, { id: "changelog:write", label: "Write" }] },
  { resource: "wiki", label: "Wiki", scopes: [{ id: "wiki:read", label: "Read" }, { id: "wiki:write", label: "Write" }] },
  { resource: "settings", label: "Settings", scopes: [{ id: "settings:read", label: "Read" }, { id: "settings:write", label: "Write" }] },
  { resource: "realtime", label: "Realtime", scopes: [{ id: "realtime", label: "Subscribe" }] },
  // Acts as the key's creator in Collab: reads the mail they see and saves reply drafts, never sends.
  { resource: "collab", label: "Collab (as you)", scopes: [{ id: "collab:draft", label: "Draft replies" }] },
];
// collab:draft reaches the creator's mail, so no preset includes it: it is chosen by hand.
const optInScopes = ["collab:draft"];
const allScopes = scopeGroups.flatMap((g) => g.scopes.map((s) => s.id)).filter((s) => !optInScopes.includes(s));
const readScopes = allScopes.filter((s) => s.endsWith(":read"));

type Expiry = "never" | "30" | "90" | "365";

export default function ApiKeys() {
  const { t } = useTranslation("settingsDev");
  const role = useRole();
  const admin = atLeast(role, "admin");
  const { data: keys, isLoading } = useQuery({ ...q.apiKeys(), enabled: admin });
  const confirm = useConfirm();
  const [creating, setCreating] = useState(false);
  const [created, setCreated] = useState<(ApiKey & { secret: string }) | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [logging, setLogging] = useState<ApiKey | null>(null);
  const [scoping, setScoping] = useState<ApiKey | null>(null);
  const [logView, setLogView] = useState<LogView>(undefined);
  const [showExpired, setShowExpired] = useState(false);

  const revoke = useListMutation<ApiKey, string>(qk.apiKeys, (id) => api.apiKeys.remove(id), (rows, id) => rows.filter((k) => k.id !== id), {
    errorText: t("apiKeys.revokeError"),
  });
  const rename = useListMutation<ApiKey, { id: string; name: string }>(
    qk.apiKeys,
    (v) => api.apiKeys.update(v.id, { name: v.name }),
    (rows, v) => rows.map((k) => (k.id === v.id ? { ...k, name: v.name } : k)),
    { errorText: t("apiKeys.renameError") },
  );

  const recordRequests = useListMutation<ApiKey, { id: string; on: boolean }>(
    qk.apiKeys,
    (v) => api.apiKeys.update(v.id, { record_requests: v.on }),
    (rows, v) => rows.map((k) => (k.id === v.id ? { ...k, record_requests: v.on } : k)),
    { errorText: t("apiKeys.requests.recordError") },
  );

  if (!admin)
    return (
      <SettingsPage title={t("apiKeys.title")}>
        <NoAccess what="API keys" />
      </SettingsPage>
    );

  const sorted = [...(keys ?? [])].sort((a, b) => b.created_at.localeCompare(a.created_at));
  const now = Date.now();
  // Expired keys wait at the bottom, folded: they no longer work.
  const live = sorted.filter((k) => keyState(k, now) !== "expired");
  const expired = sorted.filter((k) => keyState(k, now) === "expired");
  const counts = sorted.reduce(
    (c, k) => {
      const s = keyState(k, now);
      c[s] += 1;
      if (!k.last_used_at) c.unused += 1;
      return c;
    },
    { active: 0, expiring: 0, expired: 0, unused: 0 },
  );

  return (
    <SettingsPage
      wide
      title={t("apiKeys.title")}
      description={
        <>
          {t("apiKeys.description")} <span className="mono text-fg-2">Authorization: Bearer od_live_…</span>
        </>
      }
      actions={
        <Button variant="primary" icon={<Plus className="size-3.5" />} onClick={() => setCreating(true)}>
          {t("apiKeys.new")}
        </Button>
      }
    >
      {sorted.length > 0 && (
        <SummaryStrip
          items={[
            { label: t("apiKeys.summary.active"), value: counts.active + counts.expiring, tone: counts.active + counts.expiring ? "green" : undefined },
            { label: t("apiKeys.summary.expiring"), value: counts.expiring, tone: counts.expiring ? "amber" : undefined, hint: t("apiKeys.summary.expiringHint") },
            { label: t("apiKeys.summary.unused"), value: counts.unused },
            { label: t("apiKeys.summary.expired"), value: counts.expired, tone: counts.expired ? "gray" : undefined },
          ]}
        />
      )}
      <Section flush title={keys ? t("apiKeys.count", { count: sorted.length }) : undefined}>
        {isLoading ? (
          <ListSkeleton rows={3} />
        ) : !sorted.length ? (
          <SettingsEmpty
            art="settings"
            title={t("apiKeys.empty.title")}
            description={t("apiKeys.empty.description")}
            action={
              <Button variant="primary" icon={<Plus className="size-3.5" />} onClick={() => setCreating(true)}>
                {t("apiKeys.new")}
              </Button>
            }
            tips={[
              { label: t("apiKeys.empty.tips.scopes"), icon: <ShieldCheck /> },
              { label: t("apiKeys.empty.tips.once"), icon: <EyeOff /> },
            ]}
          />
        ) : (
          <div className="@container">
            <div className={cn("hidden h-8 items-center gap-3 border-b border-line-soft bg-sunken px-4 @[48rem]:grid", rowGrid)}>
              <span className="label-caps">{t("apiKeys.col.key")}</span>
              <span className="label-caps">{t("apiKeys.col.scopes")}</span>
              <span className="label-caps">{t("apiKeys.col.log")}</span>
              <span className="label-caps">{t("apiKeys.col.lastUsed")}</span>
              <span className="label-caps">{t("apiKeys.col.expires")}</span>
              <span />
            </div>
            <MotionList>
              {[...live, ...(showExpired ? expired : [])].map((k) => {
                const state = keyState(k, now);
                return (
                  <MotionRow key={k.id}>
                    <div className={cn("group relative flex flex-col gap-2 px-4 py-3 transition-colors hover:bg-sunken @[48rem]:grid @[48rem]:items-center @[48rem]:gap-3", rowGrid)}>
                      <div className="flex min-w-0 items-center gap-3">
                        <IconTile tone={state === "expired" ? "gray" : "violet"}>
                          <KeyRound />
                        </IconTile>
                        <div className="min-w-0 flex-1">
                          <div className="flex min-w-0 items-center gap-2">
                            {renaming === k.id ? (
                              <InlineName
                                initial={k.name}
                                onDone={(name) => {
                                  setRenaming(null);
                                  if (name && name !== k.name) rename.mutate({ id: k.id, name });
                                }}
                              />
                            ) : (
                              <span className="truncate text-[13px] font-medium">{k.name}</span>
                            )}
                            {state === "expired" && <StatePill state="expired">{t("apiKeys.state.expired")}</StatePill>}
                          </div>
                          <div className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[11px] text-fg-4">
                            <span className="mono min-w-0 truncate">{k.prefix.slice(0, 12)}…</span>
                            <Tooltip content={k.created_by ? `${t("apiKeys.col.createdBy")} ${k.created_by.name}, ${fullDate(k.created_at)}` : fullDate(k.created_at)}>
                              <span className="flex shrink-0 items-center gap-1">
                                <span aria-hidden>·</span>
                                {k.created_by && <Avatar name={k.created_by.name} src={k.created_by.avatar_url} size="xs" />}
                                <span className="shrink-0">{shortDate(k.created_at)}</span>
                              </span>
                            </Tooltip>
                          </div>
                          <GrantedTeams grants={k.private_teams} className="mt-1.5" />
                        </div>
                      </div>
                      <div className="min-w-0 @max-[48rem]:pl-11">
                        <ScopeSummary scopes={k.scopes} full={isFull(k.scopes)} readOnly={isReadOnly(k.scopes)} name={k.name} onEdit={admin ? () => setScoping(k) : undefined} />
                      </div>
                      <div className="min-w-0 @max-[48rem]:pl-11">
                        <LogCell
                          apiKey={k}
                          onOpen={(view) => {
                            setLogView(view);
                            setLogging(k);
                          }}
                        />
                      </div>
                      <Tooltip content={k.last_used_at ? fullDate(k.last_used_at) : t("apiKeys.neverUsed")}>
                        <span className={cn("text-[12px] @max-[48rem]:pl-11", k.last_used_at ? "text-fg-2" : "text-fg-4")}>
                          {k.last_used_at ? relLong(k.last_used_at) : t("apiKeys.neverUsed")}
                        </span>
                      </Tooltip>
                      <Tooltip content={k.expires_at ? fullDate(k.expires_at) : t("apiKeys.noExpiry")}>
                        <span
                          aria-label={k.expires_at ? undefined : t("apiKeys.noExpiry")}
                          className={cn("text-[12px] @max-[48rem]:pl-11", !k.expires_at || state === "expired" ? "text-fg-4" : state === "expiring" ? "font-medium text-tone-amber" : "text-fg-2")}
                        >
                          {k.expires_at ? shortDate(k.expires_at) : "—"}
                        </span>
                      </Tooltip>
                      <Menu>
                        <MenuTrigger asChild>
                          <button
                            type="button"
                            aria-label={`Actions for ${k.name}`}
                            className="flex size-7 items-center justify-center rounded-md text-fg-3 transition-colors hover:bg-hover hover:text-fg @max-[48rem]:absolute @max-[48rem]:right-3 @max-[48rem]:top-3"
                          >
                            <MoreHorizontal className="size-4" />
                          </button>
                        </MenuTrigger>
                        <MenuContent>
                          <MenuItem icon={<Pencil />} onSelect={() => setRenaming(k.id)}>
                            {t("apiKeys.rename")}
                          </MenuItem>
                          <MenuItem icon={<SlidersHorizontal />} onSelect={() => setScoping(k)}>
                            {t("apiKeys.scopes.menu")}
                          </MenuItem>
                          <MenuItem
                            icon={<ListTree />}
                            onSelect={() => {
                              setLogView(undefined);
                              setLogging(k);
                            }}
                          >
                            {t("apiKeys.requests.menu")}
                          </MenuItem>
                          <MenuItem
                            icon={k.record_requests ? <CircleOff /> : <CircleDot />}
                            onSelect={() => recordRequests.mutate({ id: k.id, on: !k.record_requests })}
                          >
                            {k.record_requests ? t("apiKeys.requests.recordOff") : t("apiKeys.requests.recordOn")}
                          </MenuItem>
                          <MenuSeparator />
                          <MenuItem
                            danger
                            icon={<Trash2 />}
                            onSelect={() =>
                              confirm.show(
                                <>
                                  {t("apiKeys.revokeBodyPrefix")} <span className="font-medium text-fg">{k.name}</span> {t("apiKeys.revokeBodySuffix")}
                                </>,
                                () => revoke.mutateAsync(k.id),
                                { title: t("apiKeys.revokeTitle"), confirmLabel: t("apiKeys.revokeConfirm"), danger: true },
                              )
                            }
                          >
                            {t("apiKeys.revoke")}
                          </MenuItem>
                        </MenuContent>
                      </Menu>
                    </div>
                  </MotionRow>
                );
              })}
            </MotionList>
            {expired.length > 0 && (
              <button
                type="button"
                aria-expanded={showExpired}
                onClick={() => setShowExpired((v) => !v)}
                className="flex h-9 w-full items-center gap-1.5 border-t border-line-soft px-4 text-[12px] text-fg-3 transition-colors hover:bg-sunken hover:text-fg"
              >
                <ChevronRight className={cn("size-3.5 transition-transform", showExpired && "rotate-90")} />
                {t("apiKeys.list.expiredGroup", { count: expired.length })}
              </button>
            )}
          </div>
        )}
      </Section>

      {logging && <ApiKeyRequestsDrawer apiKey={sorted.find((k) => k.id === logging.id) ?? logging} initial={logView} onClose={() => setLogging(null)} />}
      {scoping && <ScopesDialog apiKey={scoping} onClose={() => setScoping(null)} />}
      {creating && (
        <CreateKeyDialog
          onClose={() => setCreating(false)}
          onCreated={(k) => {
            setCreating(false);
            setCreated(k);
          }}
        />
      )}
      <Dialog
        open={!!created}
        onOpenChange={(o) => !o && setCreated(null)}
        title={t("apiKeys.created.title")}
        description={t("apiKeys.created.description")}
        footer={
          <Button variant="primary" onClick={() => setCreated(null)}>
            {t("apiKeys.created.done")}
          </Button>
        }
      >
        <div className="flex flex-col gap-3 pb-1">
          <CopyField value={created?.secret ?? ""} />
          <p className="flex items-start gap-2 rounded-md bg-tone-amber-soft px-3 py-2 text-[12px] text-tone-amber">
            <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-tone-amber-solid" />
            {t("apiKeys.created.warning")}
          </p>
        </div>
      </Dialog>
    </SettingsPage>
  );
}

// Key (who made it on hover of the avatar), access, log, last used, expires, menu. The table needs
// about 48rem: narrower (a window with both sidebars open, a phone), the cells stack under the name,
// by the list's own width rather than the screen's.
const rowGrid = "@[48rem]:grid-cols-[minmax(11rem,1fr)_minmax(12rem,1.3fr)_104px_100px_80px_28px]";
// A preset reads as one chip: every scope but the opt-in ones, or every read scope.
const isFull = (scopes: string[]) => allScopes.every((s) => scopes.includes(s));
const isReadOnly = (scopes: string[]) => scopes.length === readScopes.length && readScopes.every((s) => scopes.includes(s));

const SOON = 14 * 86400_000;
function keyState(k: ApiKey, now: number): "active" | "expiring" | "expired" {
  if (!k.expires_at) return "active";
  const at = Date.parse(k.expires_at);
  if (at < now) return "expired";
  return at - now < SOON ? "expiring" : "active";
}

function InlineName({ initial, onDone }: { initial: string; onDone: (v: string | null) => void }) {
  const { t: ui } = useTranslation("ui");
  const [v, setV] = useState(initial);
  const cancelled = useRef(false);
  return (
    <TextInput
      autoFocus
      inputSize="sm"
      value={v}
      maxLength={80}
      aria-label={ui("keyName")}
      className="max-w-[260px]"
      onChange={(e) => setV(e.target.value)}
      onBlur={() => onDone(cancelled.current ? null : v.trim())}
      onKeyDown={(e) => {
        if (e.key === "Enter") (e.target as HTMLInputElement).blur();
        if (e.key === "Escape") {
          e.stopPropagation();
          cancelled.current = true;
          (e.target as HTMLInputElement).blur();
        }
      }}
    />
  );
}

export function CreateKeyDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (k: ApiKey & { secret: string }) => void }) {
  const { t } = useTranslation("settingsDev");
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>(readScopes);
  const [expiry, setExpiry] = useState<Expiry>("90");
  const [privateTeams, setPrivateTeams] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () =>
      api.apiKeys.create({
        name: name.trim(),
        scopes,
        expires_at: expiry === "never" ? null : new Date(Date.now() + Number(expiry) * 86400_000).toISOString(),
        ...(privateTeams.length ? { private_team_ids: privateTeams } : {}),
      }),
    onSuccess: (k) => {
      const { secret: _secret, ...row } = k;
      upsertInList(qc, qk.apiKeys, row as ApiKey, { prepend: true });
      onCreated(k);
    },
    // A key needs a recent sign-in (with 2FA when on): the notice below says why and signs in again.
    onError: (e) => {
      if (!isReauthError(e)) setError(errorMessage(e));
    },
  });

  const submit = () => {
    setError(null);
    if (!name.trim()) return setError(t("apiKeys.form.nameRequired"));
    if (!scopes.length) return setError(t("apiKeys.form.scopeRequired"));
    create.mutate();
  };

  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("apiKeys.form.title")}
      footer={
        <>
          <span className="mr-auto text-xs text-danger-fg">{error}</span>
          <Button variant="ghost" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" loading={create.isPending} onClick={submit}>
            {t("apiKeys.form.create")}
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
        <Field label={t("apiKeys.form.name")}>
          {(p) => <TextInput {...p} autoFocus value={name} maxLength={80} placeholder={t("apiKeys.form.namePlaceholder")} onChange={(e) => setName(e.target.value)} />}
        </Field>
        <ScopeChecklist scopes={scopes} onChange={setScopes} />
        <Field label={t("apiKeys.form.expires")}>
          {(p) => (
            <Select<Expiry>
              id={p.id}
              value={expiry}
              onChange={setExpiry}
              options={[
                { value: "30", label: t("apiKeys.form.in30") },
                { value: "90", label: t("apiKeys.form.in90") },
                { value: "365", label: t("apiKeys.form.in365") },
                { value: "never", label: t("apiKeys.form.never") },
              ]}
            />
          )}
        </Field>
        <PrivateTeamsField value={privateTeams} onChange={setPrivateTeams} />
        {isReauthError(create.error) && <ReauthNotice error={create.error} />}
        <button type="submit" className="hidden" />
      </form>
    </Dialog>
  );
}

/** The scope presets and checkboxes, for a new key and for changing an existing one's. */
function ScopeChecklist({ scopes, onChange }: { scopes: string[]; onChange: (scopes: string[]) => void }) {
  const { t } = useTranslation("settingsDev");
  const preset = scopes.length === allScopes.length ? "full" : scopes.length === readScopes.length && readScopes.every((s) => scopes.includes(s)) ? "read" : "custom";
  const toggle = (id: string, on: boolean) => onChange(on ? [...new Set([...scopes, id])] : scopes.filter((x) => x !== id));
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <span className="text-[12.5px] font-medium">{t("apiKeys.form.scopes")}</span>
        <Segmented
          size="xs"
          value={preset}
          onChange={(v) => {
            if (v === "read") onChange(readScopes);
            if (v === "full") onChange(allScopes);
          }}
          options={[
            { value: "read", label: t("apiKeys.form.readOnly") },
            { value: "full", label: t("apiKeys.form.full") },
            { value: "custom", label: t("apiKeys.form.custom") },
          ]}
        />
      </div>
      <div className="overflow-hidden rounded-md border border-line">
        {scopeGroups.map((g) => (
          <div key={g.resource} className="flex h-10 items-center gap-3 border-b border-line-soft px-3 last:border-0">
            <span className="flex-1 text-[12.5px] text-fg">{g.label}</span>
            {g.scopes.map((s) => (
              <label key={s.id} className={cn("flex items-center gap-1.5 text-[12px]", scopes.includes(s.id) ? "text-fg" : "text-fg-3")}>
                <Checkbox checked={scopes.includes(s.id)} onCheckedChange={(on) => toggle(s.id, on)} aria-label={`${g.label} ${s.label.toLowerCase()}`} />
                {s.label}
              </label>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

/**
 * Changes an existing key's scopes. Taking a scope away is immediate; adding one asks for the
 * same recent sign-in as making a key (the notice below explains a refusal and signs in again).
 */
export function ScopesDialog({ apiKey, onClose }: { apiKey: ApiKey; onClose: () => void }) {
  const { t } = useTranslation("settingsDev");
  const qc = useQueryClient();
  const [scopes, setScopes] = useState<string[]>(apiKey.scopes);
  const [error, setError] = useState<string | null>(null);
  const added = scopes.filter((s) => !apiKey.scopes.includes(s));
  const removed = apiKey.scopes.filter((s) => !scopes.includes(s));

  const save = useMutation({
    mutationFn: () => api.apiKeys.update(apiKey.id, { scopes }),
    onSuccess: (k) => {
      upsertInList(qc, qk.apiKeys, k);
      onClose();
    },
    onError: (e) => {
      if (!isReauthError(e)) setError(errorMessage(e));
    },
  });

  const submit = () => {
    setError(null);
    if (!scopes.length) return setError(t("apiKeys.form.scopeRequired"));
    save.mutate();
  };

  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("apiKeys.scopes.title", { name: apiKey.name })}
      description={t("apiKeys.scopes.description")}
      footer={
        <>
          <span className="mr-auto text-xs text-danger-fg">{error}</span>
          <Button variant="ghost" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" loading={save.isPending} disabled={!added.length && !removed.length} onClick={submit}>
            {t("apiKeys.scopes.save")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3 pb-1">
        <ScopeChecklist scopes={scopes} onChange={setScopes} />
        {(added.length > 0 || removed.length > 0) && (
          <div className="flex flex-wrap items-center gap-1" aria-label={t("apiKeys.scopes.changes")}>
            {added.map((s) => (
              <MonoChip key={s} className="border-tone-amber-line text-tone-amber">
                + {s}
              </MonoChip>
            ))}
            {removed.map((s) => (
              <MonoChip key={s} className="text-fg-4 line-through">
                − {s}
              </MonoChip>
            ))}
          </div>
        )}
        {added.length > 0 && <p className="text-[12px] text-fg-3">{t("apiKeys.scopes.addingNeedsSignIn")}</p>}
        {isReauthError(save.error) && <ReauthNotice error={save.error} />}
      </div>
    </Dialog>
  );
}

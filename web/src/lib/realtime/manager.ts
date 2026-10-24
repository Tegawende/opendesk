// One socket per signed-in tab. Joins workspace:<wid> and user:<uid> for
// events, and ticket:<wid>:<id> for presence and typing on the open ticket
// (and, quietly, the tickets visible in the list).

import type { InfiniteData, QueryClient } from "@tanstack/react-query";
import { getWorkspaceId } from "../api/client";
import { api } from "../api/endpoints";
import type { Envelope, Notification, Page, Ticket } from "../api/types";
import { qk } from "../queries";
import { config } from "../config";
import { applyEnvelope } from "./apply";
import { syncDiff, syncState, type PresenceDiff, type PresenceState } from "./presence";
import { SeqSync } from "./seq";
import { applyLive, clearLive, clearPresence, noteCustomerTyping, noteTyping, setPresence, useRealtime } from "./store";
import { createSocket, type RtChannel, type RtSocket } from "./transport";

/** Query roots that realtime events keep current (see apply.ts). */
const LIVE_ROOTS = new Set(["tickets", "contacts", "companies", "notifications", "members", "kb"]);

export type ManagerOptions = {
  qc: QueryClient;
  workspaceId: string;
  userId: string;
  onNotification?: (n: Notification) => void;
  onTicketCreated?: (t: Ticket) => void;
  onSelfMembershipChanged?: () => void;
};

type TicketMode = "active" | "watch";
/** team is the private team the channel was joined with (null: none). */
type TicketEntry = { ch: RtChannel; mode: TicketMode; state: PresenceState; team: string | null };

const MAX_WATCHED = 30;
const TOKEN_REFRESH_MS = 4 * 60_000;

/**
 * The private teams a ws token may join (its `teams` claim). The payload is read
 * without checking the signature: the gateway checks it, this only picks topics.
 */
export function tokenTeams(token: string): string[] {
  const part = token.split(".")[1];
  if (!part) return [];
  try {
    const b64 = part.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(part.length / 4) * 4, "=");
    const json = JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(b64), (c) => c.charCodeAt(0)))) as { teams?: unknown };
    return Array.isArray(json.teams) ? json.teams.filter((x): x is string => typeof x === "string") : [];
  } catch {
    return [];
  }
}

export class RealtimeManager {
  private socket: RtSocket | null = null;
  private token = "";
  private tokenTimer: ReturnType<typeof setInterval> | null = null;
  private stopped = false;
  private sync: SeqSync;
  private tickets = new Map<string, TicketEntry>();
  private active: string | null = null;
  private watched = new Set<string>();
  /** Watch lists by who asked (the inbox list, waiting chats), merged into `watched`. */
  private watchSources = new Map<string, string[]>();
  private lastTyping = 0;
  private everOpened = false;
  /** team:<wid>:<tid> channels for the private teams in the current token. */
  private teamChannels = new Map<string, RtChannel>();
  private teamIds: string[] = [];
  private unsubscribeCache: (() => void) | null = null;

  constructor(private opts: ManagerOptions) {
    this.sync = new SeqSync({
      fetchSince: (since) => api.events(since),
      apply: (env) => this.apply(env),
      resync: () => void opts.qc.invalidateQueries(),
    });
  }

  private apply(env: Envelope, viaUser = false) {
    applyEnvelope(this.opts.qc, env, {
      me: this.opts.userId,
      viaUser,
      privateTeams: () => this.teamIds,
      onNotification: this.opts.onNotification,
      onTicketCreated: this.opts.onTicketCreated,
      onSelfMembershipChanged: this.opts.onSelfMembershipChanged,
    });
  }

  private async fetchToken(): Promise<{ token: string; url: string }> {
    // The token is for this manager's workspace; once the session has moved on
    // (switched, left or deleted it, logged out) there is nothing to ask for.
    if (this.stopped || getWorkspaceId() !== this.opts.workspaceId) throw new Error("realtime: workspace changed");
    const r = await api.realtimeToken();
    this.token = r.token;
    this.teamIds = tokenTeams(r.token);
    this.reconcileTeams();
    this.reconcileTickets();
    return r;
  }

  async start() {
    useRealtime.setState({ status: "connecting" });
    let url = config.gatewayUrl;
    try {
      const r = await this.fetchToken();
      if (r.url && !window.__OPENDESK__?.gatewayUrl && !import.meta.env.VITE_GATEWAY_URL) url = r.url;
    } catch {
      if (this.stopped) return;
      useRealtime.setState({ status: "offline" });
      setTimeout(() => !this.stopped && void this.start(), 5000);
      return;
    }
    if (this.stopped) return;

    const socket = createSocket(url, () => ({ token: this.token }));
    this.socket = socket;
    socket.onOpen(() => {
      useRealtime.setState({ status: "live" });
      this.everOpened = true;
    });
    const dropped = () => {
      if (this.stopped) return;
      useRealtime.setState({ status: this.everOpened ? "reconnecting" : "connecting" });
      // Tokens only need to be valid at connect; fetch a fresh one for the retry.
      this.fetchToken().catch(() => useRealtime.setState({ status: "offline" }));
    };
    socket.onClose(dropped);
    socket.onError(dropped);
    socket.connect();
    this.tokenTimer = setInterval(() => void this.fetchToken().catch(() => {}), TOKEN_REFRESH_MS);

    const { workspaceId: wid, userId: uid } = this.opts;
    const ws = socket.channel(`workspace:${wid}`);
    ws.on("event", (p) => this.sync.receive(p as Envelope));
    let joinedOnce = false;
    ws.join().receive("ok", () => {
      if (!joinedOnce) {
        joinedOnce = true;
        const joinedAt = Date.now();
        if (this.sync.last === null) {
          api
            .events(0, 1)
            .then((page) => {
              if (this.sync.last !== null) return;
              this.sync.baseline(page.latest_seq);
              // Lists loaded before the join can miss events published in between,
              // which the channel never delivers: refetch those once.
              void this.opts.qc.invalidateQueries({
                predicate: (q) => LIVE_ROOTS.has(String(q.queryKey[0])) && q.state.dataUpdatedAt > 0 && q.state.dataUpdatedAt < joinedAt,
              });
            })
            .catch(() => {});
        }
      } else {
        // A rejoin after a dropped connection: pick up what we missed.
        void this.sync.catchUp();
      }
    });

    const user = socket.channel(`user:${uid}`);
    user.on("event", (p) => {
      const env = p as Envelope;
      if (env.workspace_id !== wid) return;
      // Notifications are applied from here only (the workspace copy carries no topics).
      if (env.type === "notification.created") return this.apply(env, true);
      if (this.sync.hasSeen(env.id)) return;
      this.sync.markSeen(env.id);
      this.apply(env);
    });
    user.join();

    this.reconcileTeams();
    this.reconcileTickets();
    // A ticket's team arrives with its data (or changes): rejoin its channel with it.
    this.unsubscribeCache = this.opts.qc.getQueryCache().subscribe((e) => {
      const key = e.query.queryKey;
      if (e.type !== "updated" || key[0] !== "tickets" || key[1] !== "detail") return;
      const id = String(key[2]);
      const entry = this.tickets.get(id);
      if (entry && entry.team !== this.privateTeamOf(id)) this.reconcileTickets();
    });
  }

  /**
   * The ticket's private team when the token may join it: a private team's presence,
   * typing and live previews reach only sockets that joined its ticket with team_id.
   */
  private privateTeamOf(id: string): string | null {
    let team: string | null = null;
    const own = this.opts.qc.getQueryData<Ticket>(qk.ticket(id));
    if (own) team = own.team_id ?? null;
    else {
      for (const [, data] of this.opts.qc.getQueriesData<InfiniteData<Page<Ticket>>>({ queryKey: qk.ticketLists })) {
        const hit = data?.pages?.flatMap((p) => p.data ?? []).find((t) => t.id === id);
        if (hit) {
          team = hit.team_id ?? null;
          break;
        }
      }
    }
    return team && this.teamIds.includes(team) ? team : null;
  }

  /**
   * A private team's ticket events skip workspace:<wid> (it gets a stand-in) and come on
   * the team's channel. They are applied as they arrive; seq stays the workspace's.
   */
  private reconcileTeams() {
    if (!this.socket) return;
    const want = new Set(this.teamIds);
    for (const [tid, ch] of this.teamChannels) {
      if (!want.has(tid)) {
        ch.leave();
        this.teamChannels.delete(tid);
      }
    }
    const wid = this.opts.workspaceId;
    for (const tid of want) {
      if (this.teamChannels.has(tid)) continue;
      const ch = this.socket.channel(`team:${wid}:${tid}`);
      ch.on("event", (p) => {
        const env = p as Envelope;
        if (env.workspace_id !== wid || this.sync.hasSeen(env.id)) return;
        this.sync.markSeen(env.id);
        this.apply(env);
      });
      ch.join();
      this.teamChannels.set(tid, ch);
    }
  }

  stop() {
    this.stopped = true;
    if (this.tokenTimer) clearInterval(this.tokenTimer);
    this.unsubscribeCache?.();
    this.unsubscribeCache = null;
    for (const [tid, e] of this.tickets) {
      e.ch.leave();
      clearPresence(tid);
      clearLive(tid);
    }
    this.tickets.clear();
    for (const ch of this.teamChannels.values()) ch.leave();
    this.teamChannels.clear();
    this.socket?.disconnect();
    this.socket = null;
    useRealtime.setState({ status: "idle" });
  }

  setActiveTicket(id: string | null) {
    this.active = id;
    useRealtime.setState({ active: id });
    this.reconcileTickets();
  }

  /** Tickets whose presence to follow, per source; waiting chats come first when over the cap. */
  setWatched(ids: string[], source = "list") {
    this.watchSources.set(source, ids);
    const merged = [...(this.watchSources.get("waiting") ?? []), ...[...this.watchSources].filter(([k]) => k !== "waiting").flatMap(([, v]) => v)];
    this.watched = new Set(merged.slice(0, MAX_WATCHED));
    this.reconcileTickets();
  }

  /** viewing | replying | idle for the open ticket. */
  setPresenceState(state: "viewing" | "replying" | "idle") {
    if (!this.active) return;
    const e = this.tickets.get(this.active);
    if (e?.mode === "active") e.ch.push("presence", { state });
  }

  /**
   * Tells the other agents on the open ticket (the socket) and, on a chat with a
   * contact, its visitor (the API, which checks the ticket) that a reply is being written.
   */
  typing(contactId?: string | null) {
    if (!this.active) return;
    const now = Date.now();
    if (now - this.lastTyping < 2500) return;
    this.lastTyping = now;
    const e = this.tickets.get(this.active);
    if (e?.mode !== "active") return;
    e.ch.push("typing", {});
    if (contactId) void api.tickets.typing(this.active).catch(() => {});
  }

  private reconcileTickets() {
    if (!this.socket) return;
    const desired = new Map<string, TicketMode>();
    for (const id of this.watched) desired.set(id, "watch");
    if (this.active) desired.set(this.active, "active");

    for (const [id, e] of this.tickets) {
      if (desired.get(id) !== e.mode || e.team !== this.privateTeamOf(id)) {
        e.ch.leave();
        this.tickets.delete(id);
        clearPresence(id);
        if (!desired.has(id)) clearLive(id);
      }
    }
    for (const [id, mode] of desired) {
      if (!this.tickets.has(id)) this.joinTicket(id, mode);
    }
  }

  private joinTicket(id: string, mode: TicketMode) {
    if (!this.socket) return;
    const topic = `ticket:${this.opts.workspaceId}:${id}`;
    const team = this.privateTeamOf(id);
    const params: Record<string, unknown> = mode === "watch" ? { watch: true } : { state: "viewing" };
    if (team) params.team_id = team;
    const ch = this.socket.channel(topic, params);
    const entry: TicketEntry = { ch, mode, state: {}, team };
    this.tickets.set(id, entry);
    ch.on("presence_state", (p) => {
      entry.state = syncState(entry.state, p as PresenceState);
      setPresence(id, entry.state);
    });
    ch.on("presence_diff", (p) => {
      entry.state = syncDiff(entry.state, p as PresenceDiff);
      setPresence(id, entry.state);
    });
    if (mode === "active") {
      ch.on("typing", (p) => {
        const t = p as { user_id: string; name: string };
        if (t.user_id !== this.opts.userId) noteTyping(id, t);
      });
      // The widget agent writing a reply: a preview only, replaced by message.created.
      ch.on("live", (p) => {
        const l = p as { type?: string; data?: Record<string, unknown> };
        if (typeof l?.type !== "string" || !l.data || typeof l.data !== "object") return;
        const tid = typeof l.data.ticket_id === "string" ? l.data.ticket_id : id;
        if (tid !== id) return;
        applyLive(id, l.type, l.data);
      });
      // contact_id comes from the visitor's token, so only the ticket's own contact counts.
      ch.on("customer_typing", (p) => {
        const contactId = (p as { contact_id?: string }).contact_id;
        const ticket = this.opts.qc.getQueryData<Ticket>(qk.ticket(id));
        if (contactId && ticket?.contact?.id === contactId) noteCustomerTyping(id);
      });
    }
    ch.join().receive("ok", () => {
      if (mode === "active") ch.push("presence", { state: "viewing" });
    });
  }
}

let current: RealtimeManager | null = null;

export function startRealtime(opts: ManagerOptions) {
  current?.stop();
  current = new RealtimeManager(opts);
  void current.start();
  return current;
}

export function stopRealtime() {
  current?.stop();
  current = null;
}

export function realtime(): RealtimeManager | null {
  return current;
}

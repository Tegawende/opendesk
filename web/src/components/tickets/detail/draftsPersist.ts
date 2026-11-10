// The composer's drafts outlive the page. Each
// ticket's draft is kept in localStorage under its workspace, user and ticket, written half a
// second after the last change, restored when the ticket opens, removed once sent or emptied,
// and forgotten after 14 days. Other tabs follow (the storage event). Files being attached
// are not drafts (they live in the composer only) and are not kept.
import type { StoreApi } from "zustand";

export const DRAFT_PREFIX = "od.draft.v1:";
export const DRAFT_TTL_MS = 14 * 24 * 60 * 60 * 1000;
export const DRAFT_WRITE_MS = 500;

type Stored<T> = { at: number; draft: T };

function storage(): Storage | null {
  try {
    return typeof window === "undefined" ? null : window.localStorage;
  } catch {
    return null;
  }
}

/** The storage key of a ticket's draft; `scope` is "<workspace>:<user>". */
export function draftKey(scope: string, ticketId: string): string {
  return `${DRAFT_PREFIX}${scope}:${ticketId}`;
}

/** A stored draft, or null when there is none, it is unreadable or older than 14 days (then removed). */
export function readDraft<T>(key: string, now = Date.now()): T | null {
  const st = storage();
  try {
    const raw = st?.getItem(key);
    if (!raw) return null;
    const s = JSON.parse(raw) as Stored<T> | null;
    if (!s || typeof s.at !== "number" || !s.draft || typeof s.draft !== "object" || now - s.at > DRAFT_TTL_MS) {
      st?.removeItem(key);
      return null;
    }
    return s.draft;
  } catch {
    try {
      st?.removeItem(key);
    } catch {
      // storage unavailable: nothing to clean
    }
    return null;
  }
}

/** Stores a draft, or removes it (null). Never throws: a full or blocked storage keeps it in memory only. */
export function writeDraft<T>(key: string, draft: T | null, now = Date.now()): void {
  try {
    const st = storage();
    if (!st) return;
    if (draft === null) st.removeItem(key);
    else st.setItem(key, JSON.stringify({ at: now, draft } satisfies Stored<T>));
  } catch {
    // quota or privacy mode: the draft stays in memory
  }
}

/** Removes every stored draft older than 14 days or unreadable, of any workspace or user. */
export function sweepDrafts(now = Date.now()): void {
  try {
    const st = storage();
    if (!st) return;
    const keys: string[] = [];
    for (let i = 0; i < st.length; i++) {
      const k = st.key(i);
      if (k?.startsWith(DRAFT_PREFIX)) keys.push(k);
    }
    keys.forEach((k) => readDraft(k, now));
  } catch {
    // storage unavailable
  }
}

/** Nothing worth keeping: no text or image, no address, no side conversation. */
export function isEmptyDraft(d: {
  html: string;
  cc?: string[];
  bcc?: string[];
  threadTo?: string[];
  threadCc?: string[];
  threadId?: string;
  threadForward?: string;
}): boolean {
  const text = d.html ? (new DOMParser().parseFromString(d.html, "text/html").body.textContent ?? "").replace(/\s/g, "") : "";
  const media = /<(img|video|iframe|hr)\b/i.test(d.html);
  return (
    !text &&
    !media &&
    !d.cc?.length &&
    !d.bcc?.length &&
    !d.threadTo?.length &&
    !d.threadCc?.length &&
    !d.threadId &&
    !d.threadForward
  );
}

export type DraftsBinding = {
  /** Loads the ticket's stored draft into the store, unless one is there already. */
  restore: (ticketId: string) => void;
  /** Writes the pending changes now (the page is going away). */
  flush: () => void;
  dispose: () => void;
};

/**
 * Keeps a store of drafts by ticket id in localStorage. `scope` is "<workspace>:<user>", or null
 * when nobody is signed in (nothing is read or written then).
 */
export function bindDrafts<T extends object>(
  store: StoreApi<Record<string, T>>,
  opts: { scope: () => string | null; empty: (d: T) => boolean },
): DraftsBinding {
  const pending = new Map<string, { key: string; timer: ReturnType<typeof setTimeout> }>();
  let prev = store.getState();
  let applying = false;

  const write = (tid: string, key: string) => {
    pending.delete(tid);
    const d = store.getState()[tid];
    writeDraft(key, d && !opts.empty(d) ? d : null);
  };
  const schedule = (tid: string) => {
    const scope = opts.scope();
    if (!scope) return;
    const old = pending.get(tid);
    if (old) clearTimeout(old.timer);
    const key = draftKey(scope, tid);
    pending.set(tid, { key, timer: setTimeout(() => write(tid, key), DRAFT_WRITE_MS) });
  };
  const apply = (fn: () => void) => {
    applying = true;
    try {
      fn();
    } finally {
      applying = false;
      prev = store.getState();
    }
  };

  const unsubscribe = store.subscribe((state) => {
    if (applying) return;
    for (const tid of new Set([...Object.keys(state), ...Object.keys(prev)])) {
      if (state[tid] !== prev[tid]) schedule(tid);
    }
    prev = state;
  });

  // Another tab wrote or removed a draft of mine: follow it, unless I am typing in this one.
  const onStorage = (e: StorageEvent) => {
    const scope = opts.scope();
    if (!e.key || !scope) return;
    const prefix = draftKey(scope, "");
    if (!e.key.startsWith(prefix)) return;
    const tid = e.key.slice(prefix.length);
    if (!tid || pending.has(tid)) return;
    const d = e.newValue ? readDraft<T>(e.key) : null;
    apply(() =>
      store.setState((s) => {
        if (d) return { ...s, [tid]: d };
        const rest = { ...s };
        delete rest[tid];
        return rest;
      }, true),
    );
  };
  const flush = () => {
    for (const [tid, p] of [...pending]) {
      clearTimeout(p.timer);
      write(tid, p.key);
    }
  };

  if (typeof window !== "undefined") {
    window.addEventListener("storage", onStorage);
    window.addEventListener("pagehide", flush);
  }
  sweepDrafts();

  return {
    restore: (tid) => {
      const scope = opts.scope();
      if (!scope || store.getState()[tid]) return;
      const d = readDraft<T>(draftKey(scope, tid));
      if (d) apply(() => store.setState({ [tid]: d }));
    },
    flush,
    dispose: () => {
      unsubscribe();
      pending.forEach((p) => clearTimeout(p.timer));
      pending.clear();
      if (typeof window !== "undefined") {
        window.removeEventListener("storage", onStorage);
        window.removeEventListener("pagehide", flush);
      }
    },
  };
}

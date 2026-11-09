// Drafts kept in localStorage.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { create } from "zustand";
import { bindDrafts, DRAFT_TTL_MS, draftKey, isEmptyDraft, readDraft, sweepDrafts, writeDraft, type DraftsBinding } from "./draftsPersist";
import composer from "./Composer.tsx?raw";

type D = { html: string; cc: string[]; mode: string };
const d = (html: string, extra: Partial<D> = {}): D => ({ html, cc: [], mode: "reply", ...extra });

let scope: string | null = "w1:u1";
let binding: DraftsBinding | null = null;
const make = () => {
  const store = create<Record<string, D>>(() => ({}));
  binding = bindDrafts(store, { scope: () => scope, empty: isEmptyDraft });
  return store;
};
const stored = (tid: string, s = "w1:u1") => readDraft<D>(draftKey(s, tid));

beforeEach(() => {
  vi.useFakeTimers();
  localStorage.clear();
  scope = "w1:u1";
});

afterEach(() => {
  binding?.dispose();
  binding = null;
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("drafts kept across reloads", () => {
  it("writes half a second after the last change, per workspace, user and ticket", () => {
    const store = make();
    store.setState({ t1: d("<p>Hel</p>") });
    store.setState({ t1: d("<p>Hello</p>") });
    vi.advanceTimersByTime(400);
    expect(stored("t1")).toBeNull();
    vi.advanceTimersByTime(100);
    expect(stored("t1")).toEqual(d("<p>Hello</p>"));
    expect(localStorage.getItem(draftKey("w1:u1", "t1"))).toContain('"at"');
    expect(stored("t1", "w1:u2")).toBeNull();
  });

  it("restores a ticket's draft when it opens, and leaves one being written alone", () => {
    writeDraft(draftKey("w1:u1", "t1"), d("<p>Kept</p>", { cc: ["a@b.c"], mode: "note" }));
    const store = make();
    binding!.restore("t1");
    expect(store.getState().t1).toEqual(d("<p>Kept</p>", { cc: ["a@b.c"], mode: "note" }));
    // Restoring does not write it again.
    const set = vi.spyOn(Storage.prototype, "setItem");
    vi.advanceTimersByTime(1000);
    expect(set).not.toHaveBeenCalled();
    store.setState({ t1: d("<p>Newer</p>") });
    binding!.restore("t1");
    expect(store.getState().t1.html).toBe("<p>Newer</p>");
  });

  it("removes it once sent or emptied", () => {
    const store = make();
    store.setState({ t1: d("<p>Hello</p>") });
    vi.advanceTimersByTime(500);
    store.setState({ t1: d("<p><br></p>") });
    vi.advanceTimersByTime(500);
    expect(localStorage.getItem(draftKey("w1:u1", "t1"))).toBeNull();
  });

  it("forgets drafts after 14 days and unreadable ones", () => {
    const now = Date.now();
    writeDraft(draftKey("w1:u1", "old"), d("<p>Old</p>"), now - DRAFT_TTL_MS - 1);
    writeDraft(draftKey("w2:u9", "other"), d("<p>Other</p>"), now - DRAFT_TTL_MS - 1);
    writeDraft(draftKey("w1:u1", "new"), d("<p>New</p>"), now - DRAFT_TTL_MS + 60_000);
    localStorage.setItem(draftKey("w1:u1", "bad"), "{not json");
    sweepDrafts(now);
    expect(Object.keys(localStorage).sort()).toEqual([draftKey("w1:u1", "new")]);
  });

  it("never throws when storage is blocked or full", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("full", "QuotaExceededError");
    });
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    const store = make();
    store.setState({ t1: d("<p>Hello</p>") });
    expect(() => vi.advanceTimersByTime(500)).not.toThrow();
    expect(() => binding!.restore("t2")).not.toThrow();
    expect(store.getState().t1.html).toBe("<p>Hello</p>");
  });

  it("reads and writes nothing while nobody is signed in", () => {
    scope = null;
    const store = make();
    store.setState({ t1: d("<p>Hello</p>") });
    vi.advanceTimersByTime(500);
    expect(localStorage.length).toBe(0);
  });

  it("follows another tab, without writing it back, unless this tab is typing", () => {
    const store = make();
    const key = draftKey("w1:u1", "t1");
    const other = JSON.stringify({ at: Date.now(), draft: d("<p>From tab B</p>") });
    localStorage.setItem(key, other);
    const set = vi.spyOn(Storage.prototype, "setItem");
    window.dispatchEvent(new StorageEvent("storage", { key, newValue: other }));
    expect(store.getState().t1.html).toBe("<p>From tab B</p>");
    vi.advanceTimersByTime(1000);
    expect(set).not.toHaveBeenCalled();
    // Sent in the other tab: gone here too.
    localStorage.removeItem(key);
    window.dispatchEvent(new StorageEvent("storage", { key, newValue: null }));
    expect(store.getState().t1).toBeUndefined();
    // Another user's or workspace's drafts are not mine.
    window.dispatchEvent(new StorageEvent("storage", { key: draftKey("w1:u2", "t1"), newValue: other }));
    expect(store.getState().t1).toBeUndefined();
    // Typing here wins over a write from elsewhere.
    store.setState({ t1: d("<p>Mine</p>") });
    window.dispatchEvent(new StorageEvent("storage", { key, newValue: other }));
    expect(store.getState().t1.html).toBe("<p>Mine</p>");
  });

  it("writes what is pending when the page goes away", () => {
    const store = make();
    store.setState({ t1: d("<p>Last words</p>") });
    window.dispatchEvent(new Event("pagehide"));
    expect(stored("t1")?.html).toBe("<p>Last words</p>");
  });
});

describe("isEmptyDraft", () => {
  it("keeps text, images, addresses and side conversations", () => {
    expect(isEmptyDraft({ html: "<p> &nbsp;</p><p><br></p>" })).toBe(true);
    expect(isEmptyDraft({ html: "<p>x</p>" })).toBe(false);
    expect(isEmptyDraft({ html: '<p><img src="a.png"></p>' })).toBe(false);
    expect(isEmptyDraft({ html: "", cc: ["a@b.c"] })).toBe(false);
    expect(isEmptyDraft({ html: "", threadTo: ["a@b.c"] })).toBe(false);
    expect(isEmptyDraft({ html: "", threadForward: "m1" })).toBe(false);
  });

  it("is how the composer keeps its drafts", () => {
    expect(composer).toContain("const keptDrafts = bindDrafts(useDrafts, {");
    expect(composer).toContain("useEffect(() => keptDrafts.restore(ticket.id), [ticket.id]);");
  });
});

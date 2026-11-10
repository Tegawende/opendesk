// A session without a contact (the visitor has not written yet) opens no socket and lists
// nothing; the first message makes the contact, and the widget then joins its socket and shows
// the reply.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Controller } from "./controller";
import { createMock, demoConfig } from "./mock";

describe("a session without a contact", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("joins the socket once the first message made the contact", async () => {
    const mock = createMock({ seed: "empty", latency: 5, lazyContact: true });
    const sockets = vi.fn(mock.socket);
    const c = new Controller({ base: "https://mock.invalid", key: "wk_demo", fetcher: mock.fetch, socketFactory: sockets, persist: false });
    const boot = c.boot(demoConfig("wk_demo"));
    await vi.advanceTimersByTimeAsync(200);
    await boot;
    expect(c.store.get().session?.contact).toBeNull();
    expect(c.store.get().conversations).toEqual([]);
    expect(sockets).not.toHaveBeenCalled();

    c.newConversation();
    const send = c.send("Where is my order?", []);
    await vi.advanceTimersByTimeAsync(200);
    await send;
    expect(c.store.get().session?.contact?.id).toBeTruthy();
    expect(sockets).toHaveBeenCalled();
    const id = c.store.get().active!;
    await vi.advanceTimersByTimeAsync(8000);
    const msgs = c.store.get().messages[id];
    expect(msgs.length).toBeGreaterThan(1);
    expect(msgs[msgs.length - 1].direction).toBe("outbound");
    c.stop();
    mock.dispose();
  });
});

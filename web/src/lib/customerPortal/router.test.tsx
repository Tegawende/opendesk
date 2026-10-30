import { afterEach, expect, it, vi } from "vitest";

afterEach(() => { history.replaceState(null, "", "/"); vi.restoreAllMocks(); });

it("retains a reset token across actual router loads without rewriting history again", async () => {
  history.replaceState(null, "", "/portal/acme/reset#token=secret");
  const replace = vi.spyOn(history, "replaceState");
  vi.resetModules();
  const { router } = await import("@/router");
  await router.load();
  expect(location.hash).toBe("");
  expect(router.state.matches.find((m) => m.routeId === "/portal/$slug/reset")?.context).toMatchObject({ portalResetToken: "secret" });
  const count = replace.mock.calls.length;
  await router.load();
  expect(replace.mock.calls.length).toBe(count);
  expect(router.state.matches.find((m) => m.routeId === "/portal/$slug/reset")?.context).toMatchObject({ portalResetToken: "secret" });
});

it("accepts legacy query tokens before the router subscribes", async () => {
  history.replaceState(null, "", "/portal/acme/reset?token=legacy&lang=fr");
  vi.resetModules();
  const { router } = await import("@/router");
  await router.load();
  expect(location.search).toBe("?lang=fr");
  expect(router.state.matches.find((m) => m.routeId === "/portal/$slug/reset")?.context).toMatchObject({ portalResetToken: "legacy" });
});

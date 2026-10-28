import { act } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { api, rows } from "../api/endpoints";
import { setWorkspaceId } from "../api/client";
import { qk } from "../queries";
import { installMock } from "@/mock";
import { db } from "@/mock/db";
import { useThreadAction } from "./hooks";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
it("updates inbox/sidebar together, refreshes overview without realtime, and rolls back a refusal", async () => {
  installMock();
  setWorkspaceId(db.workspaces[0].workspace.id);
  const list = await api.collab.threads.list({ box: "inbox" });
  const thread = list.data.find((t) => t.unread);
  if (!thread) throw new Error("Missing unread fixture");
  const detail = await api.collab.threads.get(thread.id);
  const counts = await api.collab.counts();
  const qc = new QueryClient();
  qc.setQueryData(qk.mailFolders(thread.account_id), rows(await api.collab.accounts.folders(thread.account_id)));
  qc.setQueryData(qk.mailThreadLists, { pages: [list], pageParams: [null] });
  qc.setQueryData(qk.mailThread(thread.id), detail);
  qc.setQueryData(qk.mailCounts, counts);
  qc.setQueryData(qk.overview, { workspaces: [] });
  let run: ReturnType<typeof useThreadAction>["mutateAsync"] | undefined;
  function View() { run = useThreadAction().mutateAsync; return null; }
  const root = createRoot(document.createElement("div"));
  await act(async () => root.render(<QueryClientProvider client={qc}><View /></QueryClientProvider>));
  const refused = vi.spyOn(api.collab.threads, "action").mockRejectedValueOnce(new Error("Refused"));
  try {
    await act(async () => { await expect(run?.({ ids: [thread.id], action: "read" })).rejects.toThrow("Refused"); });
    expect(qc.getQueryData(qk.mailThread(thread.id))).toEqual(detail);
    expect(qc.getQueryData(qk.mailCounts)).toEqual(counts);
    expect(qc.getQueryState(qk.overview)?.isInvalidated).toBe(true);
    refused.mockRestore();
    await act(async () => { await run?.({ ids: [thread.id], action: "read" }); });
    expect(qc.getQueryData<typeof detail>(qk.mailThread(thread.id))?.thread).toMatchObject({ unread: false, needs_action: thread.needs_action });
    expect(qc.getQueryData<typeof counts>(qk.mailCounts)?.boxes.inbox).toBe(counts.boxes.inbox - 1);
    expect(qc.getQueryData<typeof counts>(qk.mailCounts)?.accounts[thread.account_id]).toBe(counts.accounts[thread.account_id] - 1);
    expect(qc.getQueryData<{ pages: typeof list[] }>(qk.mailThreadLists)?.pages[0].data.find((t) => t.id === thread.id)?.unread).toBe(false);
  } finally {
    refused.mockRestore();
    await act(async () => root.unmount());
    qc.clear();
    setWorkspaceId(null);
  }
});

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useReadOnOpen } from "./useReadOnOpen";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
let root: Root;
const onAction = vi.fn();
type Props = Parameters<typeof useReadOnOpen>[0];
const detail = { can_act: true, thread: { id: "t1", unread: true }, messages: [{ id: "m1", draft: false }] };
function View(props: Props) { useReadOnOpen(props); return null; }
async function show(over: Partial<Props> = {}) {
  await act(async () => root.render(<View detail={detail} current enabled onAction={onAction} {...over} />));
}
beforeEach(() => { onAction.mockClear(); root = createRoot(document.createElement("div")); });
afterEach(async () => { await act(async () => root.unmount()); });

it("reads the first delivery and new mail shown in the same conversation", async () => {
  await show();
  expect(onAction).toHaveBeenCalledExactlyOnceWith("read", { silent: true });
  await show({ detail: { ...detail, thread: { ...detail.thread, unread: false } } });
  await show({ detail: { ...detail, messages: [...detail.messages, { id: "m2", draft: false }] } });
  expect(onAction).toHaveBeenCalledTimes(2);
});
it("leaves explicit unread alone, including after saving a draft", async () => {
  await show();
  await show({ detail: { ...detail, thread: { ...detail.thread, unread: false } } });
  await show();
  await show({ detail: { ...detail, messages: [...detail.messages, { id: "draft", draft: true }] } });
  expect(onAction).toHaveBeenCalledTimes(1);
});
it("waits for current data and respects the preference and read-only access", async () => {
  await show({ current: false });
  await show({ enabled: false });
  await show({ detail: { ...detail, can_act: false } });
  expect(onAction).not.toHaveBeenCalled();
  await show();
  expect(onAction).toHaveBeenCalledTimes(1);
});
it("does not retry a rejected read in a loop, and tries on a new visit", async () => {
  await show();
  await show({ detail: { ...detail } });
  expect(onAction).toHaveBeenCalledTimes(1);
  await act(async () => root.render(null));
  await show();
  expect(onAction).toHaveBeenCalledTimes(2);
});
it("waits past a read cached copy when reopening an unread conversation", async () => {
  await show({ current: false, detail: { ...detail, thread: { ...detail.thread, unread: false } } });
  expect(onAction).not.toHaveBeenCalled();
  await show();
  expect(onAction).toHaveBeenCalledExactlyOnceWith("read", { silent: true });
});

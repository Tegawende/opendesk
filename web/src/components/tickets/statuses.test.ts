import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { q } from "@/lib/queries";
import { afterEach, describe, expect, it } from "vitest";
import { hideableStatus } from "@/components/settings2/DisplayOverrides";
import { i18n, loadAgentCatalogs } from "@/i18n";
import { useDisplay, finishVerb, finishWords, offeredStatuses } from "./fields";

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

describe("hidden statuses", () => {
  const hidden = { pending: { hidden: true }, on_hold: { hidden: true }, solved: { hidden: true }, closed: { label: "Done" } };

  it("offers every status until the workspace hides some", () => {
    expect(offeredStatuses(undefined)).toEqual(["open", "pending", "on_hold", "solved", "closed"]);
    expect(offeredStatuses(hidden)).toEqual(["open", "solved", "closed"]);
  });

  it("keeps the ticket's current status in its menu, even hidden", () => {
    expect(offeredStatuses(hidden, "on_hold")).toEqual(["open", "on_hold", "solved", "closed"]);
  });

  it("Finish selects solved with hidden statuses or a Closed presentation label", async () => {
    for (const labels of [hidden, { solved: { label: "Closed" }, closed: { hidden: true } }]) {
      const client = new QueryClient();
      client.setQueryData(q.workspace().queryKey, { settings: { status_labels: labels } });
      const host = document.createElement("div");
      const root = createRoot(host);
      function Finish() {
        const display = useDisplay();
        return createElement("button", { "data-status": display.finish }, display.status(display.finish));
      }
      await act(async () => root.render(createElement(QueryClientProvider, { client }, createElement(Finish))));
      expect(host.querySelector("button")?.dataset.status).toBe("solved");
      await act(async () => root.unmount());
      client.clear();
    }
  });

  it("never lets open or solved be hidden", () => {
    expect(hideableStatus("open", {})).toBe("never");
    expect(hideableStatus("pending", {})).toBe(true);
    expect(hideableStatus("closed", { solved: { hidden: true } })).toBe(true);
    expect(hideableStatus("solved", { closed: { hidden: true } })).toBe("never");
    expect(hideableStatus("solved", {})).toBe("never");
  });
});

describe("the finishing action's words", () => {
  afterEach(async () => {
    await i18n.changeLanguage("en");
  });
  type Labels = Parameters<typeof finishWords>[0];
  const prod = { solved: { label: "Closed" }, pending: { hidden: true }, closed: { hidden: true } };

  it("say solve while solved is offered, renamed or not", () => {
    for (const labels of [undefined, { pending: { hidden: true } }, { solved: { label: "Solved" } }] as Labels[]) {
      const w = finishWords(labels);
      expect(w.verb).toBe("solve");
      expect(w.button()).toBe("Solve");
      expect(w.command()).toBe("Solve ticket");
      expect(w.toast(42)).toBe("Solved #42");
      expect(w.quick(42)).toBe("Solve #42");
      expect(w.tip()).toBe("Solve the focused ticket");
      expect(w.sendTip()).toBe("Send and solve");
      expect(w.listShortcut()).toBe("Solve focused ticket");
      expect(w.shortcut()).toBe("Solve");
    }
  });

  it("say close when the semantic finish status is labelled Closed", () => {
    for (const labels of [prod, { solved: { label: " closed " }, closed: { hidden: true } }] as Labels[]) {
      const w = finishWords(labels);
      expect(w.verb).toBe("close");
      expect(w.button()).toBe("Close");
      expect(w.command()).toBe("Close ticket");
      expect(w.toast(42)).toBe("Closed #42");
      expect(w.quick(42)).toBe("Close #42");
      expect(w.tip()).toBe("Close the focused ticket");
      expect(w.sendTip()).toBe("Send and close");
      expect(w.listShortcut()).toBe("Close focused ticket");
      expect(w.shortcut()).toBe("Close");
    }
  });

  it("mark as the workspace's own finish label", () => {
    const w = finishWords({ solved: { label: "Done" }, closed: { hidden: true } });
    expect(w.verb).toBe("mark");
    expect(w.button()).toBe("Mark as Done");
    expect(w.toast(42)).toBe("Marked #42 as Done");
    expect(w.sendTip()).toBe("Send and mark as Done");
  });

  it("read an English label as closed's own name for a French agent", async () => {
    await loadAgentCatalogs("fr");
    await i18n.changeLanguage("fr");
    expect(finishVerb(prod)).toBe("close");
    expect(finishWords(prod).button()).toBe("Fermer");
    expect(finishWords(prod).toast(42)).toBe("#42 fermé");
    expect(finishWords(undefined).button()).toBe("Résoudre");
  });
});

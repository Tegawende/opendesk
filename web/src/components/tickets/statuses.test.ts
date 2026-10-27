import { afterEach, describe, expect, it } from "vitest";
import { hideableStatus } from "@/components/settings2/DisplayOverrides";
import { i18n, loadAgentCatalogs } from "@/i18n";
import { finishStatus, finishVerb, finishWords, offeredStatuses } from "./fields";

describe("hidden statuses", () => {
  const hidden = { pending: { hidden: true }, on_hold: { hidden: true }, solved: { hidden: true }, closed: { label: "Done" } };

  it("offers every status until the workspace hides some", () => {
    expect(offeredStatuses(undefined)).toEqual(["open", "pending", "on_hold", "solved", "closed"]);
    expect(offeredStatuses(hidden)).toEqual(["open", "closed"]);
  });

  it("keeps the ticket's current status in its menu, even hidden", () => {
    expect(offeredStatuses(hidden, "on_hold")).toEqual(["open", "on_hold", "closed"]);
  });

  it("solves to closed where solved is hidden", () => {
    expect(finishStatus(undefined)).toBe("solved");
    expect(finishStatus({ pending: { hidden: true } })).toBe("solved");
    expect(finishStatus(hidden)).toBe("closed");
  });

  it("never lets open, or both solved and closed, be hidden", () => {
    expect(hideableStatus("open", {})).toBe("never");
    expect(hideableStatus("pending", {})).toBe(true);
    expect(hideableStatus("closed", { solved: { hidden: true } })).toBe(false);
    expect(hideableStatus("solved", { closed: { hidden: true } })).toBe(false);
    expect(hideableStatus("solved", {})).toBe(true);
  });
});

describe("the finishing action's words", () => {
  afterEach(async () => {
    await i18n.changeLanguage("en");
  });
  type Labels = Parameters<typeof finishWords>[0];
  const prod = { solved: { label: "Resolved", hidden: true }, pending: { hidden: true }, closed: { label: "Closed" } };

  it("say solve while solved is offered, renamed or not", () => {
    for (const labels of [undefined, { pending: { hidden: true } }, { solved: { label: "Resolved" } }] as Labels[]) {
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

  it("say close where solved is hidden and closed keeps its name", () => {
    for (const labels of [prod, { solved: { hidden: true } }, { solved: { hidden: true }, closed: { label: " closed " } }] as Labels[]) {
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

  it("mark as the workspace's own name for closed when it renamed it", () => {
    const w = finishWords({ solved: { hidden: true }, closed: { label: "Done" } });
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

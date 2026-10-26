import { describe, expect, it } from "vitest";
import { hideableStatus } from "@/components/settings2/DisplayOverrides";
import { finishStatus, offeredStatuses } from "./fields";

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

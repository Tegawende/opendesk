import type { Page } from "@playwright/test";
import { expect, test } from "../lib/proof.ts";

// Maya is in Margin and Lumen Labs (web/src/mock); Ollie's tips stay out of the way.
async function open(page: Page, proof: { open: (path: string, params?: Record<string, string>) => Promise<unknown> }, path: string, params?: Record<string, string>) {
  await page.addInitScript(() => {
    localStorage.setItem("od-ollie-tips", JSON.stringify({ paused: Number.MAX_SAFE_INTEGER }));
  });
  await proof.open(path, params);
}

test("every workspace in one sidebar, grouped, and opening another one without a reload", async ({ page, proof }) => {
  await open(page, proof, "/inbox/unassigned");
  await expect(page.getByText("Everywhere", { exact: true })).toBeVisible();
  await expect(page.getByText("Lumen Labs").first()).toBeVisible();
  await proof.chapter("Together", "Every workspace in one sidebar, each under its own badge and name");
  await proof.dwell();
  await proof.shot("grouped", { caption: "Grouped: Everywhere on top, then the open workspace, Margin, with its full sidebar" });

  const lumen = page.getByRole("button", { name: "Fold Lumen Labs" });
  await lumen.scrollIntoViewIfNeeded();
  await expect(lumen).toBeInViewport();
  await proof.dwell();
  await proof.shot("lumen-group", { caption: "Further down, the Lumen Labs group with its own folders, team, view and mail accounts" });

  await page.getByRole("navigation", { name: "Main" }).getByRole("button", { name: "Mine 3" }).click();
  await expect(page.getByRole("button", { name: "Lumen Labs" }).first()).toBeVisible();
  await expect(page.getByRole("heading", { name: "Mine", level: 1 })).toBeVisible();
  await proof.chapter("Open Lumen Labs", "Its Mine folder opens at once, with nothing reloaded");
  await proof.dwell();
  await proof.shot("lumen-mine", { caption: "Lumen Labs' Mine folder, opened from the shared sidebar" });
});

test("Assigned to me across every workspace, each ticket marked with its workspace", async ({ page, proof }) => {
  await open(page, proof, "/everywhere/mine");
  await expect(page.getByRole("heading", { name: "Assigned to me everywhere" })).toBeVisible();
  await expect(page.getByRole("img", { name: "In Lumen Labs" }).first()).toBeVisible();
  await proof.chapter("Everywhere", "Tickets assigned to you in every workspace, newest activity first");
  await proof.dwell();
  await proof.shot("everywhere-mine", { caption: "Assigned to me everywhere: Margin and Lumen Labs tickets together, each with its badge" });
});

test("mixed layout combines the sidebar, each row marked with its workspace", async ({ page, proof }) => {
  await open(page, proof, "/inbox/unassigned?mock_together=mixed");
  await expect(page.getByRole("button", { name: "Mine, In Lumen Labs" })).toBeVisible();
  await proof.chapter("Mixed", "Inbox, teams, views and mail combined, each row led by its workspace badge");
  await proof.dwell();
  await proof.shot("mixed", { caption: "Mixed: one combined sidebar, each row led by its workspace badge" });
});

test("choose how workspaces share the sidebar", async ({ page, proof }) => {
  await open(page, proof, "/settings/workspaces");
  await expect(page.getByText("Show workspaces")).toBeVisible();
  await proof.chapter("Workspaces settings", "Together or separate, grouped or mixed, the Everywhere lists, order and what each shows");
  await proof.dwell();
  await proof.shot("settings", { caption: "Settings, Workspaces: Together, Grouped, Everywhere lists, and the order of workspaces" });

  await page.getByRole("radio", { name: /Separate/ }).click();
  await expect(page.getByText("Everywhere", { exact: true })).toHaveCount(0);
  await proof.dwell();
  await proof.shot("separate", { caption: "Separate: the sidebar shows one workspace at a time, as before" });
});

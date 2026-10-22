import type { Page } from "@playwright/test";
import { expect, test } from "../lib/proof.ts";

async function openNewsletter(page: Page) {
  await page.getByRole("option", { name: /Wednesday Web Drop/ }).click();
  await expect(page.getByRole("heading", { name: "Wednesday Web Drop" })).toBeVisible();
  await page.getByRole("button", { name: "Not now" }).click();
}

async function openNewsletterAgain(page: Page) {
  await page.getByRole("option", { name: /Wednesday Web Drop/ }).click();
  await expect(page.getByRole("heading", { name: "Wednesday Web Drop" })).toBeVisible();
}

test("comment on a long email without scrolling to the end", async ({ page, proof }) => {
  await proof.open("/mail");
  await openNewsletter(page);
  const field = page.getByRole("textbox", { name: "Comment, @ to mention a teammate" });
  await expect(field).toBeInViewport();
  await proof.chapter("Pinned comments", "Writing a comment and replying stay under the conversation, however long the mail is");
  await proof.shot("pinned", { caption: "A long newsletter: the comment field, the 2 comments and the replies are pinned at the bottom" });

  await field.click();
  await field.pressSequentially("Plane is in Leads now, I'll reach out on Friday.", { delay: 40 });
  await expect(page.getByRole("button", { name: "Comment", exact: true })).toBeEnabled();
  await proof.shot("writing", { caption: "Writing opens the editor in place, with who reads it" });

  await page.keyboard.press("Control+Enter");
  const posted = page.getByRole("region", { name: "Team comments" }).getByText("Plane is in Leads now");
  await expect(posted).toBeInViewport();
  await proof.dwell();
  await proof.shot("posted", { caption: "Posted: the conversation scrolls to the new comment" });
});

test("jump to the comments and reply from the pinned bar", async ({ page, proof }) => {
  await proof.open("/mail");
  await openNewsletter(page);
  await proof.chapter("Shortcuts", "Shift C comments, the count jumps to the discussion, Reply stays a click away");

  await page.keyboard.press("Shift+C");
  await expect(page.getByRole("textbox", { name: "Comment, @ to mention a teammate" })).toBeFocused();
  await proof.shot("shortcut", { caption: "Shift C starts a comment from anywhere in the conversation" });
  await page.keyboard.press("Escape");

  await page.getByRole("button", { name: "2 comments" }).click();
  await expect(page.getByRole("region", { name: "Team comments" })).toBeInViewport();
  await proof.dwell();
  await proof.shot("comments", { caption: "The count jumps to the team's discussion" });

  await page.getByRole("button", { name: "Reply", exact: true }).click();
  await expect(page.getByRole("button", { name: "Send" }).first()).toBeVisible();
  await proof.dwell();
  await proof.shot("replying", { caption: "Replying: the reply buttons step aside, the comment field stays" });
});

test("an unsent comment waits for its conversation", async ({ page, proof }) => {
  await proof.open("/mail");
  await openNewsletter(page);
  await proof.chapter("Unsent comments", "A comment you started stays with its conversation while you read another");
  const field = page.getByRole("textbox", { name: "Comment, @ to mention a teammate" });
  await field.click();
  await field.pressSequentially("Check whether Quo has an API", { delay: 40 });

  await page.getByRole("option", { name: /Q4 renewal and seat count/ }).first().click();
  await expect(page.getByRole("heading", { name: /Q4 renewal and seat count/ })).toBeVisible();
  await expect(field).toHaveText("");
  await proof.dwell();
  await proof.shot("other-conversation", { caption: "Another conversation: its own empty comment field" });

  await openNewsletterAgain(page);
  await expect(field).toHaveText("Check whether Quo has an API");
  await proof.dwell();
  await proof.shot("unsent-kept", { caption: "Back on the newsletter: the comment is still there, ready to post" });
});

test("Cc in a forward opens its own field", async ({ page, proof }) => {
  await proof.open("/mail");
  await openNewsletter(page);
  await page.getByRole("button", { name: "Forward", exact: true }).click();
  const to = page.getByRole("combobox", { name: "To" });
  await expect(to).toBeVisible();
  await proof.chapter("Cc", "Adding Cc puts the cursor in Cc, and the To suggestions stay closed");
  await page.getByRole("button", { name: "Cc", exact: true }).click();
  await expect(page.getByRole("combobox", { name: "Cc" })).toBeFocused();
  await expect(to).toHaveAttribute("aria-expanded", "false");
  await proof.dwell();
  await proof.shot("forward-cc", { caption: "Forwarding: Cc added with the cursor in it and its own suggestions, To untouched" });
});

test("a toast's button stays on one line", async ({ page, proof }) => {
  await proof.open("/mail");
  await expect(page.getByRole("option", { name: /Wednesday Web Drop/ })).toBeVisible();
  // The toast a mention raises, with a long description beside its button.
  // The app's own toast module, loaded from the dev server inside the page.
  await page.evaluate(async (path) => {
    const { toast } = (await import(path)) as { toast: { info: (msg: string, opts: object) => void } };
    toast.info("You were mentioned #1034", { description: "Can guests comment without an account?", action: { label: "Open", onClick: () => undefined } });
  }, "/src/components/ui/toast.tsx");
  const toastEl = page.locator("[data-sonner-toast]").first();
  await expect(toastEl.getByRole("button", { name: "Open", exact: true })).toBeVisible();
  await proof.dwell();
  await proof.shot("toast-open", { caption: "A mention toast: the description wraps, Open stays whole", target: toastEl });
});

test("scroll the folder tabs beside an open conversation", async ({ page, proof }) => {
  await proof.open("/mail");
  await openNewsletter(page);
  const tabs = page.getByRole("navigation", { name: "Folders" });
  const right = tabs.getByRole("button", { name: "Scroll tabs right" });
  await expect(right).toBeVisible();
  await proof.chapter("Folder tabs", "The arrows scroll the tabs and stay where they went");
  await proof.shot("tabs-start", { caption: "A narrow list: more tabs to the right", target: tabs });

  await right.click();
  await expect(tabs.getByRole("link", { name: /Archive/ })).toBeInViewport();
  await proof.dwell();
  await proof.shot("tabs-right", { caption: "Scrolled right: Drafts and Archive in view, the open Inbox tab does not pull it back", target: tabs });

  await tabs.getByRole("button", { name: "Scroll tabs left" }).click();
  await expect(tabs.getByRole("link", { name: /Inbox/ })).toBeInViewport();
  await proof.dwell();
  await proof.shot("tabs-back", { caption: "And back to the start", target: tabs });
});

test.describe("dark", () => {
  test.use({ colorScheme: "dark" });

  test("the pinned comment bar in the dark theme", async ({ page, proof }) => {
    // The app is white first: dark is the member's own choice.
    await page.addInitScript(() => localStorage.setItem("od-theme", "dark"));
    await proof.open("/mail");
    await openNewsletter(page);
    await expect(page.getByRole("textbox", { name: "Comment, @ to mention a teammate" })).toBeInViewport();
    await proof.shot("pinned-dark", { caption: "The pinned bar in the dark theme" });
  });
});

test("new mail in your own account notifies you, and you can mute it per account", async ({ page, proof }) => {
  await page.addInitScript(() => localStorage.setItem("od-ollie-tips", JSON.stringify({ paused: Number.MAX_SAFE_INTEGER })));
  await proof.open("/inbox/unassigned");
  await page.getByRole("button", { name: /^Notifications/ }).click();
  const note = page.getByRole("button", { name: /New mail from/ }).first();
  await expect(note).toBeVisible();
  await proof.chapter("New mail", "Mail that lands in your own account's inbox shows under the bell");
  await proof.dwell();
  await proof.shot("bell", { caption: "New mail in the Work account, in the notifications panel" });

  await note.click();
  await expect(page).toHaveURL(/\/mail\//);
  await proof.dwell();
  await proof.shot("opened", { caption: "It opens the conversation in Mail" });

  await proof.open("/settings/mail-accounts");
  await page.getByText("Work", { exact: true }).first().click();
  const toggle = page.getByRole("switch", { name: "New mail" });
  await expect(toggle).toBeChecked();
  await toggle.scrollIntoViewIfNeeded();
  await proof.chapter("Mute an account", "Choose per account whether its new mail reaches you");
  await toggle.click();
  await expect(page.getByText("New mail in Work is muted for you")).toBeVisible();
  await expect(toggle).not.toBeChecked();
  await proof.dwell();
  await proof.shot("muted", { caption: "New mail in Work muted for you alone" });
});

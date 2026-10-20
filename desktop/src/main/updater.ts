// Auto-update from GitHub Releases (electron-updater, generic provider on the
// rolling desktop-latest release), checked at launch and every six hours.
// Installed builds only. The state is mirrored to the page
// (`window.opendeskDesktop.update`, contracts/native.md), which shows the
// "Update ready" banner.
//
// Channels: stable reads latest*.yml, beta reads beta*.yml. A stable release
// also writes the beta files, so beta users get every stable version newer than
// their beta. Downgrades are never allowed: someone who leaves the beta keeps
// their beta version until a stable version passes it.

import { app, dialog, Notification, shell } from "electron";
import { autoUpdater, type ProgressInfo, type UpdateInfo } from "electron-updater";
import { devArg, DOWNLOAD_URL, isDev, RELEASE_NOTES_URL, UPDATE_INTERVAL_MS } from "./env";

/**
 * The app installs updates itself only where electron-updater checks the update's code
 * signature against the running app's (macOS, Windows). Linux packages carry no such
 * signature, so there people install a new version from the download page.
 */
export const selfUpdates = process.platform === "darwin" || process.platform === "win32";
import { store, type UpdateChannel } from "./store";

export type UpdateState =
  | { status: "idle" }
  | { status: "checking" }
  | { status: "available"; version: string }
  | { status: "downloading"; version: string; percent: number }
  | { status: "ready"; version: string; notes_url: string }
  | { status: "error" };

let state: UpdateState = { status: "idle" };
let started = false;
let onStateChange: ((s: UpdateState, statusChanged: boolean) => void) | null = null;

function set(next: UpdateState) {
  const statusChanged = next.status !== state.status;
  state = next;
  onStateChange?.(state, statusChanged);
}

export function updateState(): UpdateState {
  return state;
}

export function updateReady() {
  return state.status === "ready";
}

export function updateChannel(): UpdateChannel {
  return store.updateChannel();
}

function applyChannel(c: UpdateChannel) {
  autoUpdater.channel = c === "beta" ? "beta" : "latest";
  autoUpdater.allowPrerelease = c === "beta";
  // Setting the channel turns downgrades on; a version is only ever replaced by a newer one.
  autoUpdater.allowDowngrade = false;
}

/** Choose the channel and look for an update on it. */
export function setUpdateChannel(c: UpdateChannel) {
  if (c === store.updateChannel()) return;
  store.setUpdateChannel(c);
  if (!started) return void onStateChange?.(state, false);
  applyChannel(c);
  // A finished download stays ready; anything else starts over on the new channel.
  if (state.status !== "ready") set({ status: "idle" });
  else onStateChange?.(state, false);
  checkInBackground();
}

/** Look for an update without telling anyone unless one is found. */
export function checkInBackground() {
  if (!started || state.status === "checking" || state.status === "downloading" || state.status === "ready") return;
  void autoUpdater.checkForUpdates().catch(() => {});
}

export function startUpdater(onChange: (s: UpdateState, statusChanged: boolean) => void) {
  onStateChange = onChange;
  if (started) return;
  if (!app.isPackaged) {
    demoState();
    return;
  }
  if (!selfUpdates) return;
  started = true;
  autoUpdater.autoDownload = true;
  autoUpdater.autoInstallOnAppQuit = true;
  autoUpdater.logger = null;
  applyChannel(store.updateChannel());

  autoUpdater.on("checking-for-update", () => set({ status: "checking" }));
  autoUpdater.on("update-not-available", () => set({ status: "idle" }));
  autoUpdater.on("update-available", (info: UpdateInfo) => set({ status: "available", version: info.version }));
  autoUpdater.on("download-progress", (p: ProgressInfo) => {
    const version = state.status === "available" || state.status === "downloading" ? state.version : "";
    const percent = Math.max(0, Math.min(100, Math.floor(p.percent ?? 0)));
    if (state.status === "downloading" && state.percent === percent) return;
    set({ status: "downloading", version, percent });
  });
  autoUpdater.on("update-downloaded", (info: UpdateInfo) => {
    set({ status: "ready", version: info.version, notes_url: RELEASE_NOTES_URL(info.version) });
    if (!Notification.isSupported()) return;
    const n = new Notification({ title: "Update ready", body: `opendesk ${info.version} installs when you restart.`, silent: true });
    n.on("click", () => restartToUpdate());
    n.show();
  });
  autoUpdater.on("error", (e) => {
    console.warn("update check failed:", e?.message ?? String(e));
    // A download that failed half way is retried at the next check.
    if (state.status !== "ready") set({ status: "error" });
  });

  checkInBackground();
  setInterval(checkInBackground, UPDATE_INTERVAL_MS).unref();
}

export function restartToUpdate() {
  // Silent on Windows (no installer window) and the app opens again after.
  if (state.status === "ready") autoUpdater.quitAndInstall(true, true);
}

/** From the menu: check now and say what happened. */
export async function checkNow(parent: Electron.BrowserWindow | null) {
  const show = (message: string, detail?: string) =>
    parent && !parent.isDestroyed() ? dialog.showMessageBox(parent, { message, detail, buttons: ["OK"] }) : dialog.showMessageBox({ message, detail, buttons: ["OK"] });
  if (!app.isPackaged) {
    await show("Updates are checked in installed builds", "Development builds don't update themselves.");
    return;
  }
  if (!selfUpdates) {
    const opts = { message: "Install updates from the download page", detail: `You have version ${app.getVersion()}. New versions for Linux are on the download page.`, buttons: ["Open download page", "OK"], defaultId: 0, cancelId: 1 };
    const r = parent && !parent.isDestroyed() ? await dialog.showMessageBox(parent, opts) : await dialog.showMessageBox(opts);
    if (r.response === 0) void shell.openExternal(DOWNLOAD_URL);
    return;
  }
  if (state.status === "ready") {
    const r = await dialog.showMessageBox({ message: `opendesk ${state.version} is ready`, detail: "Restart opendesk to install it.", buttons: ["Restart now", "Later"], defaultId: 0, cancelId: 1 });
    if (r.response === 0) restartToUpdate();
    return;
  }
  if (state.status === "downloading" || state.status === "available") {
    await show(`opendesk ${state.version} is downloading`, "You'll be asked to restart when it's ready.");
    return;
  }
  try {
    const r = await autoUpdater.checkForUpdates();
    if (r?.isUpdateAvailable) await show(`opendesk ${r.updateInfo.version} is downloading`, "You'll be asked to restart when it's ready.");
    else
      await show(
        "opendesk is up to date",
        `Version ${app.getVersion()}${store.updateChannel() === "beta" ? ", on beta updates" : ""}`,
      );
  } catch {
    await show("Couldn't check for updates", "Check your connection and try again.");
  }
}

/**
 * Development builds: `--update=downloading|ready` shows that state to the page,
 * so the web app's update banner can be seen without a release.
 */
function demoState() {
  if (!isDev) return;
  const demo = devArg("update");
  if (demo === "ready") set({ status: "ready", version: "9.9.9", notes_url: RELEASE_NOTES_URL("9.9.9") });
  else if (demo === "downloading") set({ status: "downloading", version: "9.9.9", percent: 42 });
}

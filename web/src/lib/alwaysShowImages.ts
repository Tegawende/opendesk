// Whether a customer's email shows its images from other sites at once (Settings → Profile →
// Appearance, per browser like the theme, off by default). Off, they wait for "Load images",
// so opening an email tells its sender nothing; on, they load with the email, as in most
// helpdesks, and the sender's server can tell it was opened.

import { useSyncExternalStore } from "react";

const KEY = "od-always-show-images";
const listeners = new Set<() => void>();

function read(): boolean {
  try {
    return localStorage.getItem(KEY) === "on";
  } catch {
    return false;
  }
}

let enabled = read();

export function setAlwaysShowImages(on: boolean) {
  enabled = on;
  try {
    if (on) localStorage.setItem(KEY, "on");
    else localStorage.removeItem(KEY);
  } catch {
    /* storage unavailable */
  }
  listeners.forEach((l) => l());
}

export function alwaysShowImages(): boolean {
  return enabled;
}

if (typeof window !== "undefined") {
  // Another tab changed it.
  window.addEventListener("storage", (e) => {
    if (e.key !== KEY && e.key !== null) return;
    enabled = read();
    listeners.forEach((l) => l());
  });
}

// Stable across renders, so a reader subscribes once.
function subscribe(cb: () => void) {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

function snapshot() {
  return enabled;
}

export function useAlwaysShowImages(): boolean {
  return useSyncExternalStore(subscribe, snapshot, snapshot);
}

// The emails whose images the reader loaded with "Load images", for as long as the tab is
// open, so the email shows them again when reopened and a print of it shows what they saw.
const loadedIds = new Set<string>();
const loadedListeners = new Set<() => void>();

export function loadImagesFor(messageId: string) {
  if (loadedIds.has(messageId)) return;
  loadedIds.add(messageId);
  loadedListeners.forEach((l) => l());
}

export function imagesLoadedFor(messageId: string): boolean {
  return loadedIds.has(messageId);
}

function subscribeLoaded(cb: () => void) {
  loadedListeners.add(cb);
  return () => {
    loadedListeners.delete(cb);
  };
}

/** Whether the reader loaded this email's images in this tab. */
export function useImagesLoadedFor(messageId: string): boolean {
  const get = () => loadedIds.has(messageId);
  return useSyncExternalStore(subscribeLoaded, get, get);
}

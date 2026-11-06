// How the app shows a file it holds before upload (a blob on the app's own origin).
// Nothing that could run a script there is opened as itself: it is the browser twin of
// hd.InlineSafe and hd.DetectType on the API.

// Shown by the browser without running anything: the raster images the API serves
// inline, PDF, plain text, audio and video.
const viewable = ["image/png", "image/jpeg", "image/gif", "image/webp", "image/avif", "image/bmp", "application/pdf", "text/plain"];

// Rendered by a browser as an active document (hd's activeTypes); shown as their text.
const active = ["text/html", "image/svg+xml", "application/xhtml+xml", "text/xml", "application/xml", "text/javascript", "application/javascript"];

/**
 * The type to open a file with in a new tab, or null when it is not opened at all:
 * active documents and other text open as plain text, unknown and binary types do not open.
 */
export function viewType(type: string): string | null {
  const base = type.split(";")[0].trim().toLowerCase();
  if (viewable.includes(base) || base.startsWith("audio/") || base.startsWith("video/")) return base;
  if (active.includes(base) || base.startsWith("text/") || base.endsWith("+xml")) return "text/plain;charset=utf-8";
  return null;
}

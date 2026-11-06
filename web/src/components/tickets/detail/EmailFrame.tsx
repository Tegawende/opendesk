import { useEffect, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/cn";
import { useTheme } from "@/lib/theme";

// A designed email (receipts, newsletters, notifications) renders as its sender laid it
// out: its own styles in a frame of its own, light like a mail client's reading pane.
// The frame runs no scripts; links open in a new tab. It grows to its content, and a
// fixed-width layout wider than the column is scaled down to fit.
//
// A themed frame (an internal note laid out with tables or colours, such as an assistant's
// summary) is ours rather than a sender's: it sits on the message's own background and, in
// the dark theme, its colours are turned dark (inverted, hues kept; images turned back).

const BASE = `
:root { color-scheme: light; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
html, body { margin: 0; padding: 0; background: #fff; }
body { color: #1f2328; font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; overflow-wrap: break-word; -webkit-text-size-adjust: 100%; overflow: hidden; }
#od-email { display: flow-root; }
:where(img) { max-width: 100%; }
:where(img[width][height]) { height: auto; }
:where(a) { color: #0b63ce; }
:where(img[data-od-held]) { background: #f1f3f5; }
`;

const THEMED = `
html, body { background: transparent; }
`;

const THEMED_DARK = `
#od-email { filter: invert(0.9) hue-rotate(180deg); }
#od-email :is(img, video, picture) { filter: invert(1) hue-rotate(180deg); }
`;

// Past this the frame scrolls on its own rather than growing the page.
const MAX_HEIGHT = 12000;

export function frameDocument(html: string, themed = false, dark = false) {
  const css = BASE + (themed ? THEMED : "") + (themed && dark ? THEMED_DARK : "");
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="color-scheme" content="light"><meta name="referrer" content="no-referrer"><base target="_blank"><style>${css}</style></head><body><div id="od-email">${html}</div></body></html>`;
}

/**
 * html is the output of sanitizeEmailHtml. themed: an internal note, following the app's theme.
 * print: on a page to print, which is light whatever the theme, and where the frame takes its
 * content's whole height (a frame does not scroll on paper).
 */
export function EmailFrame({ html, title, className, themed = false, print = false }: { html: string; title: string; className?: string; themed?: boolean; print?: boolean }) {
  const ref = useRef<HTMLIFrameElement>(null);
  const [height, setHeight] = useState(0);
  const theme = useTheme();
  const dark = theme.dark && !print;
  const max = print ? Infinity : MAX_HEIGHT;
  const doc = useMemo(() => frameDocument(html, themed, dark), [html, themed, dark]);

  useEffect(() => {
    const frame = ref.current;
    if (!frame) return;
    let observer: ResizeObserver | undefined;
    const fit = () => {
      const root = frame.contentDocument?.getElementById("od-email");
      if (!root) return;
      root.style.zoom = "";
      const wide = root.scrollWidth;
      const avail = frame.clientWidth;
      if (avail > 0 && wide > avail + 1) root.style.zoom = String(Math.max(avail / wide, 0.4));
      const full = Math.ceil(root.getBoundingClientRect().height);
      const h = Math.min(full, max);
      root.ownerDocument.body.style.overflowY = full > max ? "auto" : "hidden";
      setHeight((prev) => (Math.abs(prev - h) > 1 ? h : prev));
    };
    // Keyboard events inside a frame do not bubble to the reading pane.
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented || event.isComposing) return;
      const forwarded = new KeyboardEvent("keydown", { key: event.key, code: event.code, bubbles: true, cancelable: true,
        ctrlKey: event.ctrlKey, metaKey: event.metaKey, altKey: event.altKey, shiftKey: event.shiftKey });
      if (!document.dispatchEvent(forwarded)) event.preventDefault();
    };
    let frameDoc: Document | null = null;
    const onLoad = () => {
      observer?.disconnect();
      frameDoc?.removeEventListener("keydown", onKey);
      frameDoc = frame.contentDocument;
      frameDoc?.addEventListener("keydown", onKey);
      const root = frameDoc?.getElementById("od-email");
      if (!root) return;
      fit();
      observer = new ResizeObserver(fit);
      observer.observe(root);
      observer.observe(frame);
    };
    frame.addEventListener("load", onLoad);
    if (frame.contentDocument?.readyState === "complete") onLoad();
    return () => {
      frame.removeEventListener("load", onLoad);
      observer?.disconnect();
      frameDoc?.removeEventListener("keydown", onKey);
    };
  }, [doc, max]);

  // On a dark surface the light pane gets a margin of its own; a themed one has no pane.
  return (
    <div className={cn("overflow-hidden rounded-md", !themed && "bg-white dark:p-3", className)}>
      <iframe
        ref={ref}
        title={title}
        srcDoc={doc}
        sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
        referrerPolicy="no-referrer"
        className={cn("ph-no-capture block w-full border-0 [color-scheme:light]", themed ? "bg-transparent" : "bg-white")}
        style={{ height: height || 80 }}
        data-ph-mask
      />
    </div>
  );
}

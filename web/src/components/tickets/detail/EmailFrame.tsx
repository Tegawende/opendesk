import { useEffect, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/cn";

// A designed email (receipts, newsletters, notifications) renders as its sender laid it
// out: its own styles in a frame of its own, light like a mail client's reading pane.
// The frame runs no scripts; links open in a new tab. It grows to its content, and a
// fixed-width layout wider than the column is scaled down to fit.

const BASE = `
:root { color-scheme: light; }
html, body { margin: 0; padding: 0; background: #fff; }
body { color: #1f2328; font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; overflow-wrap: break-word; -webkit-text-size-adjust: 100%; overflow: hidden; }
#od-email { display: flow-root; }
:where(img) { max-width: 100%; }
:where(img[width][height]) { height: auto; }
:where(a) { color: #0b63ce; }
:where(img[data-od-held]) { background: #f1f3f5; }
`;

// Past this the frame scrolls on its own rather than growing the page.
const MAX_HEIGHT = 12000;

function frameDocument(html: string) {
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="color-scheme" content="light"><meta name="referrer" content="no-referrer"><base target="_blank"><style>${BASE}</style></head><body><div id="od-email">${html}</div></body></html>`;
}

/** html is the output of sanitizeEmailHtml. */
export function EmailFrame({ html, title, className }: { html: string; title: string; className?: string }) {
  const ref = useRef<HTMLIFrameElement>(null);
  const [height, setHeight] = useState(0);
  const doc = useMemo(() => frameDocument(html), [html]);

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
      const h = Math.min(full, MAX_HEIGHT);
      root.ownerDocument.body.style.overflowY = full > MAX_HEIGHT ? "auto" : "hidden";
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
  }, [doc]);

  // On a dark surface the light pane gets a margin of its own.
  return (
    <div className={cn("overflow-hidden rounded-md bg-white dark:p-3", className)}>
      <iframe
        ref={ref}
        title={title}
        srcDoc={doc}
        sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
        referrerPolicy="no-referrer"
        className="ph-no-capture block w-full border-0 bg-white [color-scheme:light]"
        style={{ height: height || 80 }}
        data-ph-mask
      />
    </div>
  );
}

// The edge between two panes, dragged to resize the one beside it (lib/panes.ts). It takes no
// room of its own: an 8px strip centred on the pane's border, a hairline on hover, a stronger one
// while dragging or focused. Dragging writes the width straight to the page once a frame
// (`onLive`) and saves it on release (`onWidth`); arrows step it, Home and End go to the ends,
// and a double click goes back to the default.

import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/cn";
import { clampWidth, keyWidth, maxWidth, paneLimits, type Pane } from "@/lib/panes";

type Props = {
  pane: Pane;
  /** The element the pane's width is read from. */
  target: RefObject<HTMLElement | null>;
  /** Pixels in the room the pane shares (the window, its row). */
  room: () => number | undefined;
  /** Where the pane sits in reading order: before the handle (a left pane in LTR) or after it. */
  side: "before" | "after";
  /** The width saved for the pane, null for the default. */
  width: number | null;
  /** Shows a width while dragging, without a render; null puts back the saved one. */
  onLive: (px: number | null) => void;
  /** Saves a width, or goes back to the default with null. */
  onWidth: (px: number | null) => void;
  className?: string;
};

const RESIZING = "paneResizing";

export function ResizeHandle({ pane, target, room, side, width, onLive, onWidth, className }: Props) {
  const { t } = useTranslation("ui");
  const label = { nav: t("resizeSidebar"), list: t("resizeTicketList"), details: t("resizeTicketDetails") }[pane];
  const self = useRef<HTMLDivElement>(null);
  const drag = useRef<{ id: number; x: number; w: number; room: number | undefined; grow: 1 | -1; px: number; raf: number } | null>(null);
  const [dragging, setDragging] = useState(false);
  const [value, setValue] = useState<{ now: number; max: number }>({ now: 0, max: paneLimits[pane].max });

  // ArrowRight widens a pane drawn left of its handle; reading order flips under RTL.
  const grow = (): 1 | -1 => {
    const rtl = !!self.current && getComputedStyle(self.current).direction === "rtl";
    return (side === "before") !== rtl ? 1 : -1;
  };
  const measure = () => {
    const el = target.current;
    if (!el) return;
    const now = Math.round(el.getBoundingClientRect().width);
    const max = maxWidth(pane, room());
    setValue((v) => (v.now === now && v.max === max ? v : { now, max }));
  };
  // Where a drag or a key press starts: the saved width as the room allows, so a press while the
  // pane still eases to the last one steps from where it is going; else the default as drawn.
  const current = (el: HTMLElement, space: number | undefined) => (width === null ? Math.round(el.getBoundingClientRect().width) : clampWidth(pane, width, space));
  const measureRef = useRef(measure);
  measureRef.current = measure;

  // Read again when the saved width changes, and as the pane or the room around it resizes
  // (the window, a neighbour, the pane opening); a drag reads its own width as it goes.
  useLayoutEffect(() => measureRef.current(), [width]);
  useEffect(() => {
    const el = target.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    let raf = 0;
    const ro = new ResizeObserver(() => {
      if (drag.current) return;
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => measureRef.current());
    });
    ro.observe(el);
    if (el.parentElement) ro.observe(el.parentElement);
    return () => {
      ro.disconnect();
      cancelAnimationFrame(raf);
    };
  }, [target]);

  const end = (save: boolean) => {
    const d = drag.current;
    if (!d) return;
    drag.current = null;
    cancelAnimationFrame(d.raf);
    delete document.documentElement.dataset[RESIZING];
    setDragging(false);
    if (save && d.px !== d.w) {
      onLive(d.px);
      onWidth(d.px);
    } else onLive(width);
    measureRef.current();
  };
  useEffect(() => () => end(false), []); // eslint-disable-line react-hooks/exhaustive-deps

  const down = (e: PointerEvent<HTMLDivElement>) => {
    const el = target.current;
    if (e.button !== 0 || !el) return;
    e.preventDefault();
    e.currentTarget.focus({ preventScroll: true });
    e.currentTarget.setPointerCapture(e.pointerId);
    const space = room();
    const w = current(el, space);
    drag.current = { id: e.pointerId, x: e.clientX, w, room: space, grow: grow(), px: w, raf: 0 };
    document.documentElement.dataset[RESIZING] = "";
    setDragging(true);
  };
  const move = (e: PointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (!d || e.pointerId !== d.id) return;
    d.px = clampWidth(pane, d.w + (e.clientX - d.x) * d.grow, d.room);
    if (!d.raf)
      d.raf = requestAnimationFrame(() => {
        d.raf = 0;
        onLive(d.px);
      });
  };

  const key = (e: KeyboardEvent<HTMLDivElement>) => {
    const el = target.current;
    if (!el || e.altKey || e.ctrlKey || e.metaKey) return;
    const space = room();
    const next = keyWidth(e.key, e.shiftKey, current(el, space), pane, space, grow());
    if (next === null) return;
    e.preventDefault();
    onLive(next);
    onWidth(next);
    setValue({ now: next, max: maxWidth(pane, space) });
  };

  return (
    <div
      ref={self}
      role="separator"
      aria-label={label}
      aria-orientation="vertical"
      aria-valuenow={value.now}
      aria-valuemin={paneLimits[pane].min}
      aria-valuemax={value.max}
      tabIndex={0}
      data-dragging={dragging || undefined}
      onPointerDown={down}
      onPointerMove={move}
      onPointerUp={() => end(true)}
      onPointerCancel={() => end(false)}
      onLostPointerCapture={() => end(true)}
      onDoubleClick={() => onWidth(null)}
      onKeyDown={key}
      onFocus={measure}
      className={cn(
        "relative z-20 -mx-1 w-2 shrink-0 cursor-col-resize touch-none select-none outline-none",
        "before:absolute before:inset-y-0 before:inset-x-[3px] before:transition-colors before:duration-150 hover:before:bg-line-strong hover:before:delay-100",
        "focus-visible:before:bg-focus-line data-dragging:before:bg-focus-line data-dragging:before:delay-0",
        className,
      )}
    />
  );
}

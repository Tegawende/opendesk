// A view's icon (View.icon): an emoji, an icon of the list (viewIcons.ts) or one or two
// characters on a small tile, the last two in the view's colour; the layers icon when
// it has none. Sized to sit where the plain icon sat.

import { Layers } from "lucide-react";
import type { CSSProperties } from "react";
import type { View, ViewIcon as ViewIconValue } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { isTone, tone as tones, type Tone } from "@/lib/tones";
import { viewIconDef } from "./viewIcons";

/** The icon's colour, when it has one of the palette's (an emoji never does). */
export const viewIconTone = (icon: ViewIconValue | null | undefined): Tone | null => (icon && icon.kind !== "emoji" && isTone(icon.color) ? icon.color : null);

/**
 * `size` is the box in pixels, the size of the icons beside it. A lucide icon takes its
 * size from the row, as the plain icon did.
 */
export function ViewIcon({ icon, size = 15, className }: { icon: ViewIconValue | null | undefined; size?: number; className?: string }) {
  const tint = viewIconTone(icon);
  // Inline, so the row's own icon colour (idle, hover, current) does not cover it.
  const color: CSSProperties | undefined = tint ? { color: `var(--tone-${tint})` } : undefined;
  if (icon?.kind === "emoji") {
    const style: CSSProperties = { width: size, height: size, fontSize: size < 15 ? size - 2 : size < 18 ? 13 : size - 4 };
    return (
      <span aria-hidden data-view-icon="emoji" className={cn("flex shrink-0 select-none items-center justify-center leading-none", className)} style={style}>
        {icon.value}
      </span>
    );
  }
  if (icon?.kind === "text") {
    const two = [...icon.value].length > 1;
    const style: CSSProperties = { width: size, height: size, fontSize: Math.round(size * (two ? 0.5 : 0.62)) };
    return (
      <span
        aria-hidden
        data-view-icon="text"
        className={cn("flex shrink-0 select-none items-center justify-center rounded-[4px] font-semibold leading-none tracking-[-0.02em]", tones[tint ?? "gray"].chip, className)}
        style={style}
      >
        {icon.value}
      </span>
    );
  }
  const Icon = (icon?.kind === "icon" && viewIconDef(icon.value)?.icon) || Layers;
  return <Icon aria-hidden data-view-icon={icon?.kind === "icon" ? "icon" : "none"} className={className} style={color} />;
}

const noVariation = (s: string) => s.replace(/️/g, "");

/**
 * The name to show beside the icon: a name that starts with the view's own emoji
 * (as people wrote them before views had icons) does not show it twice. The stored
 * name stays as it is.
 */
export function viewName(v: Pick<View, "name" | "icon">): string {
  if (v.icon?.kind !== "emoji") return v.name;
  const want = noVariation(v.icon.value);
  let i = 0;
  let got = "";
  while (i < v.name.length && got.length < want.length) {
    const c = v.name[i++];
    if (c !== "️") got += c;
  }
  while (v.name[i] === "️") i++;
  // Only a whole emoji followed by a space: not the first half of a longer one.
  if (got !== want || (i < v.name.length && !/\s/.test(v.name[i]))) return v.name;
  return v.name.slice(i).trim() || v.name;
}

/**
 * Whether s is one emoji, near enough to what the API takes (one grapheme that is a
 * pictograph, no letters, digits or spaces); the API has the last word.
 */
export function isOneEmoji(s: string): boolean {
  if (!s || new TextEncoder().encode(s).length > 64) return false;
  // A flag (two regional indicators) or a keycap.
  if (/^\p{Regional_Indicator}{2}$/u.test(s) || /^[0-9#*]️?⃣$/u.test(s)) return true;
  if (!/^\p{Extended_Pictographic}/u.test(s) || /[\p{L}\p{N}\s<>]/u.test(s)) return false;
  return [...new Intl.Segmenter().segment(s)].length === 1;
}

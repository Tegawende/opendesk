# opendesk brand

Monochrome, geometric and quiet. The mark, "Module", is a 2×2 grid with one piece reshaped into a circle: modular, extensible, open to plugins.

Figma source: https://www.figma.com/design/JfEIfmFuwLsDJCSRXOUQGH/Branding

## Files

| Path | Use |
| --- | --- |
| `logo/lockup.svg`, `logo/lockup-white.svg` | Primary logo (mark + wordmark) |
| `logo/*-steel.svg` | Steel versions of each logo, for a background you cannot know (an email read in light or dark mode) |
| `logo/lockup*.png`, `logo/mark*.png` | PNG logos for places that take no SVG, such as email signatures. Served at `opendesk.sh/brand/opendesk-logo{,-white,-steel}.png` and `opendesk-mark{,-white,-steel}.png` |
| `logo/stacked*.svg` | Square spaces, splash screens |
| `logo/mark*.svg` | Mark on its own (avatars, small sizes) |
| `logo/wordmark*.svg` | Wordmark on its own, outlined |
| `icons/favicon.svg` | Favicon that adapts to light and dark |
| `icons/icon-*.png`, `icons/apple-touch-icon.png` | PWA, browser and app icons |
| `social/og.jpg` | Open Graph / GitHub social preview (1920×1008), from `pnpm og` in `site/` |
| `social/avatar.jpg`, `social/avatar-light.jpg` | Profile pictures (1024×1024, square): white mark on ink, ink mark on white. Safe in a circle crop |
| `social/avatar-painted.jpg` | Profile picture: the mark on a frosted tile over the hero painting, to match the social card |
| `tokens.css`, `tokens.json` | Colours and type tokens |

Regenerate the SVGs and PNGs with `node scripts/build-brand.mjs` (after `pnpm install` in `site/`), then the social card and profile pictures with `pnpm og` in `site/`.

App icons use the ink mark on paper in light and dark appearances. `node scripts/build-brand.mjs` also writes the installed web app icons and the iOS icon. Run `pnpm icons` in `desktop/` to regenerate the desktop icons.

## Rules

- **Colour:** ink `#0A0A0A` on paper `#FFFFFF`, or the reverse. Steel `#737373` only where the background is unknown, such as an email signature. No gradients, tints, outlines or shadows.
- **Clear space:** at least ¼ of the mark's height on every side. Never close or change the gaps between modules.
- **Minimum size:** mark 16px; lockup 80px wide.
- **Type:** Geist SemiBold at −4% tracking for headlines, Geist Regular for body, Geist Mono for code and labels.
- **Name:** always lowercase: `opendesk`. Use `opendesk.sh` only when you mean the domain.

## Palette

| Name | Hex | Use |
| --- | --- | --- |
| Ink | `#0A0A0A` | Logo, primary text, primary buttons |
| Coal | `#171717` | Dark surfaces |
| Charcoal | `#262626` | Raised dark surfaces, dark borders |
| Graphite | `#525252` | Secondary text |
| Steel | `#737373` | Logo on a background you cannot know (email signatures) |
| Smoke | `#8A8A8A` | Muted text, icons |
| Ash | `#E4E4E4` | Borders, dividers |
| Mist | `#F4F4F4` | Subtle fills, hover |
| Paper | `#FFFFFF` | Background |

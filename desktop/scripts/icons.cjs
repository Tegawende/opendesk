// Renders the app icons from the brand mark: `pnpm icons` (runs in Electron).
//   build/icon.png       macOS, 1024px on Apple's grid (824px tile, soft shadow)
//   build/icon-win.png   Windows, the full-bleed brand tile (brand/icons/icon-512.png)
//   build/icons/512x512.png  Linux, the same

const { app, BrowserWindow } = require("electron");
const { copyFileSync, mkdirSync, readFileSync, writeFileSync } = require("node:fs");
const { join } = require("node:path");

const root = join(__dirname, "..");
const brand = join(root, "..", "brand");
const mark = readFileSync(join(brand, "logo", "mark.svg"), "utf8").match(/d="([^"]+)"/)[1];

const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">
  <defs>
    <filter id="s" x="-20%" y="-20%" width="140%" height="140%"><feDropShadow dx="0" dy="10" stdDeviation="14" flood-color="#000" flood-opacity="0.28"/></filter>
    <linearGradient id="g" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#f4f4f4"/></linearGradient>
  </defs>
  <rect x="100" y="100" width="824" height="824" rx="185" fill="url(#g)" filter="url(#s)"/>
  <rect x="100.5" y="100.5" width="823" height="823" rx="184.5" fill="none" stroke="#0a0a0a" stroke-opacity="0.08"/>
  <path transform="translate(296 296) scale(6.75)" fill="#0a0a0a" fill-rule="evenodd" d="${mark}"/>
</svg>`;

app.whenReady().then(async () => {
  const w = new BrowserWindow({ width: 1024, height: 1024, show: false, frame: false, transparent: true, webPreferences: { offscreen: true } });
  await w.loadURL(`data:text/html;charset=utf-8,${encodeURIComponent(`<html><body style="margin:0;background:transparent">${svg}</body></html>`)}`);
  await new Promise((r) => setTimeout(r, 500));
  const img = await w.webContents.capturePage({ x: 0, y: 0, width: 1024, height: 1024 });
  mkdirSync(join(root, "build", "icons"), { recursive: true });
  writeFileSync(join(root, "build", "icon.png"), img.resize({ width: 1024, height: 1024 }).toPNG());
  copyFileSync(join(brand, "icons", "icon-512.png"), join(root, "build", "icon-win.png"));
  copyFileSync(join(brand, "icons", "icon-512.png"), join(root, "build", "icons", "512x512.png"));
  console.log("icons written to desktop/build");
  app.quit();
});

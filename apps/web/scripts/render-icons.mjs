#!/usr/bin/env node
// Regenerates every app icon from the Polaris mark's geometry (the same paths
// as src/components/brand/polaris-mark.tsx):
//
//   src/app/icon.svg, ../site/favicon.svg   solid star on a rounded tile (tab favicons)
//   src/app/favicon.ico                     16 + 32 px, solid star
//   src/app/apple-icon.png                  180 px, full bleed (iOS rounds it), facets
//   public/icon-192.png, public/icon-512.png rounded tile, facets
//   public/icon-maskable-512.png            full bleed, mark inside the 80% safe zone
//
// Usage: pnpm icons   (needs Playwright's Chromium, already a dev dependency)

import { writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const web = join(dirname(fileURLToPath(import.meta.url)), "..");

// Catppuccin Mocha, the built-in palette: base and mauve.
const BG = "#1e1e2e";
const MARK = "#cba6f7";

// 64-unit geometry. Facets: two at full strength, two at half. The solid
// silhouette is slightly fuller so it survives 16 px.
const FACETS_FULL =
  "M14 31.5C28 27.5 30.4 21 32 3V31.5ZM50 31.5C36 35.5 33.6 41 32 54V31.5Z";
const FACETS_HALF =
  "M32 3C33.6 21 36 27.5 50 31.5H32ZM32 54C30.4 41 28 35.5 14 31.5H32Z";
const SOLID =
  "M32 2C34.6 19 38 26 52 31.5 38 37 34.6 43 32 56 29.4 43 26 37 12 31.5 26 26 29.4 19 32 2Z";
// Optical centre: between the bounding-box centre and the crossing point.
const CENTER_Y = 30;
const MARK_HEIGHT = 52;

/**
 * One square icon as SVG.
 * @param {object} o
 * @param {number} o.size     viewBox size
 * @param {number|null} o.rx  tile corner radius; null = full bleed
 * @param {number} o.mark     mark height as a fraction of size
 * @param {boolean} o.solid   solid silhouette instead of facets
 */
function iconSvg({ size, rx, mark, solid }) {
  const k = +((size * mark) / MARK_HEIGHT).toFixed(4);
  const c = size / 2;
  const tile =
    rx === null
      ? `<rect width="${size}" height="${size}" fill="${BG}"/>`
      : `<rect width="${size}" height="${size}" rx="${rx}" fill="${BG}"/>`;
  const star = solid
    ? `<path d="${SOLID}"/>`
    : `<path d="${FACETS_FULL}"/><path d="${FACETS_HALF}" opacity=".5"/>`;
  return [
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${size} ${size}">`,
    `  ${tile}`,
    `  <g fill="${MARK}" transform="translate(${c} ${c}) scale(${k}) translate(-32 -${CENTER_Y})">${star}</g>`,
    `</svg>`,
    "",
  ].join("\n");
}

const favicon = iconSvg({ size: 32, rx: 7, mark: 0.8, solid: true });
writeFileSync(join(web, "src/app/icon.svg"), favicon);
writeFileSync(join(web, "../site/favicon.svg"), favicon);

const rasters = [
  {
    out: "public/icon-512.png",
    size: 512,
    rx: 112,
    mark: 0.66,
    solid: false,
    alpha: true,
  },
  {
    out: "public/icon-192.png",
    size: 192,
    rx: 42,
    mark: 0.66,
    solid: false,
    alpha: true,
  },
  {
    out: "public/icon-maskable-512.png",
    size: 512,
    rx: null,
    mark: 0.5,
    solid: false,
    alpha: false,
  },
  {
    out: "src/app/apple-icon.png",
    size: 180,
    rx: null,
    mark: 0.6,
    solid: false,
    alpha: false,
  },
];

const browser = await chromium.launch();
const page = await browser.newPage({ deviceScaleFactor: 1 });

async function render(svg, size, alpha) {
  await page.setViewportSize({ width: size, height: size });
  await page.setContent(
    `<html><body style="margin:0;background:transparent">${svg.replace(
      "<svg ",
      `<svg width="${size}" height="${size}" style="display:block" `,
    )}</body></html>`,
  );
  return page.screenshot({ omitBackground: alpha, type: "png" });
}

for (const r of rasters) {
  writeFileSync(join(web, r.out), await render(iconSvg(r), r.size, r.alpha));
}

// favicon.ico: an ICO directory holding two PNG images (16 and 32 px).
const icoImages = [];
for (const size of [16, 32]) {
  const svg = iconSvg({ size: 32, rx: 7, mark: 0.8, solid: true });
  icoImages.push({ size, png: await render(svg, size, true) });
}
await browser.close();

const header = Buffer.alloc(6);
header.writeUInt16LE(0, 0); // reserved
header.writeUInt16LE(1, 2); // type: icon
header.writeUInt16LE(icoImages.length, 4);
let offset = 6 + 16 * icoImages.length;
const entries = icoImages.map(({ size, png }) => {
  const e = Buffer.alloc(16);
  e.writeUInt8(size, 0); // width
  e.writeUInt8(size, 1); // height
  e.writeUInt8(0, 2); // palette colours
  e.writeUInt8(0, 3); // reserved
  e.writeUInt16LE(1, 4); // colour planes
  e.writeUInt16LE(32, 6); // bits per pixel
  e.writeUInt32LE(png.length, 8);
  e.writeUInt32LE(offset, 12);
  offset += png.length;
  return e;
});
writeFileSync(
  join(web, "src/app/favicon.ico"),
  Buffer.concat([header, ...entries, ...icoImages.map((i) => i.png)]),
);

console.log("icons written");

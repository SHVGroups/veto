// Renders the brand SVGs to PNG for places that need raster images:
// social previews, app icons and the README banner on platforms that do
// not show SVG. Run after brand/build.py.
import { Resvg } from "@resvg/resvg-js";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const out = join(here, "png");
mkdirSync(out, { recursive: true });

const jobs = [
  ["app-icon.svg", "app-icon-512.png", 512],
  ["app-icon.svg", "app-icon-192.png", 192],
  ["app-icon.svg", "apple-touch-icon.png", 180],
  ["favicon.svg", "favicon-32.png", 32],
  ["og.svg", "og.png", 1200],
  ["banner.svg", "banner.png", 2560],
  ["banner-dark.svg", "banner-dark.png", 2560],
  ["architecture.svg", "architecture.png", 2560],
  ["pipeline.svg", "pipeline.png", 2560],
];

for (const [src, dst, width] of jobs) {
  const svg = readFileSync(join(here, src));
  const png = new Resvg(svg, { fitTo: { mode: "width", value: width } }).render().asPng();
  writeFileSync(join(out, dst), png);
  console.log(`wrote png/${dst}`);
}

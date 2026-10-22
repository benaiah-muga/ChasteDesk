// SPDX-License-Identifier: AGPL-3.0-only
import fs from "node:fs";
import path from "node:path";

const root = process.cwd();
const staticDir = path.join(root, "internal", "web", "static");
const consoleDir = path.join(staticDir, "console");
const indexFile = path.join(consoleDir, "index.html");
if (!fs.existsSync(indexFile)) {
  console.error("console index is missing; build the UI first");
  process.exit(1);
}
const html = fs.readFileSync(indexFile, "utf8");
const references = [...html.matchAll(/(?:src|href)="([^"]+)"/g)].map((match) => match[1]);
const required = references.filter((ref) => ref.startsWith("/static/console/"));
if (required.length < 2) {
  console.error("console index does not reference the built script and stylesheet");
  process.exit(1);
}
for (const ref of references) {
  if (/^https?:\/\//i.test(ref)) {
    console.error(`console index loads an external asset: ${ref}`);
    process.exit(1);
  }
  if (!ref.startsWith("/static/")) {
    console.error(`console index has a non-static asset reference: ${ref}`);
    process.exit(1);
  }
  const local = path.join(staticDir, ref.replace(/^\/static\//, ""));
  if (!fs.existsSync(local)) {
    console.error(`referenced asset is missing: ${ref}`);
    process.exit(1);
  }
}
const files = [];
function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const target = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(target);
    else files.push(target);
  }
}
walk(consoleDir);
for (const file of files) {
  if (!file.endsWith(".css")) continue;
  const body = fs.readFileSync(file, "utf8");
  if (/url\(\s*['"]?https?:\/\//i.test(body)) {
    console.error(`console stylesheet references a remote resource: ${path.relative(root, file)}`);
    process.exit(1);
  }
}
for (const icon of ["favicon-32.png", "favicon-192.png", "apple-touch-icon.png", "favicon-512.png"]) {
  const iconFile = path.join(staticDir, icon);
  if (!fs.existsSync(iconFile) || fs.statSync(iconFile).size < 500) {
    console.error(`required brand icon is missing: ${icon}`);
    process.exit(1);
  }
}
for (const font of ["instrument-sans.woff2", "instrument-serif.woff2", "instrument-serif-italic.woff2", "jetbrains-mono.woff2"]) {
  const fontFile = path.join(staticDir, "fonts", font);
  if (!fs.existsSync(fontFile) || fs.statSync(fontFile).size < 500) {
    console.error(`required self-hosted font is missing: ${font}`);
    process.exit(1);
  }
}
const tokens = fs.readFileSync(path.join(staticDir, "css", "tokens.css"), "utf8");
if (!tokens.includes("@font-face") || tokens.includes("@import url(http")) {
  console.error("design tokens are not self-hosted");
  process.exit(1);
}
console.log("ui assets verified: all console assets are local, fonts are self-hosted");

// SPDX-License-Identifier: AGPL-3.0-only
import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";

const root = process.cwd();
const uiDir = path.join(root, "ui");
const indexFile = path.join(root, "internal", "web", "static", "console", "index.html");
const assetDir = path.join(root, "internal", "web", "static", "console", "assets");

const build = spawnSync("npm", ["--prefix", "ui", "run", "build"], {
  cwd: root,
  stdio: "inherit",
  env: { ...process.env, CI: "true" },
});
if (build.status !== 0) {
  console.error("ui build failed");
  process.exit(1);
}
if (!fs.existsSync(indexFile)) {
  console.error("ui build missing internal/web/static/console/index.html");
  process.exit(1);
}
const html = fs.readFileSync(indexFile, "utf8");
if (!html.includes('id="root"') || !html.includes("/static/console/assets/")) {
  console.error("ui index is not a production console document");
  process.exit(1);
}
if (!fs.existsSync(assetDir)) {
  console.error("ui build has no asset directory");
  process.exit(1);
}
const assets = fs.readdirSync(assetDir);
const js = assets.filter((name) => name.endsWith(".js"));
const css = assets.filter((name) => name.endsWith(".css"));
if (js.length === 0 || css.length === 0) {
  console.error("ui build is missing JavaScript or CSS assets");
  process.exit(1);
}
const indexSize = fs.statSync(indexFile).size;
const jsSize = js.reduce((total, name) => total + fs.statSync(path.join(assetDir, name)).size, 0);
const cssSize = css.reduce((total, name) => total + fs.statSync(path.join(assetDir, name)).size, 0);
if (indexSize < 400 || jsSize < 100_000 || cssSize < 10_000) {
  console.error(`ui build is suspiciously small: index=${indexSize} js=${jsSize} css=${cssSize}`);
  process.exit(1);
}
if (!fs.existsSync(path.join(uiDir, "package-lock.json"))) {
  console.error("ui package-lock.json is missing");
  process.exit(1);
}
console.log(`ui build verified: ${js.length} chunks, ${Math.round(jsSize / 1024)} KB JS, ${Math.round(cssSize / 1024)} KB CSS`);

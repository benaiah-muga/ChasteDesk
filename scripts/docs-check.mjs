// SPDX-License-Identifier: AGPL-3.0-only
import fs from "node:fs";
import path from "node:path";

const root = process.cwd();
const required = [
  "README.md",
  "AGENTS.md",
  "CONTRIBUTING.md",
  "CHANGELOG.md",
  "SECURITY.md",
  "design.md",
  "VERSION",
  "docs/README.md",
  "docs/ARCHITECTURE.md",
  "docs/SELF_HOSTING.md",
  "docs/API.md",
  "docs/OPERATIONS.md",
  "docs/SECURITY.md",
  "docs/DEVELOPMENT.md",
  "docs/ROADMAP.md",
  "docs/PRODUCTION_READINESS.md",
  "docs/SCREENSHOTS.md",
  "docs/assets/chastedesk-banner.png",
  "docs/assets/inbox-feature.png",
  "docs/assets/sla.png",
  "internal/web/static/favicon-32.png",
  "docs/assets/overview.png",
  "docs/assets/inbox.png",
  "docs/assets/widget.png",
  "docs/assets/analytics.png",
  "docs/assets/quick-replies.png",
  "docs/assets/inbox-thread-dark.png",
];

const failures = [];
for (const file of required) {
  if (!fs.existsSync(path.join(root, file))) failures.push(`missing ${file}`);
  else if (fs.statSync(path.join(root, file)).size === 0) failures.push(`empty ${file}`);
}

const readme = fs.readFileSync(path.join(root, "README.md"), "utf8");
for (const heading of ["## Why ChasteDesk", "## Quickstart", "## Configuration", "## Architecture", "## Security", "## Documentation", "## License"]) {
  if (!readme.includes(heading)) failures.push(`README missing ${heading}`);
}
if (/localhost:3000/.test(readme) === false) failures.push("README must document the default port");
if (readme.includes("pnpm") && !readme.includes("screenshot")) failures.push("README should not present pnpm as the app runtime");

const index = fs.readFileSync(path.join(root, "docs/README.md"), "utf8");
for (const target of ["ARCHITECTURE.md", "SELF_HOSTING.md", "API.md", "OPERATIONS.md", "SECURITY.md", "DEVELOPMENT.md", "ROADMAP.md", "PRODUCTION_READINESS.md"]) {
  if (!index.includes(target)) failures.push(`docs index missing ${target}`);
}

function markdownFiles(dir) {
  const out = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === "node_modules" || entry.name === ".git") continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...markdownFiles(full));
    else if (entry.name.endsWith(".md")) out.push(full);
  }
  return out;
}

for (const file of markdownFiles(path.join(root, "docs")).concat([
  path.join(root, "README.md"),
  path.join(root, "CONTRIBUTING.md"),
  path.join(root, "CHANGELOG.md"),
  path.join(root, "SECURITY.md"),
  path.join(root, "AGENTS.md"),
  path.join(root, "design.md"),
])) {
  const text = fs.readFileSync(file, "utf8");
  if (text.includes("\u2014")) failures.push(`em dash in ${path.relative(root, file)}`);
  const links = [...text.matchAll(/\]\(([^)]+)\)/g)].map((match) => match[1]);
  for (const raw of links) {
    if (/^(https?:|mailto:|#)/.test(raw)) continue;
    const clean = raw.split("#")[0].trim();
    if (!clean) continue;
    const target = path.resolve(path.dirname(file), clean);
    if (!fs.existsSync(target)) failures.push(`broken link in ${path.relative(root, file)}: ${raw}`);
  }
}

const sourceRoots = ["internal", "cmd", "scripts", "docs", ".github"];
for (const dir of sourceRoots) {
  const walk = (current) => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) {
        if (["node_modules", ".git"].includes(entry.name)) continue;
        walk(full);
        continue;
      }
      if (!/\.(go|templ|md|css|js|mjs|sql|ya?ml|json)$/.test(entry.name)) continue;
      if (entry.name.endsWith("_templ.go")) continue;
      const text = fs.readFileSync(full, "utf8");
      if (text.includes("\u2014")) failures.push(`em dash in ${path.relative(root, full)}`);
    }
  };
  if (fs.existsSync(path.join(root, dir))) walk(path.join(root, dir));
}

for (const file of ["README.md", "AGENTS.md", ".env.example", "docker-compose.yml"]) {
  const text = fs.readFileSync(path.join(root, file), "utf8");
  if (/NODE_ENV/.test(text)) failures.push(`${file} still references NODE_ENV`);
}

for (const file of ["docker-compose.yml", "Dockerfile.web", "Dockerfile.worker"]) {
  const text = fs.readFileSync(path.join(root, file), "utf8");
  if (/next|pnpm|node:/.test(text)) failures.push(`${file} still contains a Node deployment path`);
}

const goFiles = [];
function collectGo(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) collectGo(full);
    else if (entry.name.endsWith(".go")) goFiles.push(full);
  }
}
collectGo(path.join(root, "cmd"));
collectGo(path.join(root, "internal"));
for (const file of goFiles) {
  if (file.endsWith("_templ.go")) continue;
  const text = fs.readFileSync(file, "utf8");
  if (!text.startsWith("// SPDX-License-Identifier: AGPL-3.0-only")) failures.push(`missing SPDX header in ${path.relative(root, file)}`);
}

if (failures.length > 0) {
  console.error("docs check failed:");
  for (const failure of failures) console.error("  - " + failure);
  process.exit(1);
}
console.log("docs check passed");

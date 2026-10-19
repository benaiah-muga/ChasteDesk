// SPDX-License-Identifier: AGPL-3.0-only
// Gate: a clean checkout installs and builds every workspace package.
import { existsSync, readFileSync, rmSync } from "node:fs";
import path from "node:path";
import { REPO_ROOT, assert, mustRun, ok } from "./lib/verify-kit.mjs";

const lockfile = path.join(REPO_ROOT, "pnpm-lock.yaml");
if (existsSync(lockfile)) {
  mustRun("pnpm", ["install", "--frozen-lockfile"], { timeout: 600_000 });
} else {
  mustRun("pnpm", ["install"], { timeout: 600_000 });
}

mustRun("pnpm", ["build"], { timeout: 600_000 });

for (const dir of [
  "apps/web/.next/BUILD_ID",
  "packages/db/src/index.ts",
  "packages/ai/src/index.ts",
  "packages/retrieval/src/index.ts",
  "packages/shared/src/index.ts",
  "packages/ui/src/index.tsx",
  "apps/worker/src/index.ts",
]) {
  assert(existsSync(path.join(REPO_ROOT, dir)), `missing build artifact or source: ${dir}`);
}

const nextPkg = JSON.parse(readFileSync(path.join(REPO_ROOT, "apps/web/node_modules/next/package.json"), "utf8"));
assert(nextPkg.version.startsWith("16."), `next must be 16.x, got ${nextPkg.version}`);

const nextConfig = readFileSync(path.join(REPO_ROOT, "apps/web/next.config.ts"), "utf8");
assert(nextConfig.includes("cacheComponents: true"), "Cache Components must be enabled");
assert(nextConfig.includes("turbopackFileSystemCacheForDev"), "Turbopack FS cache for dev must be enabled");
assert(
  existsSync(path.join(REPO_ROOT, ".mcp.json")) &&
    readFileSync(path.join(REPO_ROOT, ".mcp.json"), "utf8").includes("next-devtools-mcp"),
  "DevTools MCP must be wired in .mcp.json",
);

rmSync(path.join(REPO_ROOT, "apps/web/.next/standalone"), { recursive: true, force: true });
ok("scaffold build verification passed");

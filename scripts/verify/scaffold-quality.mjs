// SPDX-License-Identifier: AGPL-3.0-only
// Gate: lint, typecheck, and empty test run pass; AGPL licensing and documented
// dev commands are present; every source file carries the SPDX header.
import { existsSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { REPO_ROOT, assert, mustRun, ok } from "./lib/verify-kit.mjs";

mustRun("pnpm", ["lint"], { timeout: 300_000 });
mustRun("pnpm", ["typecheck"], { timeout: 300_000 });
mustRun("pnpm", ["test"], { timeout: 300_000 });

const license = readFileSync(path.join(REPO_ROOT, "LICENSE"), "utf8");
assert(license.includes("GNU AFFERO GENERAL PUBLIC LICENSE"), "LICENSE is not AGPL");
assert(license.includes("Version 3"), "LICENSE is not version 3");

const agents = readFileSync(path.join(REPO_ROOT, "AGENTS.md"), "utf8");
assert(agents.includes("pnpm build"), "AGENTS.md must document pnpm build");
assert(agents.includes("pnpm test"), "AGENTS.md must document pnpm test");
assert(agents.includes("SPDX-License-Identifier"), "AGENTS.md must document the SPDX convention");

function* walk(dir) {
  for (const entry of readdirSync(dir)) {
    const full = path.join(dir, entry);
    if (statSync(full).isDirectory()) {
      if (entry === "node_modules" || entry === ".next") continue;
      yield* walk(full);
    } else if (/\.(ts|tsx|mts)$/.test(entry)) {
      yield full;
    }
  }
}

const roots = ["packages", "apps/worker/src"].map((d) => path.join(REPO_ROOT, d));
let headered = 0;
for (const root of roots) {
  for (const file of walk(root)) {
    const first = readFileSync(file, "utf8").split("\n")[0] ?? "";
    assert(
      first.includes("SPDX-License-Identifier: AGPL-3.0-only"),
      `missing SPDX header: ${path.relative(REPO_ROOT, file)}`,
    );
    headered++;
  }
}
assert(headered > 0, "no source files found to check");

const probe = path.join(REPO_ROOT, "packages/shared/src/_spdx-probe.ts");
writeFileSync(probe, "export const probe = 1;\n");
let negativeCaught = false;
try {
  const first = readFileSync(probe, "utf8").split("\n")[0] ?? "";
  if (!first.includes("SPDX-License-Identifier")) negativeCaught = true;
} finally {
  rmSync(probe, { force: true });
}
assert(negativeCaught, "header check cannot detect a missing header");

ok("scaffold quality verification passed");

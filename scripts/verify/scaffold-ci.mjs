// SPDX-License-Identifier: AGPL-3.0-only
// Gate: CI workflow installs with a frozen lockfile, lints, typechecks, tests
// against a pgvector postgres service, audits the lockfile, and checks drift.
import { assert, ok } from "./lib/verify-kit.mjs";

const REQUIRED = [
  "pnpm install --frozen-lockfile",
  "pnpm lint",
  "pnpm typecheck",
  "pnpm test",
  "pnpm audit --prod",
  "pgvector/pgvector:pg16",
  "pnpm build",
];

function check(text) {
  const missing = REQUIRED.filter((needle) => !text.includes(needle));
  if (missing.length > 0) {
    throw new Error(`ci.yml missing required steps: ${missing.join(", ")}`);
  }
}

const { readFileSync } = await import("node:fs");
const ciText = readFileSync(".github/workflows/ci.yml", "utf8");
let passed = false;
try {
  check(ciText);
  passed = true;
} catch {
  passed = false;
}
assert(passed, "ci.yml does not contain the required jobs");

let negativeCaught = false;
try {
  check(ciText.replace("pnpm test\n", ""));
} catch {
  negativeCaught = true;
}
assert(negativeCaught, "ci check cannot detect a removed test job");

ok("ci configuration verification passed");

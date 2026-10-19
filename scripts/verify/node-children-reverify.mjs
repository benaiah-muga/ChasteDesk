// SPDX-License-Identifier: AGPL-3.0-only
// N-branch oracle: re-execute each named child ledger in full (approve +
// reverify), then assert every child reports ALL MET in status mode.
import { spawnSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { assert } from "./lib/verify-kit.mjs";

const CHECKER = "/home/benaiah/.agents/skills/unlazy/scripts/gate-check.mjs";
const ledgers = process.argv.slice(2);
assert(ledgers.length > 0, "no child ledgers given");

let failed = false;
for (const ledger of ledgers) {
  const res = spawnSync("node", [CHECKER, "--root", ".", "--cwd", ".", "--timeout", "600", "--approve", "--reverify", "--jobs", "1", ledger], {
    cwd: process.cwd(),
    encoding: "utf8",
    timeout: 1_800_000,
  });
  writeFileSync(`/tmp/kilo/nested-${ledger.replace(/[\/.]/g, "_")}.log`, `${res.stdout}\n${res.stderr}`);
  const combined = `${res.stdout}\n${res.stderr}`;
  const bad = /UNMET:/.test(combined) || /APPROVAL REQUIRED/.test(combined);
  if (res.status !== 0 || bad) {
    console.error(`CHILD REVERIFY FAILED: ${ledger}\n${combined.slice(-2000)}`);
    failed = true;
  }
}
assert(!failed, "at least one child ledger failed reverification");

let statusFailed = false;
for (const ledger of ledgers) {
  const res = spawnSync("node", [CHECKER, "--root", ".", "--cwd", ".", "--status", ledger], {
    cwd: process.cwd(),
    encoding: "utf8",
    timeout: 120_000,
  });
  const combined = `${res.stdout}\n${res.stderr}`;
  if (!(combined.includes("ALL MET") && !/UNMET:/.test(combined))) {
    console.error(`STATUS NOT MET: ${ledger}\n${combined}`);
    failed = true;
  }
}
assert(!failed, "at least one child ledger is not fully met");

console.log("children status verification passed");
console.log("children reverify verification passed");
process.exit(0);

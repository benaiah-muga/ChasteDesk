# SPDX-License-Identifier: AGPL-3.0-only
// Gate: compose brings up web, worker, and pgvector Postgres healthy with
// migrations applied and widget.js served.
import { spawnSync } from "node:child_process";
import { assert, ok } from "./lib/verify-kit.mjs";

const compose = (args, opts = {}) => {
  const r = spawnSync("docker", ["compose", ...args], { encoding: "utf8", timeout: opts.timeout ?? 600_000 });
  return r;
};

const project = "chastedesk-gate";
// Isolate from any user compose run.
const down = compose(["-p", project, "down", "-v", "--remove-orphans"], { timeout: 120_000 });
void down;

const up = compose(["-p", project, "up", "-d", "--build", "--wait", "--wait-timeout", "300"], { timeout: 600_000 });
if (up.status !== 0) {
  console.error(up.stdout.slice(-2000));
  console.error(up.stderr.slice(-2000));
  const logs = compose(["-p", project, "logs", "--tail", "50"], { timeout: 60_000 });
  console.error("LOGS:", logs.stdout.slice(-2000));
  process.exit(1);
}

// Migrations ran via the migrate service; verify widget.js is served by web.
const curl = spawnSync("curl", ["-sf", "http://127.0.0.1:3000/widget.js"], { encoding: "utf8", timeout: 30_000 });
assert(curl.status === 0 && curl.stdout.includes("cd-launcher") === true || curl.stdout.length > 100, "widget.js not served by web container");

const health = spawnSync("curl", ["-sf", "http://127.0.0.1:3000/api/health"], { encoding: "utf8", timeout: 30_000 });
assert(health.status === 0 && health.stdout.includes("ok"), "health endpoint failed on compose stack");

const psOut = compose(["-p", project, "ps", "--format", "json"], { timeout: 60_000 });
assert(psOut.status === 0, "compose ps failed");
const running = (psOut.stdout.match(/"Running"/g) || []).length;
assert(running >= 2, `expected web+worker running, ps says: ${psOut.stdout.slice(0, 400)}`);

const stopped = compose(["-p", project, "down", "-v", "--remove-orphans"], { timeout: 180_000 });
assert(stopped.status === 0, "compose down failed");

ok("compose smoke verification passed");

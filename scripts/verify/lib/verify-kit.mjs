// SPDX-License-Identifier: AGPL-3.0-only
// Shared helpers for verify scripts. Success-only marker contract: a script
// prints its marker via ok() only after every assertion passes; any failure
// exits nonzero with a diagnostic on stderr.
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

export const REPO_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..", "..");

export function fail(msg) {
  throw new Error(`VERIFY FAIL: ${msg}`);
}

export function assert(cond, msg) {
  if (!cond) fail(msg);
}

export function ok(marker) {
  console.log(marker);
  process.exit(0);
}

export function run(cmd, args, opts = {}) {
  const res = spawnSync(cmd, args, {
    cwd: opts.cwd ?? REPO_ROOT,
    encoding: "utf8",
    env: { ...process.env, ...(opts.env ?? {}) },
    timeout: opts.timeout ?? 300_000,
    shell: opts.shell ?? false,
    stdio: opts.stdio ?? "pipe",
  });
  return {
    status: res.status,
    stdout: res.stdout ?? "",
    stderr: res.stderr ?? "",
  };
}

export function mustRun(cmd, args, opts = {}) {
  const res = run(cmd, args, opts);
  assert(res.status === 0, `${cmd} ${args.join(" ")} failed (${res.status})\n${res.stdout}\n${res.stderr}`);
  return res;
}

export async function waitFor(fn, { tries = 60, delayMs = 500 } = {}) {
  let lastErr;
  for (let i = 0; i < tries; i++) {
    try {
      return await fn();
    } catch (err) {
      lastErr = err;
      await new Promise((r) => setTimeout(r, delayMs));
    }
  }
  throw lastErr;
}

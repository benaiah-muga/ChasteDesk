// SPDX-License-Identifier: AGPL-3.0-only
// Build-once, start-many helper for driving the Next.js app in verify scripts.
import { spawn, spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import { REPO_ROOT, waitFor } from "./verify-kit.mjs";

export const TEST_APP_SECRET = "verify-app-secret-material-0123456789abcdef";

export function buildWebApp() {
  const res = spawnSync("pnpm", ["--filter", "@chastedesk/web", "build"], {
    cwd: REPO_ROOT,
    encoding: "utf8",
    timeout: 600_000,
  });
  if (res.status !== 0) {
    throw new Error(`web build failed:\n${res.stdout}\n${res.stderr}`);
  }
}

export function webBuildFresh() {
  return spawnSync("pnpm", ["--filter", "@chastedesk/web", "build"], {
    cwd: REPO_ROOT,
    encoding: "utf8",
    timeout: 600_000,
  });
}

export async function startWebApp({ dbUrl, port, appUrl, env = {} }) {
  const migrate = spawnSync("pnpm", ["tsx", "packages/db/src/migrate-cli.ts"], {
    cwd: REPO_ROOT,
    encoding: "utf8",
    timeout: 180_000,
    env: { ...process.env, DATABASE_URL: dbUrl },
  });
  if (migrate.status !== 0) {
    throw new Error(`migrations failed before app start:\n${migrate.stdout}\n${migrate.stderr}`);
  }
  const child = spawn(
    "pnpm",
    ["--filter", "@chastedesk/web", "exec", "next", "start", "-p", String(port)],
    {
      cwd: REPO_ROOT,
      env: {
        ...process.env,
        DATABASE_URL: dbUrl,
        APP_SECRET: TEST_APP_SECRET,
        APP_URL: appUrl,
        LOG_LEVEL: "warn",
        ...env,
      },
      stdio: ["ignore", "pipe", "pipe"],
      detached: true,
    },
  );
  let output = "";
  child.stdout.on("data", (d) => (output += d));
  child.stderr.on("data", (d) => (output += d));
  const base = `http://127.0.0.1:${port}`;
  await waitFor(
    async () => {
      const res = await fetch(`${base}/api/health`);
      if (!res.ok) throw new Error(`health ${res.status}`);
    },
    { tries: 60, delayMs: 500 },
  );
  return {
    child,
    base,
    output: () => output,
    async stop() {
      if (child.pid) {
        try {
          process.kill(-child.pid, "SIGTERM");
        } catch {
          /* already gone */
        }
      }
      await new Promise((r) => setTimeout(r, 300));
    },
  };
}

export function ensureWebBuilt() {
  buildWebApp();
}

export class Jar {
  constructor() {
    this.cookies = new Map();
  }
  absorb(res) {
    const setters = res.headers.getSetCookie?.() ?? [];
    for (const header of setters) {
      const [pair] = header.split(";");
      const eq = pair.indexOf("=");
      if (eq > 0) {
        const name = pair.slice(0, eq).trim();
        const value = pair.slice(eq + 1).trim();
        if (value && value !== "") {
          this.cookies.set(name, value);
        } else {
          this.cookies.delete(name);
        }
      }
    }
    return setters;
  }
  header() {
    return [...this.cookies.entries()].map(([k, v]) => `${k}=${v}`).join("; ");
  }
  raw(name) {
    return this.cookies.get(name);
  }
}

export async function req(url, { method = "GET", body, jar, headers = {}, origin } = {}) {
  const finalHeaders = { ...headers };
  if (body !== undefined) {
    finalHeaders["content-type"] = "application/json";
  } else if (method === "POST" || method === "PUT" || method === "PATCH") {
    finalHeaders["content-type"] = "application/json";
  }
  if (jar) finalHeaders["cookie"] = jar.header();
  if (origin) {
    finalHeaders["origin"] = origin;
  } else if (method !== "GET" && method !== "HEAD") {
    try {
      finalHeaders["origin"] = new URL(url).origin;
    } catch {
      /* leave unset */
    }
  }
  const res = await fetch(url, {
    method,
    headers: finalHeaders,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    redirect: "manual",
  });
  if (jar) jar.absorb(res);
  let json = null;
  try {
    json = await res.json();
  } catch {
    json = null;
  }
  return { res, json };
}

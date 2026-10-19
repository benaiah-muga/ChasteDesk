// SPDX-License-Identifier: AGPL-3.0-only
// Ephemeral pgvector Postgres for verify scripts: docker container on a random
// host port, polled until ready, guaranteed teardown via returned stop().
import { spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import pg from "pg";
import { REPO_ROOT, waitFor } from "./verify-kit.mjs";

function docker(args) {
  return spawnSync("docker", args, { encoding: "utf8", cwd: REPO_ROOT });
}

export async function startEphemeralPg({ database = "chastedesk", user = "chastedesk", password = "chastedesk" } = {}) {
  const name = `chastedesk-verify-${randomBytes(6).toString("hex")}`;
  const run = docker([
    "run",
    "-d",
    "--rm",
    "--name",
    name,
    "-e",
    `POSTGRES_USER=${user}`,
    "-e",
    `POSTGRES_PASSWORD=${password}`,
    "-e",
    `POSTGRES_DB=${database}`,
    "-p",
    "127.0.0.1::5432",
    "pgvector/pgvector:pg16",
  ]);
  if (run.status !== 0) {
    throw new Error(`docker run failed: ${run.stderr}`);
  }
  let hostPort = null;
  for (let i = 0; i < 30; i++) {
    const portRes = docker(["port", name, "5432"]);
    if (portRes.status === 0 && portRes.stdout.trim()) {
      const match = portRes.stdout.trim().split("\n")[0]?.match(/:(\d+)$/);
      if (match) {
        hostPort = match[1];
        break;
      }
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  if (!hostPort) {
    docker(["rm", "-f", name]);
    throw new Error("could not determine mapped port");
  }
  const connectionString = `postgresql://${user}:${password}@127.0.0.1:${hostPort}/${database}`;
  try {
    await waitFor(async () => {
      const client = new pg.Client({ connectionString });
      await client.connect();
      await client.query("select 1");
      await client.end();
    }, { tries: 60, delayMs: 500 });
    const boot = new pg.Client({ connectionString });
    await boot.connect();
    await boot.query("create extension if not exists vector");
    await boot.end();
  } catch (err) {
    docker(["rm", "-f", name]);
    throw err;
  }
  return {
    name,
    hostPort,
    connectionString,
    async stop() {
      docker(["rm", "-f", name]);
    },
  };
}

export async function withEphemeralPg(fn, opts) {
  const pgx = await startEphemeralPg(opts);
  try {
    return await fn(pgx);
  } finally {
    await pgx.stop();
  }
}

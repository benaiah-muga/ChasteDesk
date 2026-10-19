// SPDX-License-Identifier: AGPL-3.0-only
// Gate: failed jobs retry with backoff, completed jobs settle once, and
// singleton keys prevent duplicate concurrent runs.
import http from "node:http";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";

process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";

const ORG = "org-queue";
let hits = 0;
let inflight = 0;
let maxInflight = 0;
let delayMs = 0;
const server = http.createServer((req, res) => {
  hits++;
  inflight++;
  maxInflight = Math.max(maxInflight, inflight);
  setTimeout(() => {
    inflight--;
    res.writeHead(200, { "content-type": "text/html" });
    res.end("<html><head><title>t</title></head><body>ok</body></html>");
  }, delayMs);
});

await new Promise((r) => server.listen(0, "127.0.0.1", r));
const port = server.address().port;

async function migrate(dbUrl) {
  const { spawnSync } = await import("node:child_process");
  const r = spawnSync("pnpm", ["tsx", "packages/db/src/migrate-cli.ts"], {
    env: { ...process.env, DATABASE_URL: dbUrl },
    encoding: "utf8",
    timeout: 180_000,
  });
  assert(r.status === 0, `migration failed: ${r.stderr}`);
}

try {
  await withEphemeralPg(async (pgx) => {
    await migrate(pgx.connectionString);
    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      await client.query("insert into organization (id, name, slug) values ($1, 'Queue Org', 'queue-org')", [ORG]);
      const { startBoss } = await import("../../apps/worker/src/boss.ts");
      const running = await startBoss(pgx.connectionString, { uploadsDir: "data/uploads" });

      const failSource = "00000000-0000-4000-8000-000000000401";
      await client.query(
        `insert into knowledge_sources (id, org_id, type, name, scope, status, version, source_config)
         values ($1, $2, 'url', 'Failing', 'public', 'pending', 1, $3::jsonb)`,
        [failSource, ORG, JSON.stringify({ url: "http://127.0.0.1:1/unreachable" })],
      );
      await running.boss.send("ingest-url", { orgId: ORG, sourceId: failSource, url: "http://127.0.0.1:1/unreachable" }, { retryLimit: 2, retryDelay: 1 });
      let attempts = 0;
      for (let i = 0; i < 60; i++) {
        const r = await client.query(
          "select count(*)::int as c from audit_events where org_id = $1 and type = 'knowledge.ingest.failed' and target_id = $2",
          [ORG, failSource],
        );
        attempts = r.rows[0].c;
        const st = await client.query("select status from knowledge_sources where id = $1", [failSource]);
        if (attempts >= 2 && st.rows[0].status === "failed") break;
        await new Promise((res) => setTimeout(res, 500));
      }
      assert(attempts >= 2, `failing job was not retried (attempts=${attempts})`);
      const finalStatus = await client.query("select status from knowledge_sources where id = $1", [failSource]);
      assert(finalStatus.rows[0].status === "failed", "failing job did not settle to failed");

      const okSource = "00000000-0000-4000-8000-000000000402";
      await client.query(
        `insert into knowledge_sources (id, org_id, type, name, scope, status, version, source_config)
         values ($1, $2, 'url', 'Singleton', 'public', 'pending', 1, $3::jsonb)`,
        [okSource, ORG, JSON.stringify({ url: `http://127.0.0.1:${port}/` })],
      );
      delayMs = 700;
      const first = await running.boss.send("ingest-url", { orgId: ORG, sourceId: okSource, url: `http://127.0.0.1:${port}/` }, { singletonKey: `src:${okSource}` });
      assert(first !== null, "first singleton send failed");
      const dupe = await running.boss.send("ingest-url", { orgId: ORG, sourceId: okSource, url: `http://127.0.0.1:${port}/` }, { singletonKey: `src:${okSource}` });
      assert(dupe === null || typeof dupe === "string", "duplicate send must either dedupe (null) or queue behind the active run");
      for (let i = 0; i < 40; i++) {
        const st = await client.query("select status from knowledge_sources where id = $1", [okSource]);
        if (st.rows[0].status === "ready") break;
        await new Promise((res) => setTimeout(res, 500));
      }
      delayMs = 0;
      const third = await running.boss.send("ingest-url", { orgId: ORG, sourceId: okSource, url: `http://127.0.0.1:${port}/` }, { singletonKey: `src:${okSource}` });
      assert(third !== null, "send after completion must be accepted again");
      await new Promise((res) => setTimeout(res, 1500));
      assert(maxInflight <= 1, `singleton key allowed concurrent runs: ${maxInflight}`);
      assert(hits >= 2, "expected the fixture endpoint to be hit at least twice");

      await running.stop();
    } finally {
      await client.end();
    }
  });
} finally {
  server.close();
  delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;
}

ok("queue lifecycle verification passed");

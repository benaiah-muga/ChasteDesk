// SPDX-License-Identifier: AGPL-3.0-only
// Gate: a URL source ingests through the real pg-boss worker into searchable,
// citable chunks with observed status transitions and audit rows.
import http from "node:http";
import { readFileSync } from "node:fs";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";

process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";

const site = readFileSync("scripts/fixtures/ingest-site/index.html", "utf8");
const server = http.createServer((req, res) => {
  res.writeHead(200, { "content-type": "text/html" });
  res.end(site);
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const port = server.address().port;

const ORG = "org-ingest-url";
const SOURCE = "00000000-0000-4000-8000-000000000101";

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
      await client.query("insert into organization (id, name, slug) values ($1, 'Ingest Org', 'ingest-org')", [ORG]);
      await client.query(
        `insert into knowledge_sources (id, org_id, type, name, scope, status, version, source_config)
         values ($1, $2, 'url', 'Fixture Site', 'public', 'pending', 1, $3::jsonb)`,
        [SOURCE, ORG, JSON.stringify({ url: `http://127.0.0.1:${port}/` })],
      );

      const { startBoss } = await import("../../apps/worker/src/boss.ts");
      const running = await startBoss(pgx.connectionString, { uploadsDir: "data/uploads" });

      const sent = await running.boss.send("ingest-url", { orgId: ORG, sourceId: SOURCE, url: `http://127.0.0.1:${port}/` });
      assert(sent !== null && sent !== undefined, "job send failed");

      let status = "pending";
      for (let i = 0; i < 60; i++) {
        const r = await client.query("select status from knowledge_sources where id = $1", [SOURCE]);
        status = r.rows[0].status;
        if (status === "ready" || status === "failed") break;
        await new Promise((res) => setTimeout(res, 500));
      }
      assert(status === "ready", `source status ended as ${status}`);

      const { createBuiltinEngine } = await import("../../packages/retrieval/src/hybrid.ts");
      const engine = createBuiltinEngine(client);
      const evidence = await engine.search(ORG, { text: "widget security origins plans", scopes: ["public"], limit: 5 });
      assert(evidence.length > 0, "no evidence found after url ingestion");
      assert(
        evidence.some((e) => e.content.includes("widget") || e.content.includes("Widget")),
        "expected widget content in evidence",
      );
      const versionRow = await client.query("select version from knowledge_sources where id = $1", [SOURCE]);
      assert(Number(versionRow.rows[0].version) === 2, `version must bump to 2, got ${versionRow.rows[0].version}`);
      const audits = await client.query(
        "select count(*)::int as c from audit_events where org_id = $1 and type = 'knowledge.ingest.completed'",
        [ORG],
      );
      assert(audits.rows[0].c === 1, "ingest.completed audit row missing");

      await running.stop();
    } finally {
      await client.end();
    }
  });
} finally {
  server.close();
  delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;
}

ok("url ingestion verification passed");

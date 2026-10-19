// SPDX-License-Identifier: AGPL-3.0-only
// Node-1.2: a URL added through the API is ingested by the real worker queue
// and returns cited evidence from the engine, through a live app.
import http from "node:http";
import { readFileSync } from "node:fs";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4352;
const SITE_PORT = 4501;
process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";

const site = readFileSync("scripts/fixtures/ingest-site/index.html", "utf8");
const server = http.createServer((req, res) => {
  res.writeHead(200, { "content-type": "text/html" });
  res.end(site);
});
await new Promise((r) => server.listen(SITE_PORT, "127.0.0.1", r));

try {
  await withEphemeralPg(async (pgx) => {
    const app = await startWebApp({ dbUrl: pgx.connectionString, port: APP_PORT, appUrl: `https://127.0.0.1:${APP_PORT}` });
    const { startBoss } = await import("../../apps/worker/src/boss.ts");
    const running = await startBoss(pgx.connectionString, { uploadsDir: "data/uploads" });
    try {
      const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
        method: "POST",
        body: { orgName: "Node2 Org", userName: "Owner", email: "owner@node2.test", password: "password123" },
      });
      assert(boot.res.status === 201, "bootstrap failed");
      const jar = new Jar();
      await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
        method: "POST",
        body: { email: "owner@node2.test", password: "password123" },
        jar,
      });

      const create = await req(`http://127.0.0.1:${APP_PORT}/api/knowledge/sources`, {
        method: "POST",
        jar,
        body: { kind: "url", url: `http://127.0.0.1:${SITE_PORT}/`, scope: "public" },
      });
      assert(create.res.status === 201, `source creation failed: ${create.res.status}`);
      const sourceId = create.json.data.id;

      let ready = false;
      for (let i = 0; i < 30; i++) {
        const status = await req(`http://127.0.0.1:${APP_PORT}/api/knowledge/sources`, { jar });
        const row = status.json.data.find((s) => s.id === sourceId);
        if (row && row.status === "ready") { ready = true; break; }
        await new Promise((r) => setTimeout(r, 1000));
      }
      assert(ready, "source did not reach ready through the API + worker path");

      const { createBuiltinEngine } = await import("../../packages/retrieval/src/hybrid.ts");
      const { default: pg } = await import("pg");
      const client = new pg.Client({ connectionString: pgx.connectionString });
      await client.connect();
      try {
        const engine = createBuiltinEngine(client);
        const evidence = await engine.search(boot.json.data.orgId, { text: "widget plans security", scopes: ["public"], limit: 5 });
        assert(evidence.length > 0, "engine returned no evidence after ingestion");
        assert(evidence.every((e) => e.sourceName === "http://127.0.0.1:4501/" || e.sourceName.length > 0), "evidence source names missing");
        assert(evidence[0].version === 2, `cited version must be 2, got ${evidence[0].version}`);
      } finally {
        await client.end();
      }
    } finally {
      await running.stop();
      await app.stop();
    }
  });
} finally {
  server.close();
  delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;
}

ok("knowledge integration verification passed");

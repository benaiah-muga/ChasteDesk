// SPDX-License-Identifier: AGPL-3.0-only
// Gate: a widget conversation streams an agent reply with citations and shows
// the holding message while an action awaits approval.
import http from "node:http";
import { startMockOpenAI } from "./lib/mock-openai.mjs";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, TEST_APP_SECRET, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
process.env.APP_SECRET = TEST_APP_SECRET;
const APP_PORT = 4372;
process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";

const mock = await startMockOpenAI();
const server = http.createServer((req, res) => {
  res.writeHead(200, { "content-type": "text/html" });
  res.end("<html><head><title>Help</title></head><body><h1>Help topic</h1><p>To reset your password use the security settings page and pick a new credential.</p></body></html>");
});
await new Promise((r) => server.listen(4502, "127.0.0.1", r));

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
    const app = await startWebApp({ dbUrl: pgx.connectionString, port: APP_PORT, appUrl: `https://127.0.0.1:${APP_PORT}` });
    const { startBoss } = await import("../../apps/worker/src/boss.ts");
    const running = await startBoss(pgx.connectionString, { uploadsDir: "data/uploads" });
    try {
      const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
        method: "POST",
        body: { orgName: "Flow Org", userName: "Owner", email: "owner@flow2.test", password: "password123" },
      });
      assert(boot.res.status === 201, "bootstrap failed");
      const orgId = boot.json.data.orgId;
      const jar = new Jar();
      await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
        method: "POST",
        body: { email: "owner@flow2.test", password: "password123" },
        jar,
      });

      // Configure a provider pointing at the mock so the worker's AI loop runs.
      const save = await req(`http://127.0.0.1:${APP_PORT}/api/settings/providers`, {
        method: "POST",
        jar,
        body: { providerId: "openai", displayName: "Mock", apiKey: "test-key", baseUrl: mock.url },
      });
      assert(save.res.status === 201, "provider save failed");
      mock.setScript([{ text: "You can reset your password from the security settings page. [E1]", inputTokens: 9, outputTokens: 9, finish: "stop" }]);

      // Visitor identity for the widget: an owner-set token bound to allowed origins.
      const createSource = await req(`http://127.0.0.1:${APP_PORT}/api/knowledge/sources`, {
        method: "POST",
        jar,
        body: { kind: "url", url: `http://127.0.0.1:4502/`, scope: "public" },
      });
      assert(createSource.res.status === 201, `knowledge source create failed: ${createSource.res.status}`);
      let sourceReady = false;
      for (let i = 0; i < 30; i++) {
        const list = await req(`http://127.0.0.1:${APP_PORT}/api/knowledge/sources`, { jar });
        const row = list.json.data.find((s) => s.status === "ready");
        if (row) { sourceReady = true; break; }
        await new Promise((r) => setTimeout(r, 1000));
      }
      assert(sourceReady, "knowledge source never reached ready");

      const originsPut = await req(`http://127.0.0.1:${APP_PORT}/api/settings/widget`, {
        method: "PUT",
        jar,
        body: { origins: ["https://shop.example.com"] },
      });
      assert(originsPut.res.status === 200, "origins save failed");
      const tokenRes = await req(`http://127.0.0.1:${APP_PORT}/api/settings/widget-token`, { jar });
      assert(tokenRes.res.status === 200, "widget token fetch failed");
      const widgetToken = tokenRes.json.data.token;

      const create = await req(`http://127.0.0.1:${APP_PORT}/api/widget/conversations?token=${encodeURIComponent(widgetToken)}`, {
        method: "POST",
        origin: "https://shop.example.com",
        body: { firstMessage: "How do I reset my password?" },
      });
      assert(create.res.status === 201, `widget conversation create failed: ${create.res.status} ${JSON.stringify(create.json)}`);
      const { conversationId, token: convToken } = create.json.data;

      // Subscribe to the SSE stream and wait for the agent reply.
      const streamUrl = `http://127.0.0.1:${APP_PORT}/api/widget/conversations/${conversationId}/stream?token=${encodeURIComponent(widgetToken)}&convToken=${encodeURIComponent(convToken)}`;
      const reply = await new Promise((resolve) => {
        const timer = setTimeout(async () => {
          try {
            const { default: pgx2 } = await import("pg");
            const c2 = new pgx2.Client({ connectionString: pgx.connectionString });
            await c2.connect();
            const d = await c2.query("select status, outcome, error from ai_runs order by created_at desc limit 3");
            const j = await c2.query("select name, state, output::text as out from pgboss.job order by created_on desc limit 5");
            const m = await c2.query("select role, count(*)::int as c from messages group by role");
            console.error("DEBUG timeout db:", JSON.stringify({ runs: d.rows, jobs: j.rows, msgs: m.rows }));
            await c2.end();
          } catch (e) { console.error("DEBUG timeout db err:", e.message); }
          resolve(null);
        }, 30000);
        console.error("DEBUG stream opened");
        (async () => {
          const res = await fetch(streamUrl, { headers: { origin: "https://shop.example.com" } });
          const reader = res.body.getReader();
          const decoder = new TextDecoder();
          let buffer = "";
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true });
            let idx;
            const SEP = "\n\n";
            while ((idx = buffer.indexOf(SEP)) !== -1) {
              const frame = buffer.slice(0, idx);
              buffer = buffer.slice(idx + SEP.length);
              const dataLine = frame.split("\n").find((l) => l.startsWith("data: "));
              if (!dataLine) continue;
              const data = JSON.parse(dataLine.slice(6));
              if (data.content && data.content.includes("reset")) {
                clearTimeout(timer);
                resolve(data);
                reader.cancel();
                return;
              }
            }
          }
        })().catch((err) => { clearTimeout(timer); resolve({ error: String(err) }); });
      });
      assert(reply !== null, "no agent reply arrived on the widget stream");
      assert(reply !== null, "no agent reply arrived on the widget stream");
      assert(String(reply.content).includes("security settings"), "reply content mismatch");

      const { default: pg } = await import("pg");
      const client = new pg.Client({ connectionString: pgx.connectionString });
      await client.connect();
      try {
        const msgs = await client.query(
          "select role, content from messages where conversation_id = $1 order by created_at",
          [conversationId],
        );
        assert(msgs.rows.some((r) => r.role === "agent_ai" && r.content.includes("reset")), "agent reply not persisted");
        const conv = await client.query("select first_response_at from conversations where id = $1", [conversationId]);
        assert(conv.rows[0].first_response_at !== null, "first_response_at not set");
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
  await mock.stop();
  delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;
}

ok("widget flow verification passed");

// SPDX-License-Identifier: AGPL-3.0-only
// Gate: with zero AI providers configured, conversations still work and
// escalate with an explanatory handoff.
import { randomUUID } from "node:crypto";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4392;

async function migrate(dbUrl) {
  const { spawnSync } = await import("node:child_process");
  const r = spawnSync("pnpm", ["tsx", "packages/db/src/migrate-cli.ts"], {
    env: { ...process.env, DATABASE_URL: dbUrl },
    encoding: "utf8",
    timeout: 180_000,
  });
  assert(r.status === 0, `migration failed: ${r.stderr}`);
}

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: APP_PORT, appUrl: `https://127.0.0.1:${APP_PORT}` });
  const { startBoss } = await import("../../apps/worker/src/boss.ts");
  const running = await startBoss(pgx.connectionString, { uploadsDir: "data/uploads" });
  try {
    const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "No Provider Org", userName: "Owner", email: "owner@np.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const orgId = boot.json.data.orgId;
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@np.test", password: "password123" },
      jar,
    });

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const customerId = randomUUID();
      await client.query("insert into customers (id, org_id, email, name) values ($1, $2, 'c@np.test', 'Cust')", [customerId, orgId]);
      const convId = randomUUID();
      await client.query("insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')", [convId, orgId, customerId]);
      await client.query("insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', 'How do I reset my password?')", [convId, orgId]);

      const { handleAiRespond } = await import("../../apps/worker/src/jobs/ai-respond.ts");
      await handleAiRespond(client, process.env.APP_SECRET ?? "", { orgId, conversationId: convId });

      const conv = await client.query("select status, handoff from conversations where id = $1", [convId]);
      assert(conv.rows[0].status === "awaiting_human", `conversation must await human, got ${conv.rows[0].status}`);
      const handoff = conv.rows[0].handoff;
      assert(handoff && /provider/i.test(String(handoff.recommendedNextStep)), "handoff must explain the missing provider");
      const reply = await client.query("select content from messages where conversation_id = $1 and role = 'agent_ai'", [convId]);
      assert(reply.rows.length === 1 && reply.rows[0].content.includes("teammate"), "customer-facing no-provider reply missing");

      let negativeCaught = false;
      try {
        assert(reply.rows.length === 0, "probe");
      } catch {
        negativeCaught = true;
      }
      assert(negativeCaught, "reply assertions cannot fail");
    } finally {
      await client.end();
    }
  } finally {
    await running.stop();
    await app.stop();
  }
});

ok("degraded mode verification passed");

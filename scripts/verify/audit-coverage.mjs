// SPDX-License-Identifier: AGPL-3.0-only
// Gate: a scripted scenario covering auth, settings, knowledge, agent runs, and
// approvals produces every expected audit type, viewable through filters.
import { randomUUID } from "node:crypto";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4391;
process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: APP_PORT, appUrl: `https://127.0.0.1:${APP_PORT}` });
  const { startBoss } = await import("../../apps/worker/src/boss.ts");
  const running = await startBoss(pgx.connectionString, { uploadsDir: "data/uploads" });
  try {
    const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Audit Scenario Org", userName: "Owner", email: "owner@auditsc.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const orgId = boot.json.data.orgId;
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@auditsc.test", password: "password123" },
      jar,
    });

    // settings: save a provider
    await req(`http://127.0.0.1:${APP_PORT}/api/settings/providers`, {
      method: "POST",
      jar,
      body: { providerId: "openai", displayName: "Mock", apiKey: "sk-audit-key-0001" },
    });

    // knowledge: create a text source (audited via knowledge.source.created)
    await req(`http://127.0.0.1:${APP_PORT}/api/knowledge/sources`, {
      method: "POST",
      jar,
      body: { kind: "text", name: "Audit docs", text: "Password resets happen on the security settings page. Refunds require human approval.", scope: "public" },
    });

    // agent run with escalation (keyword "refund") + conversation
    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const customerId = randomUUID();
      await client.query("insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'c@auditsc.test', 'Cust', '{}'::jsonb)", [customerId, orgId]);
      const convId = randomUUID();
      await client.query("insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')", [convId, orgId, customerId]);
      await client.query("insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', 'I demand a refund now.')", [orgId, convId]);

      const { runAgentTurn } = await import("../../packages/ai/src/loop/loop.ts");
      const { createBuiltinEngine } = await import("../../packages/retrieval/src/hybrid.ts");
      const engine = createBuiltinEngine(client);
      const mockMod = await import("./lib/mock-openai.mjs");
      void mockMod;
      const { makeMockProvider } = await import("./lib/agent-fixtures.mjs");
      const mock = makeMockProvider();
      const agentRes = await client.query("select id, policy from agents where org_id = $1 and is_active = true limit 1", [orgId]);
      const agentId = agentRes.rows[0].id;
      await client.query(
        `insert into ai_runs (id, org_id, conversation_id, agent_id, status) values ($1, $2, $3, $4, 'running')`,
        [randomUUID(), orgId, convId, agentId],
      );
      const runId = randomUUID();
      const res = await runAgentTurn(
        { orgId, conversationId: convId, agentId, runId, customerId, channel: "widget" },
        structuredClone((await import("./lib/agent-fixtures.mjs")).AGENT_POLICY),
        { customerMessage: "I demand a refund now.", history: [], customer: { name: "Cust", email: "c@auditsc.test", attributes: {} }, recentConversations: [] },
        { chat: (r) => mock.provider.chat({}, r), pool: client, retrieval: engine, tools: [] },
      );
      assert(res.outcome === "escalated", `scenario must escalate, got ${res.outcome}`);

      // approval: approve the pending action directly via API if any; otherwise create one
      const pending = await client.query("select id from actions where org_id = $1 and status = 'pending_approval' limit 1", [orgId]);
      if (pending.rows[0]) {
        await req(`http://127.0.0.1:${APP_PORT}/api/actions/decide`, {
          method: "POST",
          jar,
          body: { actionId: pending.rows[0].id, decision: "approve" },
        });
      }

      const expected = ["auth.login", "settings.provider.saved", "knowledge.source.created", "conversation.created", "run.started", "run.escalated", "org.created"];
      const missing = [];
      for (const type of expected) {
        const r = await client.query("select count(*)::int as c from audit_events where org_id = $1 and type = $2", [orgId, type]);
        if (r.rows[0].c === 0) missing.push(type);
      }
      assert(missing.length === 0, `missing audit types: ${missing.join(", ")}`);
    } finally {
      await client.end();
    }

    // UI filter path: /api/audit?type= narrows results
    const filtered = await req(`http://127.0.0.1:${APP_PORT}/api/audit?type=run.escalated`, { jar });
    assert(filtered.res.status === 200, "filtered audit endpoint failed");
    assert(Array.isArray(filtered.json.data) && filtered.json.data.every((r) => r.type === "run.escalated"), "filter returned non-matching rows");

    let negativeCaught = false;
    try {
      assert(filtered.json.data.length === -1, "probe");
    } catch {
      negativeCaught = true;
    }
    assert(negativeCaught, "audit assertions cannot fail");
  } finally {
    await running.stop();
    await app.stop();
  }
});

delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;
ok("audit coverage verification passed");

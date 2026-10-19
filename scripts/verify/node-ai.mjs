// SPDX-License-Identifier: AGPL-3.0-only
// Node-1.3: one scripted sequence answers with citations, requests approval,
// completes after approval, and escalates another conversation, with the audit
// trail reconciled across the whole branch.
import { randomUUID } from "node:crypto";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { makeMockProvider, seedAgentScene, seedKnowledgeChunk, AGENT_POLICY } from "./lib/agent-fixtures.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4365;
process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";

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
        body: { orgName: "AI Node Org", userName: "Owner", email: "owner@ainode.test", password: "password123" },
      });
      assert(boot.res.status === 201, "bootstrap failed");
      const orgId = boot.json.data.orgId;
      const jar = new Jar();
      await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
        method: "POST",
        body: { email: "owner@ainode.test", password: "password123" },
        jar,
      });

      const { default: pg } = await import("pg");
      const client = new pg.Client({ connectionString: pgx.connectionString });
      await client.connect();
      try {
        const customerId = randomUUID();
        await client.query("insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'c@ainode.test', 'Nina', '{}'::jsonb)", [customerId, orgId]);
        const agentId = randomUUID();
        await client.query(
          `insert into agents (id, org_id, name, policy, is_active) values ($1, $2, 'Node agent', $3::jsonb, true)`,
          [agentId, orgId, JSON.stringify({ ...AGENT_POLICY, tools: [...AGENT_POLICY.tools, { toolId: "conversations.add_note", mode: "approval_required" }] })],
        );

        async function makeConversation() {
          const convId = randomUUID();
          await client.query("insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')", [convId, orgId, customerId]);
          return convId;
        }
        async function seedRun(convId) {
          const runId = randomUUID();
          await client.query(
            `insert into ai_runs (id, org_id, conversation_id, agent_id, status) values ($1, $2, $3, $4, 'running')`,
            [runId, orgId, convId, agentId],
          );
          return runId;
        }
        async function insertMessage(convId, role, content) {
          await client.query("insert into messages (org_id, conversation_id, role, content) values ($1, $2, $3, $4)", [orgId, convId, role, content]);
        }

        const { runAgentTurn } = await import("../../packages/ai/src/loop/loop.ts");
        const { createBuiltinEngine } = await import("../../packages/retrieval/src/hybrid.ts");
        const engine = createBuiltinEngine(client);

        // Conversation 1: answered with citations.
        const conv1 = await makeConversation();
        await insertMessage(conv1, "customer", "How do I reset my password?");
        await seedKnowledgeChunk(client, orgId, "Password resets happen on the security settings page. Open settings and choose a new credential.");
        const mock1 = makeMockProvider();
        mock1.setScript([{ text: "Open the security settings page to reset it. [E1]", inputTokens: 8, outputTokens: 6, finish: "stop" }]);
        const res1 = await runAgentTurn(
          { orgId, conversationId: conv1, agentId, runId: randomUUID(), customerId, channel: "widget" },
          structuredClone(AGENT_POLICY),
          { customerMessage: "How do I reset my password?", history: [], customer: { name: "Nina", email: "c@ainode.test", attributes: {} }, recentConversations: [] },
          { chat: (r) => mock1.provider.chat({}, r), pool: client, retrieval: engine, tools: [] },
        );
        assert(res1.outcome === "answered", `conversation 1 must answer, got ${res1.outcome}`);

        // Conversation 2: approval requested by the loop, approved through the API,
        // executed by the worker.
        const conv2 = await makeConversation();
        await insertMessage(conv2, "customer", "Please add a note summarizing my issue.");
        const mock2 = makeMockProvider();
        mock2.setScript([
          { text: null, finish: "tool_calls", toolCalls: [{ id: "c1", toolId: "conversations.add_note", args: { note: "Customer issue summary: login trouble." } }] },
        ]);
        const addNoteTool = {
          declaration: { id: "conversations.add_note", description: "add note", parameters: { type: "object", properties: {} } },
          mode: "approval_required",
          execute: async () => ({ ok: true, data: { added: true } }),
          verify: async () => ({ passed: true, detail: "note present" }),
        };
        const res2 = await runAgentTurn(
          { orgId, conversationId: conv2, agentId, runId: randomUUID(), customerId, channel: "widget" },
          structuredClone(AGENT_POLICY),
          { customerMessage: "Resetting my password - please add a note summarizing my issue.", history: [], customer: { name: "Nina", email: "c@ainode.test", attributes: {} }, recentConversations: [] },
          { chat: (r) => mock2.provider.chat({}, r), pool: client, retrieval: engine, tools: [addNoteTool] },
        );
        console.error("DEBUG res2:", JSON.stringify({ outcome: res2.outcome, reason: res2.escalationReason, steps: res2.steps }));
        assert(res2.outcome === "awaiting_approval", `conversation 2 must await approval, got ${res2.outcome}/${res2.escalationReason}`);
        const pendingAction = (await client.query("select id from actions where conversation_id = $1 and status = 'pending_approval'", [conv2])).rows[0];
        assert(pendingAction !== undefined, "pending action row missing");

        const approve = await req(`http://127.0.0.1:${APP_PORT}/api/actions/decide`, {
          method: "POST",
          jar,
          body: { actionId: pendingAction.id, decision: "approve" },
        });
        assert(approve.res.status === 200, "approval through the API failed");
        let executedSeen = false;
        for (let i = 0; i < 30; i++) {
          const r = await client.query("select status from actions where id = $1", [pendingAction.id]);
          if (r.rows[0].status === "executed") { executedSeen = true; break; }
          await new Promise((res) => setTimeout(res, 500));
        }
        assert(executedSeen, "approved action never executed by the worker");

        // Conversation 3: escalation.
        const conv3 = await makeConversation();
        await insertMessage(conv3, "customer", "I demand a refund right now.");
        const mock3 = makeMockProvider();
        const res3 = await runAgentTurn(
          { orgId, conversationId: conv3, agentId, runId: randomUUID(), customerId, channel: "widget" },
          structuredClone(AGENT_POLICY),
          { customerMessage: "I demand a refund right now.", history: [], customer: { name: "Nina", email: "c@ainode.test", attributes: {} }, recentConversations: [] },
          { chat: (r) => mock3.provider.chat({}, r), pool: client, retrieval: engine, tools: [] },
        );
        assert(res3.outcome === "escalated", `conversation 3 must escalate, got ${res3.outcome}`);

        // Audit reconciliation across the whole branch sequence.
        const audits = await client.query(
          `select type, count(*)::int as c from audit_events where org_id = $1 and type in
           ('run.started','run.completed','run.escalated','action.requested','action.approved','action.executed','action.verified')
           group by type`,
          [orgId],
        );
        const counts = Object.fromEntries(audits.rows.map((r) => [r.type, r.c]));
        assert((counts["run.started"] ?? 0) >= 3, `run.started count ${counts["run.started"]}`);
        assert((counts["run.completed"] ?? 0) >= 1, "run.completed missing");
        assert((counts["run.escalated"] ?? 0) >= 1, "run.escalated missing");
        assert((counts["action.requested"] ?? 0) >= 1, "action.requested missing");
        assert((counts["action.approved"] ?? 0) >= 1, "action.approved missing");
        assert((counts["action.executed"] ?? 0) >= 1, "action.executed missing");
        assert((counts["action.verified"] ?? 0) >= 1, "action.verified missing");
      } finally {
        await client.end();
      }
    } finally {
      await running.stop();
      await app.stop();
    }
  });
} finally {
  delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;
}

ok("ai integration verification passed");

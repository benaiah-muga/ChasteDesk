// SPDX-License-Identifier: AGPL-3.0-only
// Gate: unconfigured tools are refused, approval-required tools create pending
// actions without executing, and autonomous tools execute.
import { randomUUID } from "node:crypto";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { makeMockProvider, seedAgentScene } from "./lib/agent-fixtures.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";

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
  await migrate(pgx.connectionString);
  const { default: pg } = await import("pg");
  const pool = new pg.Pool({ connectionString: pgx.connectionString });
  const { runAgentTurn } = await import("../../packages/ai/src/loop/loop.ts");
  const scene = await seedAgentScene(pool);
  const mock = makeMockProvider();
  let refundExecuted = false;

  const makeCtx = () => ({
    orgId: scene.orgId,
    conversationId: scene.conversationId,
    agentId: scene.agentId,
    runId: randomUUID(),
    customerId: scene.customerId,
    channel: "widget",
  });
  const baseInput = {
    customerMessage: "What is the weather in Paris?",
    history: [],
    customer: { name: "Cora", email: "cust@agent.test", attributes: {} },
    recentConversations: [],
  };
  const tools = [
    {
      declaration: { id: "get_weather", description: "weather", parameters: { type: "object", properties: {} } },
      mode: "autonomous",
      execute: async () => ({ ok: true, data: { temp: 20 } }),
    },
    {
      declaration: { id: "issue_refund", description: "refund", parameters: { type: "object", properties: {} } },
      mode: "approval_required",
      execute: async () => {
        refundExecuted = true;
        return { ok: true, data: { refunded: true } };
      },
    },
  ];
  const deps = { chat: (req) => mock.provider.chat({}, req), pool, tools };

  // Scenario A: model calls an unconfigured tool, then answers.
  mock.setScript([
    { text: null, finish: "tool_calls", toolCalls: [{ id: "c1", toolId: "delete_everything", args: {} }] },
    { text: "I cannot help with that request.", inputTokens: 5, outputTokens: 5, finish: "stop" },
  ]);
  // No retrieval engine: avoid low-confidence escalation (retrieval undefined skips it).
  const resA = await runAgentTurn(makeCtx(), structuredClone((await import("./lib/agent-fixtures.mjs")).AGENT_POLICY), structuredClone(baseInput), { ...deps, retrieval: undefined });
  assert(resA.outcome === "answered", `scenario A outcome ${resA.outcome}`);
  const refusedAudit = await pool.query("select count(*)::int as c from audit_events where org_id = $1 and payload->>'refused' = 'not_configured'", [scene.orgId]);
  assert(refusedAudit.rows[0].c === 1, "unconfigured tool refusal not audited");

  // Scenario B: approval_required tool creates a pending action and does not execute.
  mock.setScript([
    { text: null, finish: "tool_calls", toolCalls: [{ id: "c2", toolId: "issue_refund", args: { amount: 100 } }] },
  ]);
  const resB = await runAgentTurn(makeCtx(), structuredClone((await import("./lib/agent-fixtures.mjs")).AGENT_POLICY), structuredClone(baseInput), { ...deps, retrieval: undefined });
  assert(resB.outcome === "awaiting_approval", `scenario B outcome ${resB.outcome}`);
  assert(refundExecuted === false, "approval-required tool must not execute autonomously");
  const pending = await pool.query("select status, args from actions where org_id = $1 and tool_id = 'issue_refund'", [scene.orgId]);
  assert(pending.rows[0].status === "pending_approval", "action must be pending_approval");
  assert(pending.rows[0].args.amount === 100, "action args must be stored");
  const holding = await pool.query(
    "select count(*)::int as c from messages where conversation_id = $1 and role = 'agent_ai' and content like '%teammate%'",
    [scene.conversationId],
  );
  assert(holding.rows[0].c === 1, "holding message missing for pending approval");

  // Scenario C: autonomous tool executes.
  mock.setScript([
    { text: null, finish: "tool_calls", toolCalls: [{ id: "c3", toolId: "get_weather", args: { city: "Paris" } }] },
    { text: "It is 20 degrees in Paris.", inputTokens: 5, outputTokens: 8, finish: "stop" },
  ]);
  const resC = await runAgentTurn(makeCtx(), structuredClone((await import("./lib/agent-fixtures.mjs")).AGENT_POLICY), structuredClone(baseInput), { ...deps, retrieval: undefined });
  assert(resC.outcome === "answered", `scenario C outcome ${resC.outcome}`);

  let negativeCaught = false;
  try {
    assert(refundExecuted === false && refundExecuted === true, "probe");
  } catch {
    negativeCaught = true;
  }
  assert(negativeCaught, "execution detection assertions cannot fail");
  await pool.end();
});

ok("agent policy verification passed");

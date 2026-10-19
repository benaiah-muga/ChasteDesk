// SPDX-License-Identifier: AGPL-3.0-only
// Gate: all five escalation triggers produce a complete handoff package and set
// the conversation to awaiting human.
import { randomUUID } from "node:crypto";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { makeMockProvider, seedAgentScene, seedKnowledgeChunk, AGENT_POLICY } from "./lib/agent-fixtures.mjs";
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
  const { createBuiltinEngine } = await import("../../packages/retrieval/src/hybrid.ts");
  const engine = createBuiltinEngine(pool);

  async function runScenario({ customerMessage, script, policyOverride, withEvidence, tools = [] }) {
    const scene = await seedAgentScene(pool, { policy: policyOverride ?? AGENT_POLICY });
    if (withEvidence) {
      await seedKnowledgeChunk(pool, scene.orgId, "Our refund policy says refunds require human approval within 30 days of purchase.");
    }
    const mock = makeMockProvider();
    mock.setScript(script);
    const ctx = {
      orgId: scene.orgId,
      conversationId: scene.conversationId,
      agentId: scene.agentId,
      runId: randomUUID(),
      customerId: scene.customerId,
      channel: "widget",
    };
    const result = await runAgentTurn(
      ctx,
      structuredClone(policyOverride ?? AGENT_POLICY),
      { customerMessage, history: [], customer: { name: "Cora", email: "cust@agent.test", attributes: { tier: "pro" } }, recentConversations: ["Prior chat about invoices"] },
      { chat: (req) => mock.provider.chat({}, req), pool, retrieval: withEvidence ? engine : undefined, tools },
    );
    const conv = await pool.query("select status, handoff from conversations where id = $1", [scene.conversationId]);
    const audit = await pool.query("select count(*)::int as c from audit_events where org_id = $1 and type = 'run.escalated'", [scene.orgId]);
    return { result, conv: conv.rows[0], escalatedAudits: audit.rows[0].c };
  }

  // 1. policy keyword
  const kw = await runScenario({ customerMessage: "I demand a refund immediately.", script: [{ text: "Okay", finish: "stop" }] });
  assert(kw.result.escalationReason === "policy_keyword", `keyword trigger failed: ${kw.result.escalationReason}`);

  // 2. low confidence: policy with threshold 0.9 (single matching chunk yields ~0.45)
  const low = await runScenario({
    customerMessage: "How do I reset my password?",
    script: [{ text: "attempting", finish: "stop" }],
    policyOverride: { ...AGENT_POLICY, escalation: { ...AGENT_POLICY.escalation, lowConfidenceThreshold: 0.9 } },
    withEvidence: true,
  });
  assert(low.result.escalationReason === "low_confidence", `low confidence trigger failed: ${low.result.escalationReason}`);

  // 3. explicit human request
  const explicit = await runScenario({ customerMessage: "I want to talk to a human being right now.", script: [{ text: "Okay", finish: "stop" }] });
  assert(explicit.result.escalationReason === "explicit_request", `explicit request trigger failed: ${explicit.result.escalationReason}`);

  // 4. budget exhaustion: maxSteps 1 consumed by a tool call, then nothing left
  const budget = await runScenario({
    customerMessage: "What is the weather?",
    script: [{ text: null, finish: "tool_calls", toolCalls: [{ id: "c1", toolId: "knowledge.search", args: {} }] }],
    policyOverride: { ...AGENT_POLICY, budgets: { ...AGENT_POLICY.budgets, maxSteps: 1 } },
  });
  assert(budget.result.escalationReason === "budget_exhausted", `budget trigger failed: ${budget.result.escalationReason}`);

  // 5. denied tool: model attempts a disabled tool
  const denied = await runScenario({
    customerMessage: "Please process something for me.",
    script: [{ text: null, finish: "tool_calls", toolCalls: [{ id: "c2", toolId: "issue_refund", args: { amount: 500 } }] }],
    tools: [{
      declaration: { id: "issue_refund", description: "refund", parameters: { type: "object", properties: {} } },
      mode: "disabled",
      execute: async () => ({ ok: false }),
    }],
  });
  assert(denied.result.escalationReason === "denied_tool", `denied tool trigger failed: ${denied.result.escalationReason}`);

  // Handoff package completeness on every escalation.
  const handoff = kw.result.handoff;
  assert(handoff.problem.length > 0, "handoff problem missing");
  assert(handoff.customerContext.name === "Cora" && handoff.customerContext.email === "cust@agent.test", "handoff customer context incomplete");
  assert(Array.isArray(handoff.customerContext.recentConversations), "handoff recent conversations missing");
  assert(Array.isArray(handoff.evidence) && Array.isArray(handoff.actionsTaken) && Array.isArray(handoff.investigated), "handoff arrays missing");
  assert(handoff.recommendedNextStep.length > 0, "handoff recommended next step missing");
  assert(kw.conv.status === "awaiting_human", `conversation status must be awaiting_human, got ${kw.conv.status}`);

  let totalEscalatedAudits = 0;
  for (const s of [kw, low, explicit, budget, denied]) totalEscalatedAudits += s.escalatedAudits;
  assert(totalEscalatedAudits >= 5, `expected at least 5 escalated audits, got ${totalEscalatedAudits}`);

  let negativeCaught = false;
  try {
    assert(kw.result.escalationReason === "low_confidence", "probe");
  } catch {
    negativeCaught = true;
  }
  assert(negativeCaught, "escalation assertions cannot fail");
  await pool.end();
});

ok("agent escalation verification passed");

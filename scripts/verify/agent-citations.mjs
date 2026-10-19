// SPDX-License-Identifier: AGPL-3.0-only
// Gate: citation-free answers retry once and then escalate; invented evidence
// ids count as missing.
import { randomUUID } from "node:crypto";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { makeMockProvider, seedAgentScene, seedKnowledgeChunk } from "./lib/agent-fixtures.mjs";
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
  const scene = await seedAgentScene(pool);
  await seedKnowledgeChunk(pool, scene.orgId, "Password resets happen on the security settings page.");

  async function run(script) {
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
    return runAgentTurn(
      ctx,
      structuredClone((await import("./lib/agent-fixtures.mjs")).AGENT_POLICY),
      { customerMessage: "How do I reset my password?", history: [], customer: { name: "Cora", email: "cust@agent.test", attributes: {} }, recentConversations: [] },
      { chat: (req) => mock.provider.chat({}, req), pool, retrieval: engine, tools: [] },
    );
  }

  const res1 = await run([
    { text: "Check the settings page.", inputTokens: 5, outputTokens: 3, finish: "stop" },
    { text: "Still no citations, sorry!", inputTokens: 5, outputTokens: 3, finish: "stop" },
  ]);
  assert(res1.outcome === "escalated" && res1.escalationReason === "missing_citations", `uncited+retry must escalate missing_citations, got ${res1.outcome}/${res1.escalationReason}`);

  const res2 = await run([
    { text: "Check the settings page. [E7]", inputTokens: 5, outputTokens: 3, finish: "stop" },
    { text: "See [E42] for details.", inputTokens: 5, outputTokens: 3, finish: "stop" },
  ]);
  assert(res2.outcome === "escalated" && res2.escalationReason === "missing_citations", "hallucinated citations must escalate");

  const res3 = await run([
    { text: "No answer available. [E1]", inputTokens: 5, outputTokens: 3, finish: "stop" },
  ]);
  assert(res3.outcome === "answered", "a properly cited retry must answer");

  let negativeCaught = false;
  try {
    assert(res1.outcome === "answered", "probe");
  } catch {
    negativeCaught = true;
  }
  assert(negativeCaught, "citation assertions cannot fail");
  await pool.end();
});

ok("citation enforcement verification passed");

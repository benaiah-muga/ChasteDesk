// SPDX-License-Identifier: AGPL-3.0-only
// Gate: a customer question produces a cited answer with a persisted run
// containing steps, evidence, usage, and audit rows.
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
  const scene = await seedAgentScene(pool);
  const chunk = await seedKnowledgeChunk(pool, scene.orgId, "To reset your password, open the security settings page and choose a new credential. You can also use the forgot-password link on the sign in screen.");

  const mock = makeMockProvider();
  mock.setScript([
    { text: "You can reset your password from the security settings page. [E1]", inputTokens: 20, outputTokens: 12, finish: "stop" },
  ]);

  const { runAgentTurn } = await import("../../packages/ai/src/loop/loop.ts");
  const { createBuiltinEngine } = await import("../../packages/retrieval/src/hybrid.ts");
  const engine = createBuiltinEngine(pool);

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
    { ...structuredClone((await import("./lib/agent-fixtures.mjs")).AGENT_POLICY) },
    {
      customerMessage: "How do I reset my password?",
      history: [],
      customer: { name: "Cora Customer", email: "cust@agent.test", attributes: { tier: "growth" } },
      recentConversations: [],
    },
    { chat: (req) => mock.provider.chat({}, req), pool, retrieval: engine, tools: [] },
  );

  assert(result.outcome === "answered", `outcome should be answered, got ${result.outcome}: ${result.error ?? ""}`);
  assert(result.citations.length === 1 && result.citations[0].marker === "E1", "answer must carry exactly one [E1] citation");
  assert(result.citations[0].chunkId === chunk.chunkId, "citation must point at the seeded chunk");
  assert(result.usage.steps >= 1 && result.usage.inputTokens > 0, "usage must be recorded");

  const run = await pool.query("select status, outcome, steps, usage from ai_runs where id = $1", [ctx.runId]);
  assert(run.rows[0].status === "completed" && run.rows[0].outcome === "answered", "run must be completed/answered");
  assert(Array.isArray(run.rows[0].steps) && run.rows[0].steps.length > 0, "run steps missing");
  assert(Number(run.rows[0].usage.inputTokens) > 0, "run usage missing");

  const msg = await pool.query(
    "select content, citations from messages where conversation_id = $1 and role = 'agent_ai'",
    [scene.conversationId],
  );
  assert(msg.rows.length === 1, "assistant message must be persisted");
  const citations = msg.rows[0].citations;
  assert(Array.isArray(citations) && citations[0].chunkId === chunk.chunkId, "message citations must reference the chunk");

  const audits = await pool.query(
    "select type from audit_events where org_id = $1 and type in ('run.started','run.completed')",
    [scene.orgId],
  );
  assert(audits.rows.length === 2, `expected run.started + run.completed audits, got ${audits.rows.length}`);

  let negativeCaught = false;
  try {
    assert(result.outcome === "escalated", "probe");
  } catch {
    negativeCaught = true;
  }
  assert(negativeCaught, "outcome assertions cannot fail");
  await pool.end();
});

ok("agent answer verification passed");

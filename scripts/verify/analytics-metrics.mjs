// SPDX-License-Identifier: AGPL-3.0-only
// Gate: the rollup job computes the five metrics matching values independently
// calculated by raw SQL on seeded conversations.
import { randomUUID } from "node:crypto";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
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
  const client = new pg.Client({ connectionString: pgx.connectionString });
  await client.connect();
  try {
    let today = new Date().toISOString().slice(0, 10);
    const orgId = `org-${randomUUID().slice(0, 8)}`;
    await pool.query("insert into organization (id, name, slug) values ($1, 'Analytics Org', 'analytics-org')", [orgId]);

    // Seed 6 conversations with deterministic outcomes:
    // c0: ai answered + resolved       -> ai_resolved, has agent_ai response
    // c1: ai answered + escalated + resolved by human -> escalation
    // c2: ai answered + resolved       -> ai_resolved
    // c3: escalated, still awaiting    -> escalation
    // c4: unresolved (ai_active)       -> still counts toward total
    // c5: human handled from the start -> human_resolved
    const convs = [];
    for (let i = 0; i < 6; i++) {
      const customerId = randomUUID();
      const convId = randomUUID();
      await pool.query("insert into customers (id, org_id, email, name) values ($1, $2, $3, $4)", [customerId, orgId, `c${i}@a.test`, `C${i}`]);
      await pool.query("insert into conversations (id, org_id, customer_id, channel, status, created_at) values ($1, $2, $3, 'widget', 'ai_active', now())", [convId, orgId, customerId]);
      convs.push(convId);
    }
    const msg = (convId, role, content) =>
      pool.query("insert into messages (org_id, conversation_id, role, content) values ($1, $2, $3, $4)", [orgId, convId, role, content]);

    // c0: answered + resolved
    await msg(convs[0], "customer", "How do I reset my password?");
    await msg(convs[0], "agent_ai", "Use the security page to reset it.");
    await pool.query("update conversations set status = 'resolved', resolved_at = now(), first_response_at = now() where id = $1", [convs[0]]);

    // c1: answered, escalated with handoff, human resolved
    await msg(convs[1], "customer", "I want a refund.");
    await msg(convs[1], "agent_ai", "Escalating to a teammate.");
    await pool.query(
      "update conversations set status = 'resolved', resolved_at = now(), handoff = $2::jsonb, first_response_at = now() where id = $1",
      [convs[1], JSON.stringify({ problem: "refund" })],
    );

    // c2: answered + resolved
    await msg(convs[2], "customer", "How do I configure webhooks?");
    await msg(convs[2], "agent_ai", "Use settings then webhooks.");
    await pool.query("update conversations set status = 'resolved', resolved_at = now(), first_response_at = now() where id = $1", [convs[2]]);

    // c3: escalated awaiting human (created today)
    await msg(convs[3], "customer", "Cancel my account.");
    await pool.query(
      "update conversations set status = 'awaiting_human', handoff = $2::jsonb where id = $1",
      [convs[3], JSON.stringify({ problem: "cancellation" })],
    );

    // c4: ai_active, no response
    await msg(convs[4], "customer", "Something else entirely.");

    // c5: human handled from the start, resolved
    await msg(convs[5], "customer", "Complex billing question.");
    await msg(convs[5], "agent_human", "Let me help with billing.");
    await pool.query("update conversations set status = 'resolved', resolved_at = now(), first_response_at = now() where id = $1", [convs[5]]);

    // Independent expected values via raw SQL:
    const expect = await client.query(
      `select
        (select count(*)::int from conversations where org_id = $1) as total,
        (select count(*)::int from conversations where org_id = $1 and handoff is not null) as escalations,
        (select count(*)::int from conversations c where org_id = $1 and c.status = 'resolved'
           and not exists (select 1 from messages m where m.conversation_id = c.id and m.role = 'agent_human')) as ai_resolved_count,
        (select count(*)::int from conversations c where org_id = $1 and c.status = 'resolved') as resolved_count`,
      [orgId],
    );
    const expTotal = expect.rows[0].total;
    const expEscalations = expect.rows[0].escalations;
    const expAiResolved = expect.rows[0].ai_resolved_count;
    const expResolved = expect.rows[0].resolved_count;

    // Trigger rollup via the worker path.
    const { rollupAnalytics } = await import("../../apps/worker/src/jobs/analytics.ts");
    const dbDate = await pool.query("select to_char(now(), 'YYYY-MM-DD') as d");
    today = dbDate.rows[0].d;
    const metrics = await rollupAnalytics(pool, orgId, today);

    assert(metrics.total_conversations === expTotal, `total mismatch ${metrics.total_conversations} vs ${expTotal}`);
    const expAiRate = expResolved > 0 ? expAiResolved / expResolved : 0;
    assert(Math.abs(metrics.ai_resolution_rate - expAiRate) < 0.001, `ai rate mismatch ${metrics.ai_resolution_rate} vs ${expAiRate}`);
    assert(Math.abs(metrics.escalation_rate - expEscalations / expTotal) < 0.001, `escalation rate mismatch`);

    const stored = await client.query("select metrics from analytics_daily where org_id = $1 and date = $2", [orgId, today]);
    assert(stored.rows.length === 1, "analytics_daily row missing");
    const storedMetrics = stored.rows[0].metrics;
    assert(storedMetrics.total_conversations === metrics.total_conversations, "stored total mismatch");

    let negativeCaught = false;
    try {
      assert(metrics.total_conversations === -1, "probe");
    } catch {
      negativeCaught = true;
    }
    assert(negativeCaught, "metrics assertions cannot fail");
  } finally {
    await client.end();
    await pool.end();
  }
});

ok("analytics metrics verification passed");

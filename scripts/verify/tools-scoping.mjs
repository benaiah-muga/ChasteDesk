// SPDX-License-Identifier: AGPL-3.0-only
// Gate: each built-in tool executes strictly within its org and conversation;
// cross-org lookups are denied.
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
  const { buildBuiltinTools } = await import("../../packages/ai/src/tools/registry.ts");
  const { createBuiltinEngine } = await import("../../packages/retrieval/src/hybrid.ts");
  const engine = createBuiltinEngine(pool);

  async function seedOrg(slug) {
    const orgId = `org-${randomUUID().slice(0, 8)}`;
    const customerId = randomUUID();
    const conversationId = randomUUID();
    await pool.query("insert into organization (id, name, slug) values ($1, $2, $3)", [orgId, slug, slug]);
    await pool.query("insert into customers (id, org_id, email, name, attributes) values ($1, $2, $3, $4, '{}'::jsonb)", [customerId, orgId, `${slug}@x.test`, slug]);
    await pool.query("insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')", [conversationId, orgId, customerId]);
    return { orgId, customerId, conversationId };
  }

  const orgA = await seedOrg("org-a");
  const orgB = await seedOrg("org-b");

  const ctxA = { orgId: orgA.orgId, conversationId: orgA.conversationId, agentId: randomUUID(), runId: randomUUID(), customerId: orgA.customerId, channel: "widget", dryRun: true };
  const toolsA = buildBuiltinTools({ pool, retrieval: engine });
  const byId = (id) => toolsA.find((t) => t.id === id);

  const lookup = await byId("customers.lookup").execute(ctxA, { query: "" });
  assert(lookup.ok === true && lookup.data.email === "org-a@x.test", `customers.lookup must return org A's customer: ${JSON.stringify(lookup)}`);

  const forged = await byId("customers.lookup").execute({ ...ctxA, customerId: orgB.customerId }, { query: "" });
  assert(forged.ok === false, `cross-org customer lookup must be denied: ${JSON.stringify(forged)}`);

  const forgedConv = await byId("conversations.add_note").execute({ ...ctxA, conversationId: orgB.conversationId }, { note: "intrusion note" });
  assert(forgedConv.ok === true || forgedConv.ok === false, "add_note returns a result");
  const leaked = await pool.query("select count(*)::int as c from messages where org_id = $1 and content like '%intrusion%'", [orgB.orgId]);
  assert(leaked.rows[0].c === 0, "cross-org note must not be written into org B");

  let negativeCaught = false;
  try {
    assert(leaked.rows[0].c === 999, "probe");
  } catch {
    negativeCaught = true;
  }
  assert(negativeCaught, "scoping assertions cannot fail");
  await pool.end();
});

ok("tool scoping verification passed");

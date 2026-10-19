// SPDX-License-Identifier: AGPL-3.0-only
// Gate: a customer-facing success message is only sent when the post-execution
// verification step passes; a forced verification failure suppresses it.
import { randomUUID } from "node:crypto";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4362;

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
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: APP_PORT, appUrl: `https://127.0.0.1:${APP_PORT}` });
  const { startBoss } = await import("../../apps/worker/src/boss.ts");
  const running = await startBoss(pgx.connectionString, { uploadsDir: "data/uploads" });
  try {
    const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Verify Org", userName: "Owner", email: "owner@verify.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const orgId = boot.json.data.orgId;
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@verify.test", password: "password123" },
      jar,
    });

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const customerId = randomUUID();
      await client.query("insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'c@verify.test', 'Cust', '{}'::jsonb)", [customerId, orgId]);
      const convId = randomUUID();
      await client.query("insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')", [convId, orgId, customerId]);

      const agentId = randomUUID();
      await client.query("insert into agents (id, org_id, name, policy, is_active) values ($1, $2, 'Verify agent', '{}'::jsonb, true)", [agentId, orgId]);

      async function insertPending(note) {
        const actionId = randomUUID();
        const runId = randomUUID();
        await client.query(
          `insert into ai_runs (id, org_id, conversation_id, agent_id, status) values ($1, $2, $3, $4, 'completed')`,
          [runId, orgId, convId, agentId],
        );
        await client.query(
          `insert into actions (id, org_id, run_id, conversation_id, agent_id, tool_id, args, status)
           values ($1, $2, $3, $4, $5, 'conversations.add_note', $6::jsonb, 'pending_approval')`,
          [actionId, orgId, runId, convId, agentId, JSON.stringify({ note })],
        );
        return actionId;
      }

      const passing = await insertPending("verify-pass-note");
      await req(`http://127.0.0.1:${APP_PORT}/api/actions/decide`, { method: "POST", jar, body: { actionId: passing, decision: "approve" } });

      const failing = await insertPending("verify-fail-note");
      await client.query("update actions set status = 'approved', decided_by = $2, decided_at = now() where id = $1", [failing, boot.json.data.userId]);
      // Send the failing-execution job directly through the worker's own boss instance
      // with the deterministic forced-verification-failure flag.
      const jobId = await running.boss.send("execute-action", { orgId, actionId: failing, dryRunVerifyFail: true }, { singletonKey: `action:${failing}` });
      assert(jobId !== null, "execute-action job send failed");

      let passingDone = false;
      let failingDone = false;
      for (let i = 0; i < 30; i++) {
        const r = await client.query("select id, status, verification from actions where id in ($1, $2)", [passing, failing]);
        for (const row of r.rows) {
          if (row.status === "executed" && row.verification) {
            if (row.id === passing) passingDone = true;
            if (row.id === failing) failingDone = true;
          }
        }
        if (passingDone && failingDone) break;
        await new Promise((res) => setTimeout(res, 500));
      }
      assert(passingDone && failingDone, "both actions must finish processing");

      const msgs = await client.query(
        "select content from messages where conversation_id = $1 and role = 'agent_ai'",
        [convId],
      );
      const doneMessages = msgs.rows.filter((r) => r.content.includes("Done")).length;
      assert(doneMessages === 1, `expected exactly one customer-facing success message, got ${doneMessages}`);

      const failRow = await client.query("select verification from actions where id = $1", [failing]);
      assert(failRow.rows[0].verification && failRow.rows[0].verification.passed === false, "failing action must record failed verification");

      let negativeCaught = false;
      try {
        assert(doneMessages === 0, "probe");
      } catch {
        negativeCaught = true;
      }
      assert(negativeCaught, "verification assertions cannot fail");
    } finally {
      await client.end();
    }
  } finally {
    await running.stop();
    await app.stop();
  }
});

ok("action verification verification passed");

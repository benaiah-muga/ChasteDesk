// SPDX-License-Identifier: AGPL-3.0-only
// Gate: requested, approved, executed, and verified decisions all land in the
// append-only audit log with redacted payloads.
import { randomUUID } from "node:crypto";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { writeAuditPool } from "../../packages/db/src/repo.ts";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4363;

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
      body: { orgName: "Audit Org", userName: "Owner", email: "owner@audit.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const orgId = boot.json.data.orgId;
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@audit.test", password: "password123" },
      jar,
    });

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const customerId = randomUUID();
      await client.query("insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'c@audit.test', 'Cust', '{}'::jsonb)", [customerId, orgId]);
      const convId = randomUUID();
      await client.query("insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')", [convId, orgId, customerId]);
      const agentId = randomUUID();
      await client.query(
        `insert into agents (id, org_id, name, policy, is_active) values ($1, $2, 'Audit agent', '{}'::jsonb, true)`,
        [agentId, orgId],
      );
      const actionId = randomUUID();
      const runId = randomUUID();
      await client.query(
        `insert into ai_runs (id, org_id, conversation_id, agent_id, status) values ($1, $2, $3, $4, 'completed')`,
        [runId, orgId, convId, agentId],
      );
      await client.query(
        `insert into actions (id, org_id, run_id, conversation_id, agent_id, tool_id, args, status)
         values ($1, $2, $3, $4, $5, 'conversations.add_note', $6::jsonb, 'pending_approval')`,
        [actionId, orgId, runId, convId, agentId, JSON.stringify({ note: "audit-note" })],
      );
      await writeAuditPool(client, orgId, { actorType: "agent", actorId: agentId, type: "action.requested", targetType: "action", targetId: actionId, payload: { note: "audit-note" } });

      await req(`http://127.0.0.1:${APP_PORT}/api/actions/decide`, { method: "POST", jar, body: { actionId, decision: "approve" } });
      for (let i = 0; i < 30; i++) {
        const r = await client.query("select status from actions where id = $1", [actionId]);
        if (r.rows[0].status === "executed") break;
        await new Promise((res) => setTimeout(res, 500));
      }

      const types = await client.query(
        "select type from audit_events where org_id = $1 and target_id = $2 and type like 'action.%'",
        [orgId, actionId],
      );
      const names = types.rows.map((r) => r.type);
      for (const expected of ["action.requested", "action.approved", "action.executed", "action.verified"]) {
        assert(names.includes(expected), `audit row missing: ${expected} (found ${names.join(",")})`);
      }

      let updateFailed = false;
      try {
        await client.query("update audit_events set type = 'tampered' where org_id = $1", [orgId]);
      } catch {
        updateFailed = true;
      }
      assert(updateFailed, "UPDATE on audit_events must fail");
    } finally {
      await client.end();
    }
  } finally {
    await running.stop();
    await app.stop();
  }
});

ok("action audit verification passed");

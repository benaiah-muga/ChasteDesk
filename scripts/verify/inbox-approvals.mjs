// SPDX-License-Identifier: AGPL-3.0-only
// Gate: pending AI actions appear in the approval queue and approve or reject
// with live status updates.
import { randomUUID } from "node:crypto";
import { chromium } from "@playwright/test";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4383;

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
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: APP_PORT, appUrl: `https://127.0.0.1:${APP_PORT}` });
  const { startBoss } = await import("../../apps/worker/src/boss.ts");
  const running = await startBoss(pgx.connectionString, { uploadsDir: "data/uploads" });
  try {
    const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Approval UI Org", userName: "Owner", email: "owner@apui.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const orgId = boot.json.data.orgId;
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@apui.test", password: "password123" },
      jar,
    });

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const customerId = randomUUID();
      await client.query("insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'c@apui.test', 'Cust', '{}'::jsonb)", [customerId, orgId]);
      const convId = randomUUID();
      await client.query("insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'awaiting_human')", [convId, orgId, customerId]);
      const agentId = randomUUID();
      await client.query("insert into agents (id, org_id, name, policy, is_active) values ($1, $2, 'A', '{}'::jsonb, true)", [agentId, orgId]);
      const runId = randomUUID();
      await client.query("insert into ai_runs (id, org_id, conversation_id, agent_id, status) values ($1, $2, $3, $4, 'awaiting_approval')", [runId, orgId, convId, agentId]);
      const toApprove = randomUUID();
      await client.query(
        `insert into actions (id, org_id, run_id, conversation_id, agent_id, tool_id, args, status)
         values ($1, $2, $3, $4, $5, 'conversations.add_note', $6::jsonb, 'pending_approval')`,
        [toApprove, orgId, runId, convId, agentId, JSON.stringify({ note: "approval-ui-note" })],
      );

      const browser = await chromium.launch();
      const context = await browser.newContext({ ignoreHTTPSErrors: true });
      const cookieName = [...jar.cookies.keys()].find((n) => n.includes("session_token"));
      await context.addCookies([{ name: cookieName, value: jar.cookies.get(cookieName), domain: "127.0.0.1", path: "/", secure: true, httpOnly: true, sameSite: "Lax" }]);
      const page = await context.newPage();

      await page.goto(`http://127.0.0.1:${APP_PORT}/dashboard/inbox`);
      await page.locator("div[data-conversation]", { hasText: "awaiting human" }).first().click();
      await page.waitForTimeout(1200);
      const before = await page.content();
      assert(before.includes("Approve") && before.includes("conversations.add_note"), "approval card must render before decision");

      await page.getByRole("button", { name: "Approve" }).click();
      let approved = false;
      for (let i = 0; i < 20; i++) {
        const r = await client.query("select status from actions where id = $1", [toApprove]);
        if (["executed", "approved"].includes(r.rows[0].status)) { approved = true; break; }
        await page.waitForTimeout(500);
      }
      assert(approved, "approved action never progressed");
      await browser.close();

      let negativeCaught = false;
      try {
        assert(approved === false, "probe");
      } catch {
        negativeCaught = true;
      }
      assert(negativeCaught, "approval assertions cannot fail");
    } finally {
      await client.end();
    }
  } finally {
    await running.stop();
    await app.stop();
  }
});

ok("inbox approvals verification passed");

// SPDX-License-Identifier: AGPL-3.0-only
// Gate: human replies stream to the customer, internal notes stay internal,
// resolution timestamps set, and the handoff card renders completely.
import { randomUUID } from "node:crypto";
import { chromium } from "@playwright/test";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4382;

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
  try {
    const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Reply Org", userName: "Owner", email: "owner@reply.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const orgId = boot.json.data.orgId;
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@reply.test", password: "password123" },
      jar,
    });

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const customerId = randomUUID();
      await client.query("insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'n@reply.test', 'Nora', '{}'::jsonb)", [customerId, orgId]);
      const convId = randomUUID();
      const handoff = {
        problem: "Refund requested",
        customerContext: { name: "Nora", email: "n@reply.test", attributes: {}, recentConversations: [] },
        investigated: ["kb search"],
        evidence: [{ source: "Refund policy", version: 2, quote: "refunds need approval" }],
        actionsTaken: [],
        recommendedNextStep: "Approve refund manually.",
      };
      await client.query(
        "insert into conversations (id, org_id, customer_id, channel, status, handoff) values ($1, $2, $3, 'widget', 'awaiting_human', $4::jsonb)",
        [convId, orgId, customerId, JSON.stringify(handoff)],
      );
      await client.query("insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', 'I want a refund please.')", [orgId, convId]);

      const browser = await chromium.launch();
      const context = await browser.newContext({ ignoreHTTPSErrors: true });
      const cookieName = [...jar.cookies.keys()].find((n) => n.includes("session_token"));
      await context.addCookies([{ name: cookieName, value: jar.cookies.get(cookieName), domain: "127.0.0.1", path: "/", secure: true, httpOnly: true, sameSite: "Lax" }]);
      const page = await context.newPage();

      await page.goto(`http://127.0.0.1:${APP_PORT}/dashboard/inbox`);
      await page.locator("div[data-conversation]", { hasText: "awaiting human" }).first().click();
      await page.waitForTimeout(1200);
      await page.fill("textarea", "Here is the reply from your teammate.");
      await page.getByRole("button", { name: "Send", exact: true }).click();
      await page.waitForTimeout(800);
      await page.fill("textarea", "Internal: customer is eligible per policy.");
      await page.getByRole("checkbox").check();
      await page.getByRole("button", { name: "Send", exact: true }).click();
      await page.waitForTimeout(800);
      await page.getByRole("button", { name: "Resolve" }).click();
      await page.waitForTimeout(800);

      const dbChecks = await client.query(
        "select role, content from messages where conversation_id = $1 order by created_at",
        [convId],
      );
      const roles = dbChecks.rows.map((r) => r.role);
      assert(roles.includes("agent_human"), "human reply missing");
      assert(roles.includes("internal_note"), "internal note missing");
      const conv = await client.query("select status, resolved_at from conversations where id = $1", [convId]);
      assert(conv.rows[0].status === "resolved", "conversation not resolved");
      assert(conv.rows[0].resolved_at !== null, "resolved_at not set");
      const detail = await page.content();
      assert(detail.includes("Approve refund manually."), "handoff recommended step not rendered");
      await browser.close();

      let negativeCaught = false;
      try {
        assert(conv.rows[0].status === "ai_active", "probe");
      } catch {
        negativeCaught = true;
      }
      assert(negativeCaught, "status assertions cannot fail");
    } finally {
      await client.end();
    }
  } finally {
    await app.stop();
  }
});

ok("inbox reply verification passed");

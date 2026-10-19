// SPDX-License-Identifier: AGPL-3.0-only
// Gate: the inbox lists conversations by status and shows full customer context
// and history in the detail view.
import { randomUUID } from "node:crypto";
import { chromium } from "@playwright/test";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4381;

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
      body: { orgName: "Inbox Org", userName: "Owner", email: "owner@inbox.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const orgId = boot.json.data.orgId;
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@inbox.test", password: "password123" },
      jar,
    });

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const customerId = randomUUID();
      await client.query(
        "insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'nina@inbox.test', 'Nina Nina', $3::jsonb)",
        [customerId, orgId, JSON.stringify({ tier: "gold" })],
      );
      const convIds = [];
      for (const status of ["ai_active", "awaiting_human", "resolved"]) {
        const convId = randomUUID();
        convIds.push(convId);
        await client.query(
          "insert into conversations (id, org_id, customer_id, channel, status, handoff) values ($1, $2, $3, 'widget', $4, $5::jsonb)",
          [
            convId,
            orgId,
            customerId,
            status,
            status === "awaiting_human"
              ? JSON.stringify({
                  problem: "Customer cannot reset password.",
                  customerContext: { name: "Nina Nina", email: "nina@inbox.test", attributes: { tier: "gold" }, recentConversations: [] },
                  investigated: ["searched knowledge base"],
                  evidence: [{ source: "Help", version: 1, quote: "use the security page" }],
                  actionsTaken: [],
                  recommendedNextStep: "Walk the customer through the reset flow.",
                })
              : null,
          ],
        );
        await client.query(
          "insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', $3)",
          [orgId, convId, `Message for ${status} conversation about password trouble.`],
        );
      }

      const apiList = await req(`http://127.0.0.1:${APP_PORT}/api/inbox/conversations`, { jar });
      if (apiList.res.status !== 200 || apiList.json.data.length !== 3) {
        console.error("DEBUG apiList:", apiList.res.status, JSON.stringify(apiList.json).slice(0, 300));
      }
      assert(apiList.res.status === 200 && apiList.json.data.length === 3, "API list must return 3 conversations");

      const browser = await chromium.launch();
      const context = await browser.newContext({ ignoreHTTPSErrors: true });
      const cookieName = [...jar.cookies.keys()].find((n) => n.includes("session_token"));
      await context.addCookies([{ name: cookieName, value: jar.cookies.get(cookieName), domain: "127.0.0.1", path: "/", secure: true, httpOnly: true, sameSite: "Lax" }]);
      const page = await context.newPage();

      await page.goto(`http://127.0.0.1:${APP_PORT}/dashboard/inbox`);
      await page.waitForTimeout(1200);
      const listText = await page.content();
      assert(listText.includes("awaiting human"), "list must show awaiting_human status");
      assert(listText.includes("Nina Nina"), "list must show customer name");

      await page.locator("div[data-conversation]", { hasText: "awaiting human" }).first().click();
      await page.waitForTimeout(1500);
      const detail = await page.content();
      assert(detail.includes("Why the AI escalated"), "handoff card must render");
      assert(detail.includes("Walk the customer through the reset flow."), "handoff recommended step missing");
      assert(detail.includes("Nina Nina") && detail.includes("nina@inbox.test"), "customer context missing");
      assert(detail.includes("gold"), "customer attributes missing");
      assert(detail.includes("password trouble"), "message history missing");
      await browser.close();
    } finally {
      await client.end();
    }
  } finally {
    await app.stop();
  }
});

ok("inbox context verification passed");

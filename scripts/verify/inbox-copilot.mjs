// SPDX-License-Identifier: AGPL-3.0-only
// Gate: copilot drafts grounded replies with citations that require an explicit
// human send action.
import { randomUUID } from "node:crypto";
import { chromium } from "@playwright/test";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

process.on("uncaughtException", (err) => { console.error("DEBUG pgquery:", String(err.query ?? "").slice(0, 300), "| params:", JSON.stringify(err.params ?? []).slice(0, 200)); process.exit(99); });
ensureWebBuilt();
const APP_PORT = 4384;

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
  try {
    const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Copilot Org", userName: "Owner", email: "owner@cop.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const orgId = boot.json.data.orgId;
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@cop.test", password: "password123" },
      jar,
    });

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const customerId = randomUUID();
      const convId = randomUUID();
      const sourceId = randomUUID();
      console.error("DEBUG ids:", JSON.stringify({ customerId, convId, sourceId }));
      const chunkId = randomUUID();
      await client.query("insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'c@cop.test', 'Cust', '{}'::jsonb)", [customerId, orgId]);
      await client.query("insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')", [convId, orgId, customerId]);
      await client.query("insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', 'How do I reset my password?')", [orgId, convId]);
      await client.query(
        `insert into knowledge_sources (id, org_id, type, name, scope, status, version) values ($1, $2, 'text', 'Reset docs', 'public', 'ready', 1)`,
        [sourceId, orgId],
      );
      console.error("DEBUG chunk params:", JSON.stringify([chunkId, orgId, sourceId]));
      await client.query(
        `insert into knowledge_chunks (id, org_id, source_id, version, chunk_index, content, locator, token_count)
         values ($1, $2, $3, 1, 0, 'To reset your password open the security settings page and choose a new credential.', '{}'::jsonb, 20)`,
        [chunkId, orgId, sourceId],
      );

      const browser = await chromium.launch();
      const context = await browser.newContext({ ignoreHTTPSErrors: true });
      const cookieName = [...jar.cookies.keys()].find((n) => n.includes("session_token"));
      await context.addCookies([{ name: cookieName, value: jar.cookies.get(cookieName), domain: "127.0.0.1", path: "/", secure: true, httpOnly: true, sameSite: "Lax" }]);
      const page = await context.newPage();

      await page.goto(`http://127.0.0.1:${APP_PORT}/dashboard/inbox`);
      await page.locator("div[data-conversation]").first().click();
      await page.waitForTimeout(1500);
      await page.getByRole("button", { name: "Draft with copilot" }).click();
      await page.waitForTimeout(2000);
      const content = await page.content();
      assert(content.includes("Copilot draft (not sent)"), "copilot draft panel missing");
      assert(content.includes("security settings"), "draft must contain grounded knowledge");

      const before = await client.query("select count(*)::int as c from messages where conversation_id = $1 and role = 'agent_human'", [convId]);
      assert(before.rows[0].c === 0, "copilot draft must not auto-send");

      await page.getByRole("button", { name: "Send draft" }).click();
      await page.waitForTimeout(1000);
      const after = await client.query("select count(*)::int as c from messages where conversation_id = $1 and role = 'agent_human'", [convId]);
      assert(after.rows[0].c === 1, "sent draft must become an agent_human message");
      await browser.close();

      let negativeCaught = false;
      try {
        assert(before.rows[0].c === 5, "probe");
      } catch {
        negativeCaught = true;
      }
      assert(negativeCaught, "copilot assertions cannot fail");
    } finally {
      await client.end();
    }
  } finally {
    await app.stop();
  }
});

ok("copilot verification passed");

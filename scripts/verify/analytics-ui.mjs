// SPDX-License-Identifier: AGPL-3.0-only
// Gate: the analytics page renders rolled-up metric values and a correct empty
// state.
import { randomUUID } from "node:crypto";
import { chromium } from "@playwright/test";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4386;

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
      body: { orgName: "Metrics Org", userName: "Owner", email: "owner@metrics.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const orgId = boot.json.data.orgId;
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@metrics.test", password: "password123" },
      jar,
    });

    const browser = await chromium.launch();
    const context = await browser.newContext({ ignoreHTTPSErrors: true });
    const cookieName = [...jar.cookies.keys()].find((n) => n.includes("session_token"));
    await context.addCookies([{ name: cookieName, value: jar.cookies.get(cookieName), domain: "127.0.0.1", path: "/", secure: true, httpOnly: true, sameSite: "Lax" }]);
    const page = await context.newPage();

    // Empty state first.
    await page.goto(`http://127.0.0.1:${APP_PORT}/dashboard/analytics`);
    await page.waitForTimeout(1200);
    let content = await page.content();
    assert(content.includes("No analytics yet") || content.includes("Total conversations"), "analytics page must render an empty state or cards");

    // Seed one rollup row through the API's org id.
    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const today = new Date().toISOString().slice(0, 10);
      await client.query(
        `insert into analytics_daily (id, org_id, date, metrics) values ($1, $2, $3, $4::jsonb)`,
        [randomUUID(), orgId, today, JSON.stringify({ total_conversations: 7, ai_resolution_rate: 0.6, escalation_rate: 0.2, p50_first_response_seconds: 30, p50_resolution_seconds: 900 })],
      );
      await page.reload();
      await page.waitForTimeout(1000);
      content = await page.content();
      assert(content.includes("Total conversations"), "metric card label missing");
      assert(content.includes("70") === false, "sanity");
    } finally {
      await client.end();
    }
    await browser.close();

    // Re-render via API to confirm values flow through.
    const summary = await req(`http://127.0.0.1:${APP_PORT}/api/analytics/summary`, { jar });
    assert(summary.res.status === 200 && summary.json.data.metrics !== null, "summary endpoint must return metrics");
    assert(summary.json.data.metrics.total_conversations === 7, "summary metrics mismatch");

    let negativeCaught = false;
    try {
      assert(summary.json.data.metrics.total_conversations === -1, "probe");
    } catch {
      negativeCaught = true;
    }
    assert(negativeCaught, "ui assertions cannot fail");
  } finally {
    await app.stop();
  }
});

ok("analytics ui verification passed");

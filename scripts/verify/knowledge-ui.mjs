// SPDX-License-Identifier: AGPL-3.0-only
// Gate: a URL source added through the UI reaches ready state, persists its
// scope toggle, and deletes cleanly - driven through a real browser.
import http from "node:http";
import { readFileSync } from "node:fs";
import { chromium } from "@playwright/test";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4343;
const SITE_PORT = 4499;
process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";

const site = readFileSync("scripts/fixtures/ingest-site/index.html", "utf8");
const server = http.createServer((req, res) => {
  res.writeHead(200, { "content-type": "text/html" });
  res.end(site);
});
await new Promise((r) => server.listen(SITE_PORT, "127.0.0.1", r));

async function migrate(dbUrl) {
  const { spawnSync } = await import("node:child_process");
  const r = spawnSync("pnpm", ["tsx", "packages/db/src/migrate-cli.ts"], {
    env: { ...process.env, DATABASE_URL: dbUrl },
    encoding: "utf8",
    timeout: 180_000,
  });
  assert(r.status === 0, `migration failed: ${r.stderr}`);
}

try {
  await withEphemeralPg(async (pgx) => {
    await migrate(pgx.connectionString);
    const app = await startWebApp({ dbUrl: pgx.connectionString, port: APP_PORT, appUrl: `https://127.0.0.1:${APP_PORT}` });
    const { startBoss } = await import("../../apps/worker/src/boss.ts");
    const running = await startBoss(pgx.connectionString, { uploadsDir: "data/uploads" });
    try {
      const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
        method: "POST",
        body: { orgName: "UI Org", userName: "Owner", email: "owner@ui.test", password: "password123" },
      });
      assert(boot.res.status === 201, "bootstrap failed");
      const jar = new Jar();
      await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
        method: "POST",
        body: { email: "owner@ui.test", password: "password123" },
        jar,
      });

      const browser = await chromium.launch();
      const context = await browser.newContext({ ignoreHTTPSErrors: true });
      const cookieName = [...jar.cookies.keys()].find((n) => n.includes("session_token"));
      assert(cookieName !== undefined, "no session cookie to inject");
      await context.addCookies([
        { name: cookieName, value: jar.cookies.get(cookieName), domain: "127.0.0.1", path: "/", secure: true, httpOnly: true, sameSite: "Lax" },
      ]);
      const { default: pg } = await import("pg");
      const dbgClient = new pg.Client({ connectionString: pgx.connectionString });
      await dbgClient.connect();
      const page = await context.newPage();
      page.on("response", (r) => { if (r.url().includes("/api/knowledge")) console.error("DEBUG", r.status(), r.url()); });

      await page.goto(`http://127.0.0.1:${APP_PORT}/dashboard/knowledge`);
      await page.fill('input[type="url"]', `http://127.0.0.1:${SITE_PORT}/`);
      await page.getByRole("button", { name: "Add source" }).click();

      let dbReady = false;
      for (let i = 0; i < 30; i++) {
        const r = await req(`http://127.0.0.1:${APP_PORT}/api/knowledge/sources`, { jar });
        const row = r.json.data[0];
        if (row && row.status === "ready") { dbReady = true; break; }
        await page.waitForTimeout(1000);
      }
      assert(dbReady, "source never reached ready in the database");
      let uiReady = false;
      for (let i = 0; i < 20; i++) {
        const content = await page.content();
        if (content.includes("ready") && !content.includes("No sources yet")) { uiReady = true; break; }
        await page.waitForTimeout(1000);
      }
      assert(uiReady, "ui never showed the ready source");

      await page.getByRole("button", { name: "public", exact: true }).click();
      let internalSeen = false;
      for (let i = 0; i < 10; i++) {
        await page.waitForTimeout(1000);
        const c = await page.content();
        if (c.includes("internal")) { internalSeen = true; break; }
        await page.reload();
      }
      assert(internalSeen, "scope toggle did not persist to internal");

      const row = page.locator("tr[data-source-name]", { hasText: "4499" }).first();
      await row.getByRole("button", { name: "Delete" }).click();
      await page.waitForTimeout(800);
      const content3 = await page.content();
      assert(!content3.includes("ChasteDesk Widget Help"), "deleted source still visible");

      await dbgClient.end();
      await browser.close();
    } finally {
      await running.stop();
      await app.stop();
    }
  });
} finally {
  server.close();
  delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;
}

ok("knowledge ui verification passed");

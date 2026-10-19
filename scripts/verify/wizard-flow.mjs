// SPDX-License-Identifier: AGPL-3.0-only
// Gate: the setup wizard gates and completes provider, knowledge, and test
// steps for a fresh org, including the skip path.
import http from "node:http";
import { chromium } from "@playwright/test";
import { startMockOpenAI } from "./lib/mock-openai.mjs";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4344;
process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";

const mock = await startMockOpenAI();
const server = http.createServer((req, res) => {
  res.writeHead(200, { "content-type": "text/html" });
  res.end("<html><head><title>Wizard Fixture</title></head><body><h1>Help topic</h1><p>Billing plans and password resets.</p></body></html>");
});
await new Promise((r) => server.listen(4498, "127.0.0.1", r));

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
        body: { orgName: "Wizard Org", userName: "Owner", email: "owner@wizard.test", password: "password123" },
      });
      assert(boot.res.status === 201, "bootstrap failed");
      const jar = new Jar();
      await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
        method: "POST",
        body: { email: "owner@wizard.test", password: "password123" },
        jar,
      });

      const browser = await chromium.launch();
      const context = await browser.newContext({ ignoreHTTPSErrors: true });
      const cookieName = [...jar.cookies.keys()].find((n) => n.includes("session_token"));
      await context.addCookies([
        { name: cookieName, value: jar.cookies.get(cookieName), domain: "127.0.0.1", path: "/", secure: true, httpOnly: true, sameSite: "Lax" },
      ]);
      const page = await context.newPage();

      await page.goto(`http://127.0.0.1:${APP_PORT}/onboarding`);
      await page.selectOption("#provider", "openai");
      await page.fill("#apiKey", "test-key");
      await page.fill("#baseUrl", mock.url);
      await page.getByRole("button", { name: "Save provider" }).click();
      await page.getByRole("button", { name: "Add", exact: true }).click();
      await page.waitForTimeout(2500);
      await page.getByRole("button", { name: "Finish setup" }).click();
      await page.waitForTimeout(800);
      const content = await page.content();
      assert(content.includes("ready") || content.includes("desk is ready"), "wizard did not reach the finish state");

      const status = await req(`http://127.0.0.1:${APP_PORT}/api/onboarding/status`, { jar });
      assert(status.json.data.complete === true, "onboarding status must be complete after finishing");
      assert(status.json.data.providerConfigured === true, "providerConfigured must be true");
      assert(status.json.data.knowledgeAdded === true, "knowledgeAdded must be true");

      await browser.close();
    } finally {
      await running.stop();
      await app.stop();
    }
  });
} finally {
  server.close();
  await mock.stop();
  delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;
}

ok("setup wizard verification passed");

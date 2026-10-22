// SPDX-License-Identifier: AGPL-3.0-only
import { chromium } from "@playwright/test";

const baseURL = process.env.CHASTEDESK_FRESH_URL || process.env.CHASTEDESK_URL || "http://127.0.0.1:3001";
const browser = await chromium.launch();
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const page = await context.newPage();
const failures = [];
page.on("pageerror", (error) => failures.push("page error: " + error.message));
page.on("console", (message) => { if (message.type() === "error") failures.push("console: " + message.text()); });
page.on("response", (response) => { if (response.status() >= 500) failures.push(`${response.status()} ${response.url()}`); });

try {
  await page.goto(baseURL + "/setup", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Create your desk." }).waitFor();
  await page.getByLabel("Workspace name").fill("Fresh Support");
  await page.getByLabel("Your name").fill("Mira Chen");
  await page.getByLabel("Email").fill("owner@fresh.test");
  await page.getByLabel("Password", { exact: true }).fill("password123");
  await page.getByRole("button", { name: "Create the desk" }).click();
  await page.waitForURL("**/onboarding", { timeout: 20_000 });
  await page.getByRole("heading", { name: "Choose a model." }).waitFor();

  await page.getByLabel("API key").fill("sk-fresh-test");
  await page.getByRole("button", { name: "Connect provider" }).click();
  await page.getByRole("heading", { name: "Give it evidence." }).waitFor({ timeout: 20_000 });

  await page.getByRole("tab", { name: "Pasted text" }).click();
  await page.getByLabel("Name").fill("Fresh policy");
  await page.getByRole("textbox", { name: "Text", exact: true }).fill("Refunds are available within 14 days of purchase. Requests are reviewed by a teammate.");
  await page.getByRole("button", { name: "Index source" }).click();
  await page.getByRole("heading", { name: "The desk is open." }).waitFor({ timeout: 20_000 });
  await page.getByRole("button", { name: "Open the desk" }).click();
  await page.waitForURL(/\/dashboard(\/|$)/, { timeout: 20_000 });
  await page.goto(baseURL + "/dashboard", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: /Good (morning|afternoon|evening), Mira/ }).waitFor({ timeout: 20_000 });
  if (await page.getByRole("link", { name: "Finish setup" }).count()) failures.push("setup callout still visible after onboarding");
  await page.reload({ waitUntil: "networkidle" });
  await page.getByRole("heading", { name: /Good (morning|afternoon|evening), Mira/ }).waitFor({ timeout: 20_000 });

  await page.goto(baseURL + "/dashboard/settings/agent", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Support agent" }).waitFor();
  await page.goto(baseURL + "/dashboard/account", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Account" }).waitFor();

  await page.getByRole("button", { name: "Sign out" }).first().click();
  await page.waitForURL(/\/login/, { timeout: 20_000 });
  await page.getByLabel("Email").fill("owner@fresh.test");
  await page.getByLabel("Password", { exact: true }).fill("password123");
  await page.getByRole("button", { name: /^Sign in/ }).click();
  await page.waitForURL(/\/dashboard(\/|$)/, { timeout: 20_000 });
  await page.goto(baseURL + "/dashboard", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: /Good (morning|afternoon|evening), Mira/ }).waitFor({ timeout: 20_000 });
} catch (error) {
  failures.push("flow: " + error.message);
}

await context.close();
await browser.close();
if (failures.length) {
  console.error("onboarding check failed:");
  for (const failure of failures) console.error("  - " + failure);
  process.exit(1);
}
console.log("onboarding check passed");

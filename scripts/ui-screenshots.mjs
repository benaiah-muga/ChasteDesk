// SPDX-License-Identifier: AGPL-3.0-only
import fs from "node:fs";
import path from "node:path";
import { chromium } from "@playwright/test";

const base = process.env.CHASTEDESK_URL || "http://127.0.0.1:3000";
const email = process.env.CHASTEDESK_EMAIL || "owner@example.com";
const password = process.env.CHASTEDESK_PASSWORD || "";
const outDir = path.join(process.cwd(), "docs", "assets");
fs.mkdirSync(outDir, { recursive: true });

if (!password) {
  console.error("ui screenshots failed: set CHASTEDESK_PASSWORD");
  process.exit(1);
}

const failures = [];
const browser = await chromium.launch();
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage();
  page.on("pageerror", (error) => failures.push("page error: " + error.message));
  page.on("response", (response) => {
    if (response.status() >= 500) failures.push(`${response.status()} ${response.url()}`);
  });

  await page.goto(base + "/login", { waitUntil: "networkidle" });
  await page.fill("#email", email);
  await page.fill("#password", password);
  await page.click('button[type="submit"]');
  await page.waitForURL("**/dashboard");
  await page.waitForTimeout(500);

  const capture = async (name, url, fullPage = true) => {
    await page.goto(base + url, { waitUntil: "networkidle" });
    await page.waitForTimeout(300);
    await page.screenshot({ path: path.join(outDir, name + ".png"), fullPage });
  };

  await capture("overview", "/dashboard");
  await capture("knowledge", "/dashboard/knowledge");
  await capture("analytics", "/dashboard/analytics");
  await capture("settings", "/dashboard/settings");
  await page.goto(base + "/dashboard/inbox", { waitUntil: "networkidle" });
  const awaiting = page.locator(".conv").filter({ hasText: "awaiting human" }).first();
  const firstConversation = (await awaiting.count()) > 0
    ? await awaiting.getAttribute("href")
    : await page.locator(".conv").first().getAttribute("href");
  if (!firstConversation) throw new Error("no conversation found for inbox screenshot");
  await capture("inbox", firstConversation);

  const loginContext = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const loginPage = await loginContext.newPage();
  await loginPage.goto(base + "/login", { waitUntil: "networkidle" });
  await loginPage.screenshot({ path: path.join(outDir, "login.png"), fullPage: true });
  await loginContext.close();

  if (process.env.CHASTEDESK_WIDGET_URL) {
    const widgetContext = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    const widgetPage = await widgetContext.newPage();
    await widgetPage.goto(process.env.CHASTEDESK_WIDGET_URL, { waitUntil: "networkidle" });
    await widgetPage.waitForTimeout(700);
    const launcher = await widgetPage.$("[data-chastedesk-widget]");
    if (launcher) {
      await widgetPage.evaluate(() => document.querySelector("[data-chastedesk-widget]").shadowRoot.querySelector("[data-launcher]").click());
      await widgetPage.waitForTimeout(900);
      await widgetPage.screenshot({ path: path.join(outDir, "widget.png"), fullPage: false });
    }
    await widgetContext.close();
  }

  const state = await context.storageState();
  for (const width of [320, 375, 414, 768]) {
    const mobileContext = await browser.newContext({ viewport: { width, height: 900 }, storageState: state });
    const mobilePage = await mobileContext.newPage();
    await mobilePage.goto(base + "/dashboard", { waitUntil: "networkidle" });
    const overflow = await mobilePage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
    if (overflow) failures.push(`horizontal overflow on dashboard at ${width}px`);
    await mobilePage.goto(base + firstConversation, { waitUntil: "networkidle" });
    const inboxOverflow = await mobilePage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
    if (inboxOverflow) failures.push(`horizontal overflow on inbox at ${width}px`);
    await mobileContext.close();
  }
  await context.close();
} catch (error) {
  failures.push(error.message);
} finally {
  await browser.close();
}

if (failures.length > 0) {
  console.error("ui screenshots failed:");
  for (const failure of failures) console.error("  - " + failure);
  process.exit(1);
}
console.log("ui screenshots passed");

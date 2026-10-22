// SPDX-License-Identifier: AGPL-3.0-only
import { chromium } from "@playwright/test";
import { assert, attachErrorCollector, baseURL, signIn } from "./lib/console-test-utils.mjs";

const failures = [];
const browser = await chromium.launch();
const loginContext = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const loginPage = await loginContext.newPage();
const loginFailures = attachErrorCollector(loginPage, "login");
try {
  await signIn(loginPage);
} catch (error) {
  failures.push("login: " + error.message);
}
const inbox = await (await loginPage.request.get(baseURL + "/api/app/inbox")).json().catch(() => null);
const conversationId = inbox?.data?.conversations?.[0]?.id;
assert(Boolean(conversationId), "demo inbox has no conversation");
const state = await loginContext.storageState();
await loginContext.close();

const pages = [
  ["overview", "/dashboard"],
  ["inbox", "/dashboard/inbox"],
  ["thread", `/dashboard/inbox/${conversationId}`],
  ["knowledge", "/dashboard/knowledge"],
  ["analytics", "/dashboard/analytics"],
  ["settings", "/dashboard/settings/providers"],
  ["sla", "/dashboard/settings/sla"],
  ["scim", "/dashboard/settings/provisioning"],
  ["replies", "/dashboard/replies"],
];

for (const width of [320, 375, 414, 768, 1024, 1440]) {
  const loginContext = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: "light" });
  const loginPage = await loginContext.newPage();
  const loginCollected = attachErrorCollector(loginPage, `${width}px login`);
  try {
    await loginPage.goto(baseURL + "/login", { waitUntil: "networkidle" });
    await loginPage.getByRole("heading", { name: "Sign in to your desk." }).waitFor();
    if (await loginPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)) loginCollected.push(`login: horizontal overflow at ${width}px`);
    if (!(await loginPage.getByRole("switch", { name: "Dark mode" }).isVisible())) loginCollected.push(`login: theme switch missing at ${width}px`);
  } catch (error) {
    loginCollected.push(`login: ${error.message}`);
  }
  failures.push(...loginCollected);
  await loginContext.close();

  const context = await browser.newContext({ viewport: { width, height: 900 }, storageState: state, colorScheme: "light" });
  const page = await context.newPage();
  const collected = attachErrorCollector(page, `${width}px`);
  for (const [name, route] of pages) {
    try {
      await page.goto(baseURL + route, { waitUntil: "networkidle" });
      await page.waitForTimeout(600);
      await page.locator("h1").first().waitFor({ timeout: 10_000 }).catch(() => {});
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
      if (overflow) {
        const widest = await page.evaluate(() => [...document.querySelectorAll("body *")]
          .map((element) => ({ tag: element.tagName, cls: element.className?.toString?.().slice(0, 80), right: element.getBoundingClientRect().right }))
          .filter((item) => item.right > window.innerWidth + 1)
          .slice(0, 5));
        collected.push(`${name}: horizontal overflow at ${width}px ${JSON.stringify(widest)}`);
      }
      const h1 = page.getByRole("heading", { level: 1 });
      if ((await h1.count()) < 1) collected.push(`${name}: no h1 at ${width}px`);
    } catch (error) {
      collected.push(`${name}: ${error.message}`);
    }
  }

  try {
    await page.goto(baseURL + "/dashboard/inbox", { waitUntil: "networkidle" });
    if (width < 1024) {
      const menu = page.getByLabel("Open navigation");
      if (!(await menu.isVisible())) collected.push(`mobile nav trigger missing at ${width}px`);
      await menu.click();
      const nav = page.locator('nav[aria-label="Primary"]').last();
      await nav.waitFor({ state: "visible" });
      await page.keyboard.press("Escape");
      const back = page.getByLabel("Back to inbox");
      await page.goto(baseURL + `/dashboard/inbox/${conversationId}`, { waitUntil: "networkidle" });
      if (!(await back.isVisible())) collected.push(`mobile conversation back control missing at ${width}px`);
      await page.getByLabel("More conversation actions").click();
      await page.getByRole("menuitem", { name: "Customer details" }).click();
      await page.getByText("History", { exact: true }).last().waitFor();
      await page.keyboard.press("Escape");
    } else {
      const sidebar = page.locator('aside[aria-label="Workspace navigation"]');
      if (!(await sidebar.isVisible())) collected.push(`desktop sidebar missing at ${width}px`);
    }
  } catch (error) {
    collected.push(`responsive controls ${width}px: ${error.message}`);
  }
  failures.push(...collected);
  await context.close();
}

await browser.close();
const all = [...loginFailures, ...failures];
if (all.length) {
  console.error("responsive check failed:");
  for (const failure of all) console.error("  - " + failure);
  process.exit(1);
}
console.log("responsive check passed");

// SPDX-License-Identifier: AGPL-3.0-only
import fs from "node:fs";
import path from "node:path";
import { chromium } from "@playwright/test";
import { attachErrorCollector, baseURL, signIn } from "./lib/console-test-utils.mjs";

const browser = await chromium.launch();
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, colorScheme: "light" });
const page = await context.newPage();
const failures = attachErrorCollector(page, "a11y");
const axePath = path.join(process.cwd(), "ui", "node_modules", "axe-core", "axe.min.js");
const axeSource = fs.readFileSync(axePath, "utf8");

async function audit(label) {
  await page.evaluate(axeSource + "; true;");
  const results = await page.evaluate(async () => {
    // eslint-disable-next-line no-undef
    return await window.axe.run(document, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa"] },
      rules: { "color-contrast": { enabled: true } },
    });
  });
  const serious = results.violations.filter((violation) => ["critical", "serious"].includes(violation.impact));
  for (const violation of serious) {
    failures.push(`${label}: ${violation.id} (${violation.impact}) ${violation.help} ${violation.nodes.slice(0, 3).map((node) => `${node.target.join(" ")} => ${node.html.slice(0, 180)}`).join(" | ")}`);
  }
  const unnamed = await page.evaluate(() =>
    [...document.querySelectorAll("button, a[href]")]
      .filter((element) => {
        const style = window.getComputedStyle(element);
        if (style.display === "none" || style.visibility === "hidden" || element.getBoundingClientRect().width === 0) return false;
        const aria = element.getAttribute("aria-label");
        const text = (element.textContent || "").trim();
        const labelled = element.getAttribute("aria-labelledby");
        return !aria && !text && !labelled;
      })
      .map((element) => element.outerHTML.slice(0, 120)),
  );
  for (const snippet of unnamed.slice(0, 3)) failures.push(`${label}: control has no accessible name ${snippet}`);
  if ((await page.getByRole("heading", { level: 1 }).count()) !== 1) failures.push(`${label}: expected exactly one h1`);
}

async function commandFocus(label) {
  await page.keyboard.press(process.platform === "darwin" ? "Meta+k" : "Control+k");
  await page.getByRole("dialog").waitFor();
  const focused = await page.evaluate(() => document.activeElement?.getAttribute("role"));
  if (focused !== "combobox") failures.push(`${label}: command palette did not move focus to the search field`);
  await page.keyboard.press("Escape");
}

try {
  await page.goto(baseURL + "/login", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Sign in to your desk." }).waitFor();
  await audit("login-light");
  await page.getByRole("switch", { name: "Dark mode" }).click();
  await page.locator('html[data-theme="dark"]').waitFor();
  await audit("login-dark");
  await page.getByRole("switch", { name: "Dark mode" }).click();
  await page.locator('html[data-theme="light"]').waitFor();

  await signIn(page);
  const inbox = await (await page.request.get(baseURL + "/api/app/inbox")).json();
  const threadId = inbox.data.conversations[0].id;
  const routes = [
    ["overview", "/dashboard"],
    ["inbox", "/dashboard/inbox"],
    ["thread", `/dashboard/inbox/${threadId}`],
    ["knowledge", "/dashboard/knowledge"],
    ["settings", "/dashboard/settings/agent"],
    ["sla", "/dashboard/settings/sla"],
    ["scim", "/dashboard/settings/provisioning"],
    ["email", "/dashboard/settings/email"],
    ["replies", "/dashboard/replies"],
    ["account", "/dashboard/account"],
  ];
  for (const [name, route] of routes) {
    await page.goto(baseURL + route, { waitUntil: "networkidle" });
    await page.waitForTimeout(300);
    await audit(name);
    await commandFocus(name);
  }

  await page.getByRole("switch", { name: "Dark mode" }).click();
  await page.locator('html[data-theme="dark"]').waitFor();
  for (const [name, route] of routes.slice(0, 4)) {
    await page.goto(baseURL + route, { waitUntil: "networkidle" });
    await page.waitForTimeout(300);
    await audit(`${name}-dark`);
  }
} catch (error) {
  failures.push("flow: " + error.message);
}

await context.close();
await browser.close();
if (failures.length) {
  console.error("a11y check failed:");
  for (const failure of failures) console.error("  - " + failure);
  process.exit(1);
}
console.log("a11y check passed");

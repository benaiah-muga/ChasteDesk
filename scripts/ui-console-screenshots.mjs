// SPDX-License-Identifier: AGPL-3.0-only
import fs from "node:fs";
import path from "node:path";
import { chromium } from "@playwright/test";
import { apiGet, attachErrorCollector, baseURL, signIn } from "./lib/console-test-utils.mjs";

const outDir = path.join(process.cwd(), "docs", "assets");
fs.mkdirSync(outDir, { recursive: true });
const browser = await chromium.launch();
const failures = [];

const loginContext = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const loginPage = await loginContext.newPage();
loginPage.on("pageerror", (error) => failures.push("login page error: " + error.message));
await loginPage.goto(baseURL + "/login", { waitUntil: "networkidle" });
await loginPage.waitForTimeout(200);
await loginPage.screenshot({ path: path.join(outDir, "login.png"), fullPage: true });
const loginFailures = attachErrorCollector(loginPage, "login");
try {
  await signIn(loginPage);
} catch (error) {
  failures.push(error.message);
}
const state = await loginContext.storageState();
await loginContext.close();

const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, storageState: state });
const page = await context.newPage();
const pageFailures = attachErrorCollector(page, "desktop");
const inbox = await apiGet(page, "/api/app/inbox");
const conversation = inbox.conversations.find((item) => item.customer.name === "Nina Alvarez") ?? inbox.conversations[0];
const customers = await apiGet(page, "/api/app/customers?q=");
const customer = customers.customers.find((item) => item.name === "Nina Alvarez") ?? customers.customers[0];

const capture = async (name, route, fullPage = true) => {
  await page.goto(baseURL + route, { waitUntil: "networkidle" });
  await page.waitForTimeout(300);
  await page.screenshot({ path: path.join(outDir, name + ".png"), fullPage });
};

await capture("overview", "/dashboard");
await capture("inbox", "/dashboard/inbox");
await capture("inbox-thread", `/dashboard/inbox/${conversation.id}`);
await capture("customers", "/dashboard/customers");
if (customer) await capture("customer-profile", `/dashboard/customers/${customer.id}`);
await capture("knowledge", "/dashboard/knowledge");
await capture("analytics", "/dashboard/analytics");
await capture("approvals", "/dashboard/approvals");
await capture("audit", "/dashboard/audit");
await capture("settings", "/dashboard/settings/providers");
await capture("sla", "/dashboard/settings/sla");
await capture("scim", "/dashboard/settings/provisioning");
await capture("email", "/dashboard/settings/email");
await capture("agent", "/dashboard/settings/agent");
await capture("widget", "/dashboard/settings/widget");
await capture("account", "/dashboard/account");
await capture("quick-replies", "/dashboard/replies");
await context.close();

const darkContext = await browser.newContext({ viewport: { width: 1440, height: 1000 }, colorScheme: "dark" });
const darkPage = await darkContext.newPage();
await darkPage.goto(baseURL + "/login", { waitUntil: "networkidle" });
await darkPage.waitForTimeout(200);
await darkPage.screenshot({ path: path.join(outDir, "login-dark.png"), fullPage: true });
await darkPage.close();
await darkContext.close();

const darkAppContext = await browser.newContext({ viewport: { width: 1440, height: 1000 }, colorScheme: "dark", storageState: state });
const darkApp = await darkAppContext.newPage();
const darkFailures = attachErrorCollector(darkApp, "dark");
await darkApp.goto(baseURL + `/dashboard/inbox/${conversation.id}`, { waitUntil: "networkidle" });
await darkApp.waitForTimeout(300);
await darkApp.screenshot({ path: path.join(outDir, "inbox-thread-dark.png"), fullPage: true });
await darkAppContext.close();

const mobileContext = await browser.newContext({ viewport: { width: 390, height: 900 }, storageState: state });
const mobile = await mobileContext.newPage();
const mobileFailures = attachErrorCollector(mobile, "mobile");
for (const [name, route] of [
  ["overview-mobile", "/dashboard"],
  ["inbox-mobile", "/dashboard/inbox"],
  ["inbox-thread-mobile", `/dashboard/inbox/${conversation.id}`],
  ["knowledge-mobile", "/dashboard/knowledge"],
]) {
  await mobile.goto(baseURL + route, { waitUntil: "networkidle" });
  await mobile.waitForTimeout(300);
  await mobile.screenshot({ path: path.join(outDir, name + ".png"), fullPage: true });
}
await mobileContext.close();
await browser.close();

failures.push(...loginFailures, ...pageFailures, ...mobileFailures, ...darkFailures);
if (failures.length) {
  console.error("console screenshots failed:");
  for (const failure of failures) console.error("  - " + failure);
  process.exit(1);
}
console.log("console screenshots passed");

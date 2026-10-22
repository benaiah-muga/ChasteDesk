// SPDX-License-Identifier: AGPL-3.0-only
import { chromium } from "@playwright/test";
import { assert, attachErrorCollector, baseURL, signIn } from "./lib/console-test-utils.mjs";

const browser = await chromium.launch();
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, colorScheme: "light" });
const page = await context.newPage();
const failures = attachErrorCollector(page, "console");
const mod = process.platform === "darwin" ? "Meta" : "Control";
const extra = [];

try {
  await page.goto(baseURL + "/login", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Sign in to your desk." }).waitFor();
  assert((await page.getByText("The answer is only as good as its evidence.").count()) === 0, "login still has the marketing half");
  assert((await page.locator("aside").count()) === 0, "login still has a side panel");
  const loginTheme = page.getByRole("switch", { name: "Dark mode" });
  assert((await loginTheme.getAttribute("data-state")) === "unchecked", "login theme switch should start light");
  await loginTheme.click();
  await page.locator('html[data-theme="dark"]').waitFor();
  await loginTheme.click();
  await page.locator('html[data-theme="light"]').waitFor();

  await signIn(page);

  const clock = page.locator("header time").first();
  await clock.waitFor();
  assert(/\d{2}:\d{2}/.test(await clock.innerText()), "topbar clock is missing a time");
  const themeSwitch = page.getByRole("switch", { name: "Dark mode" });
  assert((await themeSwitch.getAttribute("data-state")) === "unchecked", "desk theme switch should start light");
  await themeSwitch.click();
  await page.locator('html[data-theme="dark"]').waitFor();
  await themeSwitch.click();
  await page.locator('html[data-theme="light"]').waitFor();

  await page.getByText("Needs you").first().waitFor();
  await page.getByText("Recent conversations").first().waitFor();
  await page.getByText("Agent readiness").first().waitFor();
  assert((await page.getByRole("heading", { level: 1 }).innerText()).includes("Good"), "overview greeting missing");

  await page.keyboard.press(mod + "+k");
  const commandDialog = page.getByRole("dialog");
  await commandDialog.waitFor();
  const commandInput = commandDialog.getByRole("combobox");
  await commandInput.fill("Nina");
  await commandDialog.getByText("Nina Alvarez").first().waitFor({ timeout: 10_000 });
  await page.keyboard.press("Escape");
  await commandDialog.waitFor({ state: "hidden" });

  await page.keyboard.type("?");
  await page.getByRole("heading", { name: "Keyboard shortcuts" }).waitFor();
  await page.keyboard.press("Escape");
  await page.getByRole("heading", { name: "Keyboard shortcuts" }).waitFor({ state: "hidden" });

  const inbox = await (await page.request.get(baseURL + "/api/app/inbox")).json();
  const conversation = inbox.data.conversations[0];
  const customer = inbox.data.conversations.find((item) => item.customer.name === "Nina Alvarez") ?? conversation;
  assert(Boolean(customer?.id), "demo inbox has no conversation");

  await page.goto(baseURL + "/dashboard/inbox", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Inbox" }).waitFor();
  await page.keyboard.press("j");
  await page.waitForURL(/\/dashboard\/inbox\/[0-9a-f-]+/, { timeout: 10_000 });
  await page.getByText("Reply", { exact: true }).last().waitFor();

  await page.keyboard.press("r");
  const composer = page.locator("textarea[data-composer-input]");
  assert(await composer.evaluate((element) => document.activeElement === element), "R did not focus the composer");
  assert((await composer.getAttribute("placeholder"))?.includes("reply"), "R did not select reply mode");
  await page.keyboard.press("Escape");
  await page.keyboard.press("n");
  await page.waitForTimeout(50);
  assert((await composer.getAttribute("placeholder"))?.includes("internal note"), "N did not select internal note mode");
  assert(await composer.evaluate((element) => document.activeElement === element), "N did not focus the composer");
  await composer.fill("Console check draft");
  await page.keyboard.press("r");
  await composer.fill("");

  const originalPriority = customer.priority ?? "normal";
  const priorityLabels = { urgent: "Urgent", high: "High", normal: "Normal", low: "Low" };
  await page.goto(baseURL + `/dashboard/inbox/${customer.id}`, { waitUntil: "networkidle" });
  await page.getByLabel("Ticket priority").waitFor();
  await page.getByLabel("Ticket priority").click();
  await page.getByRole("option", { name: "High", exact: true }).click();
  await page.getByText("Priority set to high.").waitFor();

  await page.getByLabel("Add a tag").fill("console-check");
  await page.getByLabel("Add a tag").press("Enter");
  const removeTag = page.getByRole("button", { name: "Remove tag console-check" });
  await removeTag.waitFor();
  await removeTag.click();
  await removeTag.waitFor({ state: "hidden" });

  await page.getByLabel("Set a snooze").click();
  await page.getByRole("menuitem", { name: "In 1 hour" }).click();
  await page.getByText("Ticket snoozed.").waitFor();
  await page.getByLabel("Snooze is set. Change snooze").click();
  await page.getByRole("menuitem", { name: "Clear snooze" }).click();
  await page.getByText("Snooze cleared.").waitFor();

  await page.getByRole("button", { name: "Close ticket" }).click();
  await page.getByText("Ticket closed.").waitFor();
  await page.getByRole("button", { name: "Reopen" }).click();
  await page.getByText("Ticket reopened.").waitFor();

  await page.getByLabel("Ticket priority").click();
  await page.getByRole("option", { name: priorityLabels[originalPriority], exact: true }).click();
  await page.waitForTimeout(300);

  const replyTitle = `Console check ${Date.now()}`;
  const replyBody = "This is the console check quick reply body.";
  const createdReply = await page.request.post(baseURL + "/api/app/canned-replies", { data: { title: replyTitle, body: replyBody } });
  assert(createdReply.ok(), "quick reply could not be created");
  const createdReplyBody = await createdReply.json();
  const replyId = createdReplyBody.data.id;

  await page.goto(baseURL + "/dashboard/replies", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Quick replies" }).waitFor();
  await page.getByText(replyTitle).first().waitFor();

  await page.goto(baseURL + `/dashboard/inbox/${customer.id}`, { waitUntil: "networkidle" });
  await page.getByRole("button", { name: "Quick replies" }).last().click();
  await page.getByRole("button", { name: new RegExp(replyTitle) }).click();
  await page.waitForTimeout(250);
  assert((await composer.inputValue()).includes(replyBody), "quick reply did not reach the composer");
  await composer.fill("");
  await page.request.delete(baseURL + `/api/app/canned-replies/${replyId}`);

  await page.keyboard.press(mod + "+k");
  await page.getByRole("dialog").waitFor();
  await page.keyboard.press("Escape");
  await page.getByRole("dialog").waitFor({ state: "hidden" });

  await page.goto(baseURL + "/dashboard/approvals", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Approvals" }).waitFor();
  const approveButton = page.getByRole("button", { name: "Approve", exact: true }).first();
  if (await approveButton.count()) {
    await approveButton.click();
    await page.getByRole("alertdialog").waitFor();
    await page.getByRole("button", { name: "Cancel" }).click();
    await page.getByRole("alertdialog").waitFor({ state: "hidden" });
  }

  await page.goto(baseURL + "/dashboard/knowledge", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Knowledge" }).waitFor();
  await page.getByRole("button", { name: "Add source" }).click();
  await page.getByRole("heading", { name: "Add a knowledge source" }).waitFor();
  await page.keyboard.press("Escape");
  await page.getByRole("heading", { name: "Add a knowledge source" }).waitFor({ state: "hidden" });

  await page.goto(baseURL + "/dashboard/settings/providers", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "AI providers" }).waitFor();
  await page.getByRole("button", { name: "Add provider" }).click();
  await page.getByRole("heading", { name: /^(Update provider|Connect a provider)$/ }).waitFor();
  await page.keyboard.press("Escape");

  await page.goto(baseURL + "/dashboard/settings/agent", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Support agent" }).waitFor();
  await page.getByText("Tools and approvals").waitFor();

  await page.goto(baseURL + "/dashboard/settings/sla", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "SLA policies" }).waitFor();
  await page.getByText("Targets by priority").waitFor();

  await page.goto(baseURL + "/dashboard/settings/provisioning", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "SCIM provisioning" }).waitFor();
  await page.getByText("SCIM 2.0 endpoint").waitFor();

  await page.goto(baseURL + "/dashboard/settings/email", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Email channel" }).waitFor();
  await page.getByText("Outbound delivery").waitFor();

  await page.goto(baseURL + "/dashboard/customers", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Customers" }).waitFor();
  await page.locator('a[href^="/dashboard/customers/"]').first().click();
  await page.getByRole("heading", { level: 1 }).first().waitFor();
  await page.getByText("Conversations", { exact: true }).waitFor();

  await page.goto(baseURL + "/dashboard/audit", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Audit log" }).waitFor();
  await page.getByText("Append-only record").first().waitFor();

  await page.goto(baseURL + "/dashboard/account", { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: "Account" }).waitFor();
  await page.getByText("Session protected").waitFor();
} catch (error) {
  extra.push("flow: " + error.message);
}

await context.close();
await browser.close();
if (failures.length || extra.length) {
  console.error("console check failed:");
  for (const failure of [...failures, ...extra]) console.error("  - " + failure);
  process.exit(1);
}
console.log("console check passed");

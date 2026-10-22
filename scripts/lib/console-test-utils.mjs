// SPDX-License-Identifier: AGPL-3.0-only
export const baseURL = process.env.CHASTEDESK_URL || "http://127.0.0.1:3000";
export const demoEmail = process.env.CHASTEDESK_EMAIL || "owner@example.com";
export const demoPassword = process.env.CHASTEDESK_PASSWORD || "";

export function requireCredentials() {
  if (!demoPassword) {
    console.error("console check failed: set CHASTEDESK_PASSWORD to the demo owner password");
    process.exit(1);
  }
}

export function attachErrorCollector(page, label) {
  const failures = [];
  page.on("pageerror", (error) => failures.push(`${label}: page error: ${error.message}`));
  page.on("console", (message) => {
    if (message.type() === "error") failures.push(`${label}: console: ${message.text()}`);
  });
  page.on("response", (response) => {
    if (response.status() >= 500) failures.push(`${label}: ${response.status()} ${response.url()}`);
  });
  return failures;
}

export async function signIn(page, target = baseURL) {
  requireCredentials();
  await page.goto(target + "/login", { waitUntil: "networkidle" });
  await page.getByLabel("Email").fill(demoEmail);
  await page.getByLabel("Password", { exact: true }).fill(demoPassword);
  await page.getByRole("button", { name: /^Sign in/ }).click();
  await page.waitForURL((url) => url.pathname === "/dashboard" || url.pathname === "/onboarding", { timeout: 30_000 });
  if (new URL(page.url()).pathname === "/onboarding") {
    throw new Error("demo owner is still in onboarding; complete onboarding or use an owner with a finished workspace");
  }
  await page.getByText("Desk brief").first().waitFor({ timeout: 20_000 });
  await page.waitForTimeout(400);
}

export async function apiGet(page, path) {
  const response = await page.request.get(baseURL + path);
  if (!response.ok()) {
    throw new Error(`GET ${path} returned ${response.status()}: ${await response.text()}`);
  }
  const body = await response.json();
  return body.data;
}

export function assert(condition, message) {
  if (!condition) throw new Error(message);
}

export function fail(collected, extra = []) {
  const failures = [...collected, ...extra];
  if (failures.length > 0) {
    console.error("console check failed:");
    for (const failure of failures) console.error("  - " + failure);
    process.exit(1);
  }
}

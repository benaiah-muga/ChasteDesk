// SPDX-License-Identifier: AGPL-3.0-only
import { execFileSync, spawnSync } from "node:child_process";
import crypto from "node:crypto";

const project = "chastedesk-smoke-" + crypto.randomBytes(3).toString("hex");
const port = 3900 + Math.floor(Math.random() * 80);
const env = {
  ...process.env,
  APP_SECRET: crypto.randomBytes(48).toString("base64"),
  APP_URL: `http://127.0.0.1:${port}`,
  PORT: String(port),
  POSTGRES_PASSWORD: "smoke-" + crypto.randomBytes(8).toString("hex"),
};
const compose = (args) => spawnSync("docker", ["compose", "-p", project, ...args], { env, encoding: "utf8", stdio: "pipe" });
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function waitFor(url, tries = 60) {
  for (let attempt = 0; attempt < tries; attempt += 1) {
    try {
      const response = await fetch(url, { redirect: "manual" });
      if (response.status < 500) return response;
    } catch (error) {
      // keep polling while the stack starts
    }
    await sleep(2000);
  }
  throw new Error("timed out waiting for " + url);
}

let failed = false;
try {
  const up = compose(["up", "-d", "--build", "--wait"]);
  if (up.status !== 0) throw new Error("compose up failed: " + up.stderr + up.stdout);
  const health = await waitFor(`http://127.0.0.1:${port}/api/health`);
  if (health.status !== 200) throw new Error("health status " + health.status);
  const ready = await waitFor(`http://127.0.0.1:${port}/api/ready`);
  if (ready.status !== 200) throw new Error("readiness status " + ready.status);
  const setup = await fetch(`http://127.0.0.1:${port}/setup`, { redirect: "manual" });
  if (setup.status !== 200) throw new Error("setup page status " + setup.status);
  const setupBody = await setup.text();
  if (!setupBody.includes("Create your desk")) throw new Error("setup page did not render");
  const widget = await fetch(`http://127.0.0.1:${port}/widget.js`);
  const widgetBody = await widget.text();
  if (widget.status !== 200 || !widgetBody.includes("chastedesk.widget")) throw new Error("widget bundle not served");
  const root = await fetch(`http://127.0.0.1:${port}/`, { redirect: "manual" });
  if (root.status !== 303 || root.headers.get("location") !== "/setup") throw new Error("root did not redirect to setup");
  try {
    execFileSync("grep", ["-q", "CHASTESK_INGEST_ALLOW_PRIVATE", "docker-compose.yml"]);
    throw new Error("compose file contains the SSRF test escape hatch");
  } catch (error) {
    if (error.message.includes("contains the SSRF")) throw error;
  }
} catch (error) {
  failed = true;
  console.error("compose smoke failed: " + error.message);
  const logs = compose(["logs", "--no-color", "--tail", "80"]);
  console.error(logs.stdout || "");
  console.error(logs.stderr || "");
} finally {
  const down = compose(["down", "-v", "--remove-orphans"]);
  if (down.status !== 0) console.error("compose down failed: " + (down.stderr || ""));
}

if (failed) process.exit(1);
console.log("compose smoke passed");

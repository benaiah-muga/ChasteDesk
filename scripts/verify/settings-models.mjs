// SPDX-License-Identifier: AGPL-3.0-only
// Gate: model listing and the connectivity test succeed and fail correctly
// against a mock OpenAI-compatible provider.
import { startMockOpenAI } from "./lib/mock-openai.mjs";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4342;

const mock = await startMockOpenAI();

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: APP_PORT, appUrl: `https://127.0.0.1:${APP_PORT}` });
  try {
    const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Models Org", userName: "Owner", email: "owner@models.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@models.test", password: "password123" },
      jar,
    });

    const save = await req(`http://127.0.0.1:${APP_PORT}/api/settings/providers`, {
      method: "POST",
      jar,
      body: { providerId: "openai", displayName: "Mock OpenAI", apiKey: "test-key", baseUrl: mock.url },
    });
    assert(save.res.status === 201, "provider save failed");

    const models = await req(`http://127.0.0.1:${APP_PORT}/api/settings/models?configId=`, { jar });
    void models;

    const list = await req(`http://127.0.0.1:${APP_PORT}/api/settings/providers`, { jar });
    const providerId = list.json.data[0].id;
    const modelsRes = await req(`http://127.0.0.1:${APP_PORT}/api/settings/models?configId=${providerId}`, { jar });
    assert(modelsRes.res.status === 200, `models listing failed: ${modelsRes.res.status}`);
    assert(modelsRes.json.data.some((m) => m.id === "mock-mini"), "mock-mini missing from model list");

    const test = await req(`http://127.0.0.1:${APP_PORT}/api/settings/providers-connect`, { method: "POST", jar, body: { configId: providerId } });
    assert(test.res.status === 200 && test.json.data.ok === true, `connectivity test should pass: ${JSON.stringify(test.json)}`);

    const broken = await req(`http://127.0.0.1:${APP_PORT}/api/settings/providers`, {
      method: "POST",
      jar,
      body: { providerId: "openai-compatible", displayName: "Broken", baseUrl: "http://127.0.0.1:9/v1" },
    });
    assert(broken.res.status === 201, "broken provider save failed");
    const list2 = await req(`http://127.0.0.1:${APP_PORT}/api/settings/providers`, { jar });
    const brokenId = list2.json.data.find((p) => p.providerId === "openai-compatible").id;
    const test2 = await req(`http://127.0.0.1:${APP_PORT}/api/settings/providers-connect`, { method: "POST", jar, body: { configId: brokenId } });
    assert(test2.json.data.ok === false, "connectivity test against dead endpoint must fail");

    let negativeCaught = false;
    try {
      assert(test2.json.data.ok === true, "probe");
    } catch {
      negativeCaught = true;
    }
    assert(negativeCaught, "connectivity assertions cannot fail");
  } finally {
    await app.stop();
  }
});

await mock.stop();
ok("model settings verification passed");

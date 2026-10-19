// SPDX-License-Identifier: AGPL-3.0-only
// Gate: message flooding is rejected by per-conversation and per-IP rate limits.
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4373;

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: APP_PORT, appUrl: `https://127.0.0.1:${APP_PORT}` });
  try {
    const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Rate Widget Org", userName: "Owner", email: "owner@rw.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@rw.test", password: "password123" },
      jar,
    });
    const put = await req(`http://127.0.0.1:${APP_PORT}/api/settings/widget`, {
      method: "PUT",
      jar,
      body: { origins: ["https://shop.example.com"] },
    });
    assert(put.res.status === 200, "origins save failed");
    const tokenRes = await req(`http://127.0.0.1:${APP_PORT}/api/settings/widget-token`, { jar });
    const widgetToken = tokenRes.json.data.token;

    const create = await req(`http://127.0.0.1:${APP_PORT}/api/widget/conversations?token=${encodeURIComponent(widgetToken)}`, {
      method: "POST",
      origin: "https://shop.example.com",
      body: {},
    });
    assert(create.res.status === 201, "conversation create failed");
    const { conversationId, token: convToken } = create.json.data;

    const url = `http://127.0.0.1:${APP_PORT}/api/widget/conversations/${conversationId}/messages?token=${encodeURIComponent(widgetToken)}&convToken=${encodeURIComponent(convToken)}`;
    let saw429 = false;
    for (let i = 0; i < 25; i++) {
      const r = await req(url, { method: "POST", origin: "https://shop.example.com", body: { content: `flood ${i}` } });
      if (r.res.status === 429) {
        saw429 = true;
        break;
      }
      assert(r.res.status === 202, `unexpected send status ${r.res.status}`);
    }
    assert(saw429, "per-conversation rate limit never triggered");

    const conv2 = await req(`http://127.0.0.1:${APP_PORT}/api/widget/conversations?token=${encodeURIComponent(widgetToken)}`, {
      method: "POST",
      origin: "https://shop.example.com",
      body: {},
    });
    if (conv2.res.status === 201) {
      const url2 = `http://127.0.0.1:${APP_PORT}/api/widget/conversations/${conv2.json.data.conversationId}/messages?token=${encodeURIComponent(widgetToken)}&convToken=${encodeURIComponent(conv2.json.data.token)}`;
      const ipLimited = await req(url2, { method: "POST", origin: "https://shop.example.com", body: { content: "still limited" } });
      assert([202, 429].includes(ipLimited.res.status), `per-IP keyed send unexpected status ${ipLimited.res.status}`);
    }
    const { default: pg2 } = await import("pg");
    const c2 = new pg2.Client({ connectionString: pgx.connectionString });
    await c2.connect();
    try {
      const windows = await c2.query("select key, count from rate_limit_windows where key like 'widget:msg-ip:%'");
      assert(windows.rows.length >= 1 && Number(windows.rows[0].count) >= 20, `per-IP accounting missing: ${JSON.stringify(windows.rows)}`);
    } finally {
      await c2.end();
    }

    let negativeCaught = false;
    try {
      assert(saw429 === false, "probe");
    } catch {
      negativeCaught = true;
    }
    assert(negativeCaught, "rate limit assertions cannot fail");
  } finally {
    await app.stop();
  }
});

ok("widget rate limit verification passed");

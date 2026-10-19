// SPDX-License-Identifier: AGPL-3.0-only
// Gate: widget endpoints accept allowed origins with valid tokens and reject
// disallowed, absent, and expired trust.
import { randomUUID } from "node:crypto";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const APP_PORT = 4371;
process.env.CHASTESK_INGEST_ALLOW_PRIVATE = "1";

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: APP_PORT, appUrl: `https://127.0.0.1:${APP_PORT}` });
  try {
    const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Widget Org", userName: "Owner", email: "owner@widget.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@widget.test", password: "password123" },
      jar,
    });

    const put = await req(`http://127.0.0.1:${APP_PORT}/api/settings/widget`, {
      method: "PUT",
      jar,
      body: { origins: ["https://shop.example.com"] },
    });
    assert(put.res.status === 200, `widget origins save failed: ${put.res.status}`);

    const tokenRes = await req(`http://127.0.0.1:${APP_PORT}/api/settings/widget-token`, { jar });
    assert(tokenRes.res.status === 200, "widget token fetch failed");
    const token = tokenRes.json.data.token;

    const good = await req(`http://127.0.0.1:${APP_PORT}/api/widget/config?token=${encodeURIComponent(token)}`, {
      origin: "https://shop.example.com",
    });
    assert(good.res.status === 200, `valid origin+token must pass: ${good.res.status} ${JSON.stringify(good.json)}`);

    const badOrigin = await req(`http://127.0.0.1:${APP_PORT}/api/widget/config?token=${encodeURIComponent(token)}`, {
      origin: "https://evil.example.net",
    });
    assert(badOrigin.res.status === 403, `disallowed origin must 403, got ${badOrigin.res.status}`);

    const noOrigin = await fetch(`http://127.0.0.1:${APP_PORT}/api/widget/config?token=${encodeURIComponent(token)}`);
    assert(noOrigin.status === 403, `absent origin must 403, got ${noOrigin.status}`);

    const tampered = await req(`http://127.0.0.1:${APP_PORT}/api/widget/config?token=${encodeURIComponent(token.slice(0, -3) + "abc")}`, {
      origin: "https://shop.example.com",
    });
    assert(tampered.res.status === 401, `tampered token must 401, got ${tampered.res.status}`);

    const { createHmac } = await import("node:crypto");
    const { deriveSubkey } = await import("../../packages/db/src/crypto.ts");
    const { TEST_APP_SECRET } = await import("./lib/web-app.mjs");
    const dot = token.indexOf(".");
    const body = token.slice(0, dot);
    const payload = JSON.parse(Buffer.from(body, "base64url").toString("utf8"));
    payload.exp = 1000000000;
    const expiredBody = Buffer.from(JSON.stringify(payload)).toString("base64url");
    const expiredSig = createHmac("sha256", deriveSubkey(TEST_APP_SECRET, `widget-${boot.json.data.orgId}`)).update(expiredBody).digest("base64url");
    const expiredToken = `${expiredBody}.${expiredSig}`;
    const expired = await req(`http://127.0.0.1:${APP_PORT}/api/widget/config?token=${encodeURIComponent(expiredToken)}`, {
      origin: "https://shop.example.com",
    });
    assert(expired.res.status === 401, `expired token must 401, got ${expired.res.status}`);

    let negativeCaught = false;
    try {
      assert(good.res.status === 999, "probe");
    } catch {
      negativeCaught = true;
    }
    assert(negativeCaught, "origin assertions cannot fail");
  } finally {
    await app.stop();
  }
});

delete process.env.CHASTESK_INGEST_ALLOW_PRIVATE;
ok("widget origin verification passed");

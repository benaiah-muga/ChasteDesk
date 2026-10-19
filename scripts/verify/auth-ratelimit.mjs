// SPDX-License-Identifier: AGPL-3.0-only
// Gate: repeated failed logins trigger lockout with 429s until the window expires.
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { req, ensureWebBuilt, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const PORT = 4314;

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({
    dbUrl: pgx.connectionString,
    port: PORT,
    appUrl: `https://127.0.0.1:${PORT}`,
  });
  try {
    const boot = await req(`http://127.0.0.1:${PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Rate Org", userName: "R", email: "owner@rate.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");

    const first = await req(`http://127.0.0.1:${PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@rate.test", password: "wrong-password" },
    });
    assert(first.res.status !== 429, "first failed login must not be rate limited");

    let saw429 = false;
    for (let i = 0; i < 12; i++) {
      const r = await req(`http://127.0.0.1:${PORT}/api/auth/sign-in/email`, {
        method: "POST",
        body: { email: "owner@rate.test", password: "wrong-password" },
      });
      if (r.res.status === 429) {
        saw429 = true;
        break;
      }
    }
    assert(saw429, "lockout never triggered after repeated failed logins");

    const correctDuringLock = await req(`http://127.0.0.1:${PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@rate.test", password: "password123" },
    });
    assert(correctDuringLock.res.status === 429, `correct credentials during lockout must 429, got ${correctDuringLock.res.status}`);
  } finally {
    await app.stop();
  }
});

ok("auth rate limit verification passed");

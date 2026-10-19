// SPDX-License-Identifier: AGPL-3.0-only
// Gate: saved provider keys exist only as ciphertext, API responses mask them,
// and logs contain no key material.
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const PORT = 4341;
const KEY = "sk-gate-visible-key-material-554433";

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: PORT, appUrl: `https://127.0.0.1:${PORT}` });
  try {
    const boot = await req(`http://127.0.0.1:${PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Keys Org", userName: "Owner", email: "owner@keys.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");
    const jar = new Jar();
    await req(`http://127.0.0.1:${PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@keys.test", password: "password123" },
      jar,
    });

    const save = await req(`http://127.0.0.1:${PORT}/api/settings/providers`, {
      method: "POST",
      jar,
      body: { providerId: "openai", displayName: "OpenAI", apiKey: KEY },
    });
    assert(save.res.status === 201, `provider save failed: ${save.res.status} ${JSON.stringify(save.json)}`);

    const list = await req(`http://127.0.0.1:${PORT}/api/settings/providers`, { jar });
    assert(list.res.status === 200, "provider list failed");
    const row = list.json.data.find((p) => p.providerId === "openai");
    assert(row !== undefined, "saved provider missing from list");
    assert(row.keyLast4 === KEY.slice(-4), `masked key must end with last4, got ${row.keyLast4}`);
    assert(!JSON.stringify(list.json).includes(KEY), "api response leaked the plaintext key");

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const stored = await client.query("select key_cipher, key_iv, key_tag, key_last4 from provider_configs where org_id = $1", [boot.json.data.orgId]);
      const row2 = stored.rows[0];
      assert(row2.key_cipher && row2.key_cipher !== KEY, "key must be stored encrypted");
      assert(!row2.key_cipher.includes(KEY.slice(8, 24)), "ciphertext must not embed plaintext fragments");
      assert(row2.key_last4 === KEY.slice(-4), "last4 mismatch");
    } finally {
      await client.end();
    }

    const appLog = app.output();
    assert(!appLog.includes(KEY), "app logs contain the plaintext key");

    let negativeCaught = false;
    try {
      assert(row.keyLast4 === "wrong", "probe");
    } catch {
      negativeCaught = true;
    }
    assert(negativeCaught, "mask assertions cannot fail");
  } finally {
    await app.stop();
  }
});

ok("settings secrets verification passed");

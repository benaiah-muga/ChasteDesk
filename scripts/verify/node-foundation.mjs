// SPDX-License-Identifier: AGPL-3.0-only
// Node-1.1: bootstrap, login, provider save through the API, audit row end to end.
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const PORT = 4331;

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: PORT, appUrl: `https://127.0.0.1:${PORT}` });
  try {
    const boot = await req(`http://127.0.0.1:${PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Integrated Org", userName: "Owner", email: "owner@node1.test", password: "password123" },
    });
    assert(boot.res.status === 201, `bootstrap failed: ${boot.res.status}`);

    const jar = new Jar();
    const login = await req(`http://127.0.0.1:${PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@node1.test", password: "password123" },
      jar,
    });
    assert(login.res.status === 200, "login after bootstrap failed");

    const providers = await req(`http://127.0.0.1:${PORT}/api/settings/providers`, { jar });
    assert(providers.res.status === 200 && Array.isArray(providers.json.data), "provider list endpoint failed");

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const audits = await client.query(
        "select count(*)::int as c from audit_events where org_id = $1 and type = 'org.created'",
        [boot.json.data.orgId],
      );
      assert(audits.rows[0].c === 1, "org.created audit row missing after bootstrap+login");
    } finally {
      await client.end();
    }
  } finally {
    await app.stop();
  }
});

ok("foundation integration verification passed");

// SPDX-License-Identifier: AGPL-3.0-only
// Gate: role matrix - agent denied settings and audit, admin allowed, anonymous 401.
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const PORT = 4313;

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: PORT, appUrl: `https://127.0.0.1:${PORT}` });
  try {
    const boot = await req(`http://127.0.0.1:${PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Rbac Org", userName: "Boss", email: "owner@rbac.test", password: "password123" },
    });
    assert(boot.res.status === 201, "bootstrap failed");

    const ownerJar = new Jar();
    const ownerLogin = await req(`http://127.0.0.1:${PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@rbac.test", password: "password123" },
      jar: ownerJar,
    });
    assert(ownerLogin.res.status === 200, "owner login failed");

    for (const [name, email] of [
      ["Admin Person", "admin@rbac.test"],
      ["Agent Person", "agent@rbac.test"],
    ]) {
      const signup = await req(`http://127.0.0.1:${PORT}/api/auth/sign-up/email`, {
        method: "POST",
        body: { name, email, password: "password123" },
      });
      assert(signup.res.status === 200 || signup.res.status === 201, `signup failed for ${email}`);
    }

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const org = (await client.query("select id from organization limit 1")).rows[0];
      for (const [email, role] of [
        ["admin@rbac.test", "admin"],
        ["agent@rbac.test", "agent"],
      ]) {
        const u = (await client.query('select id from "user" where email=$1', [email])).rows[0];
        await client.query(
          "insert into member (id, organization_id, user_id, role) values (gen_random_uuid()::text, $1, $2, $3)",
          [org.id, u.id, role],
        );
      }
    } finally {
      await client.end();
    }

    const adminJar = new Jar();
    const agentJar = new Jar();
    const adminLogin = await req(`http://127.0.0.1:${PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "admin@rbac.test", password: "password123" },
      jar: adminJar,
    });
    const agentLogin = await req(`http://127.0.0.1:${PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "agent@rbac.test", password: "password123" },
      jar: agentJar,
    });
    assert(adminLogin.res.status === 200, "admin login failed");
    assert(agentLogin.res.status === 200, "agent login failed");

    const agentAudit = await req(`http://127.0.0.1:${PORT}/api/audit`, { jar: agentJar });
    assert(agentAudit.res.status === 403, `agent audit access must 403, got ${agentAudit.res.status}`);
    const agentSettings = await req(`http://127.0.0.1:${PORT}/api/settings/providers`, { jar: agentJar });
    assert(agentSettings.res.status === 403, `agent settings access must 403, got ${agentSettings.res.status}`);

    const adminAudit = await req(`http://127.0.0.1:${PORT}/api/audit`, { jar: adminJar });
    assert(adminAudit.res.status === 200, `admin audit access must 200, got ${adminAudit.res.status}`);
    const adminSettings = await req(`http://127.0.0.1:${PORT}/api/settings/providers`, { jar: adminJar });
    assert(adminSettings.res.status === 200, `admin settings access must 200, got ${adminSettings.res.status}`);

    const ownerAudit = await req(`http://127.0.0.1:${PORT}/api/audit`, { jar: ownerJar });
    assert(ownerAudit.res.status === 200, `owner audit access must 200, got ${ownerAudit.res.status}`);

    const anonymous = await req(`http://127.0.0.1:${PORT}/api/audit`);
    assert(anonymous.res.status === 401, `anonymous audit access must 401, got ${anonymous.res.status}`);

    const forged = await fetch(`http://127.0.0.1:${PORT}/api/audit`, {
      headers: { cookie: "chastedesk.session=forged-token-value" },
    });
    assert(forged.status === 401, `forged session must 401, got ${forged.status}`);
  } finally {
    await app.stop();
  }
});

ok("rbac verification passed");

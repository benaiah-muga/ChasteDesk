// SPDX-License-Identifier: AGPL-3.0-only
// Gate: first-run bootstrap creates org, owner, defaults exactly once; repeats 409.
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { req, ensureWebBuilt, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const PORT = 4312;

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: PORT, appUrl: `https://127.0.0.1:${PORT}` });
  try {
    const first = await req(`http://127.0.0.1:${PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Bootstrap Org", userName: "Root", email: "root@boot.test", password: "password123" },
    });
    assert(first.res.status === 201, `bootstrap failed: ${first.res.status} ${JSON.stringify(first.json)}`);
    assert(typeof first.json.data.orgId === "string", "bootstrap must return orgId");

    const repeat = await req(`http://127.0.0.1:${PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Second Org", userName: "X", email: "x@boot.test", password: "password123" },
    });
    assert(repeat.res.status === 409, `second bootstrap must 409, got ${repeat.res.status}`);
    assert(repeat.json?.error?.code === "conflict", "409 body must use conflict code");

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const orgs = await client.query("select count(*)::int as c from organization");
      assert(orgs.rows[0].c === 1, `expected exactly one organization, got ${orgs.rows[0].c}`);
      const agentsRows = await client.query("select policy from agents");
      assert(agentsRows.rows.length === 1, "bootstrap must seed exactly one default agent");
      const policy = agentsRows.rows[0].policy;
      assert(typeof policy?.purpose === "string" && policy.purpose.length > 10, "default agent policy missing purpose");
      assert(Array.isArray(policy?.tools) && policy.tools.length > 0, "default agent policy missing tools");
      const widgets = await client.query("select count(*)::int as c from widget_configs");
      assert(widgets.rows[0].c === 1, "bootstrap must seed a widget config");
      const audits = await client.query("select count(*)::int as c from audit_events where type='org.created'");
      assert(audits.rows[0].c === 1, "bootstrap must audit org.created");
    } finally {
      await client.end();
    }

    const status = await req(`http://127.0.0.1:${PORT}/api/onboarding/status`);
    assert(status.json?.data?.orgCreated === false || status.json?.data != null, "onboarding status endpoint must respond");
  } finally {
    await app.stop();
  }
});

ok("bootstrap verification passed");

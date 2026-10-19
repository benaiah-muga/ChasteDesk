// SPDX-License-Identifier: AGPL-3.0-only
// Gate: an external system can create conversations and send messages over the
// API key channel into the same AI pipeline.
import { randomUUID, createHash } from "node:crypto";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
process.env.DRIZZLE_LOG = "1";
const APP_PORT = 4374;

const sha = (s) => createHash("sha256").update(s).digest("hex");

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: APP_PORT, appUrl: `https://127.0.0.1:${APP_PORT}` });
  const { startBoss } = await import("../../apps/worker/src/boss.ts");
  const running = await startBoss(pgx.connectionString, { uploadsDir: "data/uploads" });
  try {
    const boot = await req(`http://127.0.0.1:${APP_PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "API Org", userName: "Owner", email: "owner@api.test", password: "password123" },
    });
    assert(boot.res.status === 201, `bootstrap failed: ${boot.res.status} ${JSON.stringify(boot.json)}`);
    const orgId = boot.json.data.orgId;
    const jar = new Jar();
    await req(`http://127.0.0.1:${APP_PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@api.test", password: "password123" },
      jar,
    });

    const { default: pg } = await import("pg");
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      const plaintext = "cd_gatelivekey0001";
      await client.query(
        "insert into api_keys (id, org_id, name, key_prefix, key_hash) values ($1, $2, 'Gate key', $3, $4)",
        [randomUUID(), orgId, plaintext.slice(0, 8), sha(plaintext)],
      );

      const auth = { authorization: `Bearer ${plaintext}` };
      const create = await req(`http://127.0.0.1:${APP_PORT}/api/v1/conversations`, {
        method: "POST",
        headers: auth,
        body: { customer: { email: "external@corp.test", name: "External Corp" }, message: "Hello from the external system." },
      });
      if (create.res.status !== 201) {
        const { default: pgx2 } = await import("pg");
        const c2 = new pgx2.Client({ connectionString: pgx.connectionString });
        await c2.connect();
        const cust = await c2.query("select id, email from customers");
        console.error("DEBUG customers:", JSON.stringify(cust.rows));
        await c2.end();
        console.error("DEBUG app log tail:", app.output().slice(-1200));
      }
      assert(create.res.status === 201, `api create failed: ${create.res.status} ${JSON.stringify(create.json)}`);
      const conversationId = create.json.data.conversationId;

      const send = await req(`http://127.0.0.1:${APP_PORT}/api/v1/conversations/${conversationId}/messages`, {
        method: "POST",
        headers: auth,
        body: { content: "Second message from the external system." },
      });
      assert(send.res.status === 202, `api send failed: ${send.res.status}`);

      const wrong = await req(`http://127.0.0.1:${APP_PORT}/api/v1/conversations`, {
        method: "POST",
        headers: { authorization: "Bearer cd_wrongkey0000000000" },
        body: { customer: { email: "x@y.test" }, message: "hi" },
      });
      assert(wrong.res.status === 401, `wrong key must 401, got ${wrong.res.status}`);

      const revoked = "cd_revokedkey0001";
      await client.query(
        "insert into api_keys (id, org_id, name, key_prefix, key_hash) values ($1, $2, 'Revoked', $3, $4)",
        [randomUUID(), orgId, revoked.slice(0, 8), sha(revoked)],
      );
      await client.query("delete from api_keys where key_hash = $1", [sha(revoked)]);
      const revokedRes = await req(`http://127.0.0.1:${APP_PORT}/api/v1/conversations`, {
        method: "POST",
        headers: { authorization: `Bearer ${revoked}` },
        body: { customer: { email: "x@y.test" }, message: "hi" },
      });
      assert(revokedRes.res.status === 401, "deleted key must 401");

      const count = await client.query(
        "select count(*)::int as c from conversations where org_id = $1 and channel = 'api'",
        [orgId],
      );
      assert(count.rows[0].c === 1, "api conversation not created");

      let negativeCaught = false;
      try {
        assert(count.rows[0].c === 999, "probe");
      } catch {
        negativeCaught = true;
      }
      assert(negativeCaught, "api channel assertions cannot fail");
    } finally {
      await client.end();
    }
  } finally {
    await running.stop();
    await app.stop();
  }
});

ok("api channel verification passed");

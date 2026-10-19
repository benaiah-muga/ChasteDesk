// SPDX-License-Identifier: AGPL-3.0-only
// Gate: migrations create the full section-5 schema on a clean pgvector
// database; --check-audit-immutable runs only the append-only audit check.
import pg from "pg";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, mustRun, ok } from "./lib/verify-kit.mjs";

const TABLES = [
  "user", "session", "account", "verification", "organization", "member", "invitation",
  "customers", "conversations", "messages", "knowledge_sources", "knowledge_chunks",
  "agents", "provider_configs", "api_keys", "widget_configs", "ai_runs", "actions",
  "audit_events", "analytics_daily",
];

const ORG_SCOPED = [
  "customers", "conversations", "messages", "knowledge_sources", "knowledge_chunks",
  "agents", "provider_configs", "api_keys", "widget_configs", "ai_runs", "actions",
  "analytics_daily",
];

const ENUMS = [
  "channel", "conversation_status", "message_role", "knowledge_source_type",
  "knowledge_scope", "knowledge_status", "action_status", "run_status", "audit_actor",
];

async function migrate(dbUrl) {
  mustRun("pnpm", ["tsx", "packages/db/src/migrate-cli.ts"], {
    env: { DATABASE_URL: dbUrl },
    timeout: 180_000,
  });
}

async function tableExists(client, name) {
  const res = await client.query(
    "select 1 from information_schema.tables where table_schema='public' and table_name=$1",
    [name],
  );
  return res.rowCount > 0;
}

async function columns(client, table) {
  const res = await client.query(
    `select column_name, data_type, udt_name, is_nullable, column_default, is_generated
     from information_schema.columns
     where table_schema='public' and table_name=$1`,
    [table],
  );
  const map = {};
  for (const row of res.rows) map[row.column_name] = row;
  return map;
}

async function fingerprint(client) {
  const tables = await client.query(
    `select table_name from information_schema.tables
     where table_schema='public' and table_type='BASE TABLE' order by table_name`,
  );
  const parts = [];
  for (const { table_name } of tables.rows) {
    const cols = await client.query(
      `select column_name, data_type, udt_name, is_nullable, is_generated
       from information_schema.columns
       where table_schema='public' and table_name=$1 order by column_name`,
      [table_name],
    );
    parts.push(
      `${table_name}:` +
        cols.rows.map((c) => `${c.column_name}|${c.data_type}|${c.udt_name}|${c.is_nullable}|${c.is_generated}`).join(";"),
    );
  }
  return parts.join("\n");
}

async function assertAuditImmutable(client) {
  const ins = await client.query(
    `insert into audit_events (org_id, actor_type, type, target_type, payload)
     values ('org_probe', 'system', 'probe.event', 'probe', '{}'::jsonb) returning id`,
  );
  const id = ins.rows[0].id;
  let updateFailed = false;
  try {
    await client.query("update audit_events set type='tampered' where id=$1", [id]);
  } catch {
    updateFailed = true;
  }
  assert(updateFailed, "UPDATE on audit_events did not fail");
  let deleteFailed = false;
  try {
    await client.query("delete from audit_events where id=$1", [id]);
  } catch {
    deleteFailed = true;
  }
  assert(deleteFailed, "DELETE on audit_events did not fail");
  const again = await client.query(
    `insert into audit_events (org_id, actor_type, type, target_type, payload)
     values ('org_probe', 'system', 'probe.event2', 'probe', '{}'::jsonb) returning id`,
  );
  assert(again.rowCount === 1, "INSERT into audit_events must remain permitted");
}

if (process.argv.includes("--check-audit-immutable")) {
  await withEphemeralPg(async (pgx) => {
    await migrate(pgx.connectionString);
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      await assertAuditImmutable(client);
    } finally {
      await client.end();
    }
  });
  ok("audit immutability verification passed");
} else {
  await withEphemeralPg(async (pgx) => {
    await migrate(pgx.connectionString);
    const client = new pg.Client({ connectionString: pgx.connectionString });
    await client.connect();
    try {
      for (const table of TABLES) {
        assert(await tableExists(client, table), `missing table: ${table}`);
      }
      for (const enumName of ENUMS) {
        const res = await client.query(
          "select 1 from pg_type where typname=$1 and typtype='e'",
          [enumName],
        );
        assert(res.rowCount === 1, `missing enum type: ${enumName}`);
      }
      for (const table of ORG_SCOPED) {
        const cols = await columns(client, table);
        assert(cols.org_id, `missing org_id on ${table}`);
        assert(cols.org_id.is_nullable === "NO", `org_id must be not null on ${table}`);
      }
      const conv = await columns(client, "conversations");
      for (const col of ["status", "triage", "handoff", "first_response_at", "resolved_at", "channel"]) {
        assert(conv[col], `conversations missing column ${col}`);
      }
      const chunks = await columns(client, "knowledge_chunks");
      assert(chunks.embedding && chunks.embedding.udt_name === "vector", "knowledge_chunks.embedding must be vector");
      assert(chunks.tsv && chunks.tsv.is_generated === "ALWAYS", "knowledge_chunks.tsv must be a generated column");
      const msgs = await columns(client, "messages");
      assert(msgs.citations, "messages missing citations column");
      const prov = await columns(client, "provider_configs");
      assert(prov.key_cipher, "provider_configs missing encrypted key column");
      const trig = await client.query(
        `select tgname from pg_trigger where tgrelid='audit_events'::regclass and not tgisinternal`,
      );
      const names = trig.rows.map((r) => r.tgname);
      assert(names.includes("audit_events_no_update"), "missing audit_events_no_update trigger");
      assert(names.includes("audit_events_no_delete"), "missing audit_events_no_delete trigger");

      const fpA = await fingerprint(client);

      const insConv = await client.query(
        `insert into organization (id, name, slug) values ('org1','Org One','org-one') returning id`,
      );
      assert(insConv.rowCount === 1, "organization insert failed");
      const cust = await client.query(
        `insert into customers (org_id, email, name) values ('org1','a@b.c','A') returning id`,
      );
      await client.query(
        `insert into conversations (org_id, customer_id, channel) values ('org1',$1,'widget')`,
        [cust.rows[0].id],
      );
      const convCount = await client.query(
        "select count(*)::int as c from conversations where org_id='org1'",
      );
      assert(convCount.rows[0].c === 1, "org-scoped conversation insert failed");

      const tablesAfter = await client.query(
        "select table_name from information_schema.tables where table_schema='public' and table_type='BASE TABLE'",
      );
      for (const { table_name } of tablesAfter.rows) {
        await client.query(`drop table if exists "${table_name}" cascade`);
      }
      const enumTypes = await client.query(
        `select t.typname from pg_type t join pg_namespace n on n.oid=t.typnamespace
         where n.nspname='public' and t.typtype='e'`,
      );
      for (const { typname } of enumTypes.rows) {
        await client.query(`drop type if exists "${typname}" cascade`);
      }
      await client.query("drop schema if exists drizzle cascade");
      const remaining = await client.query(
        "select count(*)::int as c from information_schema.tables where table_schema='public' and table_type='BASE TABLE'",
      );
      assert(remaining.rows[0].c === 0, "down-drop left tables behind");

      await migrate(pgx.connectionString);
      const fpB = await fingerprint(client);
      assert(fpA === fpB, "re-migrated schema differs from original fingerprint");
      const org = await tableExists(client, "organization");
      assert(org, "re-migrated database missing tables");
    } finally {
      await client.end();
    }
  });
  ok("database migration verification passed");
}

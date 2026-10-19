// SPDX-License-Identifier: AGPL-3.0-only
// Gate: down-and-back-up is lossless at the schema level, verified against a
// second independent ephemeral database for determinism.
import pg from "pg";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, mustRun, ok } from "./lib/verify-kit.mjs";

async function migrate(dbUrl) {
  mustRun("pnpm", ["tsx", "packages/db/src/migrate-cli.ts"], {
    env: { DATABASE_URL: dbUrl },
    timeout: 180_000,
  });
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

async function dropAll(client) {
  const tables = await client.query(
    `select table_name from information_schema.tables
     where table_schema='public' and table_type='BASE TABLE' order by table_name desc`,
  );
  for (const { table_name } of tables.rows) {
    await client.query(`drop table if exists "${table_name}" cascade`);
  }
  const types = await client.query(
    `select t.typname from pg_type t join pg_namespace n on n.oid=t.typnamespace
     where n.nspname='public' and t.typtype='e' order by t.typname desc`,
  );
  for (const { typname } of types.rows) {
    await client.query(`drop type if exists "${typname}" cascade`);
  }
}

let firstFp = null;
await withEphemeralPg(async (pgx) => {
  await migrate(pgx.connectionString);
  const client = new pg.Client({ connectionString: pgx.connectionString });
  await client.connect();
  try {
    firstFp = await fingerprint(client);
    await dropAll(client);
    const remaining = await client.query(
      "select count(*)::int as c from information_schema.tables where table_schema='public' and table_type='BASE TABLE'",
    );
    assert(remaining.rows[0].c === 0, "down left tables behind");
  } finally {
    await client.end();
  }
});

await withEphemeralPg(async (pgx) => {
  await migrate(pgx.connectionString);
  const client = new pg.Client({ connectionString: pgx.connectionString });
  await client.connect();
  try {
    const secondFp = await fingerprint(client);
    assert(firstFp !== null, "first fingerprint missing");
    assert(firstFp === secondFp, "schema fingerprint differs between independent clean migrations");
  } finally {
    await client.end();
  }
});

ok("migration rollback verification passed");

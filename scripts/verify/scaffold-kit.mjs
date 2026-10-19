// SPDX-License-Identifier: AGPL-3.0-only
// Gate: ephemeral pgvector helper and verify-kit helpers work.
import pg from "pg";
import { startEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";

assert(1 === 1, "verify-kit assert is broken");
try {
  assert(false, "probe");
  throw new Error("verify-kit assert must throw on false");
} catch (err) {
  assert(err.message === "VERIFY FAIL: probe", "verify-kit assert failure shape changed");
}

const { spawnSync } = await import("node:child_process");
const pgx = await startEphemeralPg();
try {
  const client = new pg.Client({ connectionString: pgx.connectionString });
  await client.connect();
  const res = await client.query("select version() as v");
  assert(res.rows[0].v.includes("PostgreSQL"), "ephemeral pg is not a postgres server");
  await client.query("create table if not exists _probe (id int primary key)");
  await client.query("insert into _probe values (1)");
  const count = await client.query("select count(*)::int as c from _probe");
  assert(count.rows[0].c === 1, "ephemeral pg write failed");
  const vec = await client.query("select '[1,2,3]'::vector as v");
  assert(String(vec.rows[0].v).includes("1,2,3"), "pgvector extension not functional");
  await client.end();
} finally {
  await pgx.stop();
}

const ps = spawnSync("docker", ["ps", "--format", "{{.Names}}"], { encoding: "utf8" });
assert(!ps.stdout.includes(pgx.name), "ephemeral pg container was not torn down");

ok("verify kit verification passed");

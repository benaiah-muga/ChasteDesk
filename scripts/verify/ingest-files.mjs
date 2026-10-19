// SPDX-License-Identifier: AGPL-3.0-only
// Gate: pdf, md, docx, and txt fixtures ingest while oversize, disallowed-type,
// and bad-magic uploads are rejected.
import { copyFileSync, writeFileSync } from "node:fs";
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";

const ORG = "org-ingest-files";
const DIR = "data/uploads-gate";

async function migrate(dbUrl) {
  const { spawnSync } = await import("node:child_process");
  const r = spawnSync("pnpm", ["tsx", "packages/db/src/migrate-cli.ts"], {
    env: { ...process.env, DATABASE_URL: dbUrl },
    encoding: "utf8",
    timeout: 180_000,
  });
  assert(r.status === 0, `migration failed: ${r.stderr}`);
}

async function waitForStatus(client, sourceId, expected, timeoutMs = 60_000) {
  for (let i = 0; i < timeoutMs / 500; i++) {
    const r = await client.query("select status, error from knowledge_sources where id = $1", [sourceId]);
    const row = r.rows[0];
    if (row && row.status === expected) return row;
    await new Promise((res) => setTimeout(res, 500));
  }
  const r = await client.query("select status, error from knowledge_sources where id = $1", [sourceId]);
  return r.rows[0];
}

await withEphemeralPg(async (pgx) => {
  await migrate(pgx.connectionString);
  const { mkdirSync } = await import("node:fs");
  mkdirSync(DIR, { recursive: true });
  for (const f of ["notes.pdf", "notes.md", "notes.docx", "notes.txt"]) {
    copyFileSync(`scripts/fixtures/files/${f}`, `${DIR}/${f}`);
  }
  writeFileSync(`${DIR}/evil.exe`, Buffer.from("MZ"));
  writeFileSync(`${DIR}/fake.pdf`, "this is not a pdf");
  writeFileSync(`${DIR}/huge.txt`, "x".repeat(11 * 1024 * 1024));

  const { default: pg } = await import("pg");
  const client = new pg.Client({ connectionString: pgx.connectionString });
  await client.connect();
  try {
    await client.query("insert into organization (id, name, slug) values ($1, 'Files Org', 'files-org')", [ORG]);
    const files = [
      ["00000000-0000-4000-8000-000000000201", "notes.pdf", "ChasteDesk password reset guide"],
      ["00000000-0000-4000-8000-000000000202", "notes.md", "Password resets and billing plans"],
      ["00000000-0000-4000-8000-000000000203", "notes.docx", "password reset steps and billing plan"],
      ["00000000-0000-4000-8000-000000000204", "notes.txt", "password resets, billing plans"],
    ];
    for (const [id, filename, needle] of files) {
      await client.query(
        `insert into knowledge_sources (id, org_id, type, name, scope, status, version, source_config)
         values ($1, $2, 'file', $3, 'public', 'pending', 1, $4::jsonb)`,
        [id, ORG, filename, JSON.stringify({ path: `${DIR}/${filename}`, filename })],
      );
      const { startBoss } = await import("../../apps/worker/src/boss.ts");
      void startBoss;
    }

    const { startBoss } = await import("../../apps/worker/src/boss.ts");
    const running = await startBoss(pgx.connectionString, { uploadsDir: DIR });

    for (const [id, filename] of files) {
      await running.boss.send("ingest-file", { orgId: ORG, sourceId: id, filePath: `${DIR}/${filename}`, filename });
    }
    for (const [id, , needle] of files) {
      const row = await waitForStatus(client, id, "ready");
      assert(row && row.status === "ready", `ingestion of ${id} did not reach ready: ${JSON.stringify(row)}`);
      const chunks = await client.query("select content from knowledge_chunks where source_id = $1", [id]);
      const all = chunks.rows.map((r) => r.content).join("\n");
      assert(all.includes(needle), `parsed content missing needle for ${id}: ${all.slice(0, 200)}`);
    }

    const bad = [
      ["00000000-0000-4000-8000-000000000301", "evil.exe"],
      ["00000000-0000-4000-8000-000000000302", "fake.pdf"],
      ["00000000-0000-4000-8000-000000000303", "huge.txt"],
    ];
    for (const [id, filename] of bad) {
      await client.query(
        `insert into knowledge_sources (id, org_id, type, name, scope, status, version, source_config)
         values ($1, $2, 'file', $3, 'public', 'pending', 1, $4::jsonb)`,
        [id, ORG, filename, JSON.stringify({ path: `${DIR}/${filename}`, filename })],
      );
      await running.boss.send("ingest-file", { orgId: ORG, sourceId: id, filePath: `${DIR}/${filename}`, filename });
    }
    for (const [id] of bad) {
      const row = await waitForStatus(client, id, "failed");
      assert(row && row.status === "failed", `reject case ${id} did not fail: ${JSON.stringify(row)}`);
      assert(row.error && row.error.length > 0, `reject case ${id} missing error message`);
    }

    const audits = await client.query(
      "select count(*)::int as c from audit_events where org_id = $1 and type in ('knowledge.ingest.completed','knowledge.ingest.failed')",
      [ORG],
    );
    assert(audits.rows[0].c >= 7, `expected audit rows for every ingest outcome, got ${audits.rows[0].c}`);

    await running.stop();
  } finally {
    await client.end();
  }
});

ok("file ingestion verification passed");

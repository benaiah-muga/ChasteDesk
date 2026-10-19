// SPDX-License-Identifier: AGPL-3.0-only
// Gate: chunks vector-match only their own embedding model; stale-model
// sources are flagged for reindex and clear after reindexing.
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { embed, insertChunk, seedCorpus, SOURCES } from "./lib/retrieval-fixtures.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";

async function migrate(dbUrl) {
  const { spawnSync } = await import("node:child_process");
  const r = spawnSync("pnpm", ["tsx", "packages/db/src/migrate-cli.ts"], {
    env: { ...process.env, DATABASE_URL: dbUrl },
    encoding: "utf8",
    timeout: 180_000,
  });
  assert(r.status === 0, `migration failed: ${r.stderr}`);
}

await withEphemeralPg(async (pgx) => {
  await migrate(pgx.connectionString);
  const { default: pg } = await import("pg");
  const client = new pg.Client({ connectionString: pgx.connectionString });
  await client.connect();
  try {
    const orgId = await seedCorpus(client);
    for (let i = 0; i < 4; i++) {
      await insertChunk(client, {
        orgId,
        sourceId: SOURCES.para,
        content: `Password reset guide part ${i}: use the security settings page and credential options to reset access.`,
        embeddingModel: "model-a",
      });
    }
    for (let i = 0; i < 4; i++) {
      await insertChunk(client, {
        orgId,
        sourceId: SOURCES.modelB,
        content: `Billing plan invoice notes part ${i}: payment pricing subscription refund table.`,
        embeddingModel: "model-b",
      });
    }
    await client.query(`update knowledge_sources set embedding_model = 'model-a' where id = '${SOURCES.para}'`);
    await client.query(`update knowledge_sources set embedding_model = 'model-b' where id = '${SOURCES.modelB}'`);

    const { createBuiltinEngine } = await import("../../packages/retrieval/src/hybrid.ts");
    const engine = createBuiltinEngine(client);

    const results = await engine.search(orgId, {
      text: "reset password credential",
      scopes: ["public"],
      queryEmbedding: embed("reset password credential access security"),
      embeddingModel: "model-a",
      limit: 20,
    });
    assert(results.length > 0, "search returned nothing");
    const fromB = results.filter((e) => e.sourceId === SOURCES.modelB);
    assert(fromB.length === 0, `model-b chunks leaked into a model-a search: ${fromB.length}`);

    const stale = await engine.sourcesNeedingReindex(orgId, "model-a");
    const staleIds = stale.map((s) => s.source_id).sort();
    assert(staleIds.includes(SOURCES.modelB), "model-b source not flagged for reindex");

    await client.query(
      `update knowledge_sources set embedding_model = 'model-a' where id = '${SOURCES.modelB}'`,
    );
    await client.query(
      `update knowledge_chunks set embedding_model = 'model-a' where source_id = '${SOURCES.modelB}'`,
    );
    const staleAfter = await engine.sourcesNeedingReindex(orgId, "model-a");
    assert(
      !staleAfter.some((s) => s.source_id === SOURCES.modelB),
      "reindex flag did not clear after reindexing",
    );
  } finally {
    await client.end();
  }
});

ok("embedding metadata verification passed");

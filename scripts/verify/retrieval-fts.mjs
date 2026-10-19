// SPDX-License-Identifier: AGPL-3.0-only
// Gate: with no embeddings present, search still returns sensibly ranked
// evidence without errors, and degrades silently when vectors are missing.
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
    for (let i = 0; i < 12; i++) {
      await insertChunk(client, {
        orgId,
        sourceId: SOURCES.unrelated,
        content: `Billing doc ${i}: invoice payment plan pricing subscription refund notes and billing policy section ${i}.`,
        embeddingModel: null,
      });
    }
    await insertChunk(client, {
      orgId,
      sourceId: SOURCES.para,
      content: "Password reset instructions: to reset your password use the security page and credential options.",
      embeddingModel: null,
    });

    const { createBuiltinEngine } = await import("../../packages/retrieval/src/hybrid.ts");
    const engine = createBuiltinEngine(client);

    const results = await engine.search(orgId, { text: "reset password", scopes: ["public"], limit: 10 });
    assert(results.length > 0, "fts-only search returned nothing");
    const top = results[0];
    assert(top.content.includes("reset"), "fts-only top result should contain the query term");
    for (const e of results) {
      assert(e.confidence >= 0 && e.confidence <= 1, `confidence out of bounds: ${e.confidence}`);
      assert(e.sourceName.length > 0, "evidence missing source name");
    }

    const noMatch = await engine.search(orgId, { text: "zzzunmatchable", scopes: ["public"] });
    assert(noMatch.length >= 0, "no-match search must not throw");

    const withVecRequest = await engine.search(orgId, {
      text: "reset password",
      scopes: ["public"],
      queryEmbedding: embed("reset password"),
      embeddingModel: "test-embed-v1",
    });
    assert(withVecRequest.length > 0, "vector request against missing vectors must fall back to lexical results");
  } finally {
    await client.end();
  }
});

ok("fts fallback verification passed");

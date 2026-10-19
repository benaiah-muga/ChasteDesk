// SPDX-License-Identifier: AGPL-3.0-only
// Gate: hybrid search ranks the paraphrase chunk first with complete Evidence,
// while FTS-only leaves non-lexical chunks in the following positions.
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { embed, EMBED_MODEL, insertChunk, PARAPHRASE_CONTENT, LEXICAL_CONTENT, INTERNAL_CONTENT, seedCorpus, SOURCES } from "./lib/retrieval-fixtures.mjs";
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

    const para = await insertChunk(client, { orgId, sourceId: SOURCES.para, content: PARAPHRASE_CONTENT });
    await insertChunk(client, { orgId, sourceId: SOURCES.lexical, content: LEXICAL_CONTENT });
    await insertChunk(client, { orgId, sourceId: SOURCES.internal, content: INTERNAL_CONTENT });

    const fillerMixes = [0.71, 0.63, 0.55, 0.5, 0.45, 0.4];
    for (let i = 0; i < fillerMixes.length; i++) {
      await insertChunk(client, {
        orgId,
        sourceId: SOURCES.fillers,
        content: `Reset your login credential, then review billing plan payment settings. Filler guide ${i} for ranking depth.`,
      });
    }
    for (let i = 0; i < 20; i++) {
      await insertChunk(client, {
        orgId,
        sourceId: SOURCES.unrelated,
        content: `Unrelated doc ${i}: lorem ipsum dolor amet kale provision region tenant notice channel.`,
      });
    }

    const { createBuiltinEngine } = await import("../../packages/retrieval/src/hybrid.ts");
    const engine = createBuiltinEngine(client);

    const queryEmbedding = embed("reset my password");
    const hybrid = await engine.search(orgId, {
      text: "qsigphrase",
      scopes: ["public"],
      queryEmbedding,
      embeddingModel: EMBED_MODEL,
    });
    assert(hybrid.length > 1, `hybrid returned too few results: ${hybrid.length}`);
    const paraEvidence = hybrid.find((e) => e.chunkId === para.id);
    assert(paraEvidence !== undefined, "paraphrase chunk missing from hybrid results");
    assert(hybrid[0].chunkId === para.id, `hybrid top1 must be the paraphrase chunk, got ${hybrid[0].chunkId}`);
    assert(hybrid[0].score > hybrid[1].score, `hybrid top1 score must strictly exceed top2: ${hybrid[0].score} vs ${hybrid[1].score}`);
    for (const e of hybrid) {
      assert(e.sourceName.length > 0, "evidence missing sourceName");
      assert(e.version === 3, "evidence version mismatch");
      assert(e.scope === "public" || e.scope === "internal", "evidence scope invalid");
      assert(Array.isArray(e.locator.headings), "evidence missing locator headings");
      assert(e.confidence > 0 && e.confidence < 1, `confidence out of range: ${e.confidence}`);
    }
    assert(!hybrid.some((e) => e.scope === "internal"), "internal evidence must not leak with public scope");

    const withInternal = await engine.search(orgId, {
      text: "secret phrase",
      scopes: ["public", "internal"],
      queryEmbedding,
      embeddingModel: EMBED_MODEL,
    });
    assert(withInternal.some((e) => e.scope === "internal"), "internal evidence missing when internal scope requested");

    const ftsOnly = await engine.search(orgId, { text: "qsigphrase", scopes: ["public"] });
    assert(ftsOnly.length >= 1, "fts-only returned nothing");
    assert(ftsOnly[0].chunkId === para.id, "fts-only top1 must be the only lexical match");
    assert(ftsOnly.every((e) => e.score > 0), "fts-only rows must all be lexical matches");

    assert(hybrid.length > ftsOnly.length, "hybrid must surface concept matches beyond lexical matches");
  } finally {
    await client.end();
  }
});

ok("hybrid retrieval verification passed");
